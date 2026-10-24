// The traffic log is the file of JSON lines that holds the halves of every
// exchange when the operator turns the traffic log on. A half is appended the
// moment it exists, one file per day, pruned with the daemon log's own
// retention.
//
// Reading one request back is the call the request detail makes every time an
// operator opens a drawer, and a day file holds whole bodies of up to the
// inbound read limit each. Scanning every line of every day to find one
// request would cost more than the page, so the log keeps an index of where
// each request's lines begin: the open day's is built as it is written, and a
// finished day's is built once and cached.
package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/server"
)

const (
	trafficLogPrefix = "traffic-"
	trafficLogSuffix = ".log"
	// trafficLogKeep is the fortnight the daemon log keeps, applied here so
	// both files in the logs folder age out together.
	trafficLogKeep = daemonLogKeep

	// requestIDPrefix is the head of every line, which is what lets an index
	// read a request id without decoding the line behind it.
	requestIDPrefix = `{"request_id":`

	// indexReadBytes is how much of a line an index reads to find its request
	// id. A generated id is a few dozen bytes, so this is ample.
	indexReadBytes = 512

	// closedIndexKeep bounds how many finished days stay indexed in memory. A
	// drawer opens one request, and browsing the list moves forward through
	// the same day, so a handful of days covers a session.
	closedIndexKeep = 3

	// lineChunkBytes is how much of one line a read takes at a time, so a body
	// of tens of megabytes is not allocated in one call.
	lineChunkBytes = 64 << 10
)

// closedIndex is the offset index of one finished day, remembered with the
// file identity that produced it so a rewritten file is indexed again rather
// than read through a stale map.
type closedIndex struct {
	path    string
	modTime time.Time
	size    int64
	lines   map[string][]int64
}

// trafficLog is the writer for the state's logs folder and the offset index
// that makes reading one request back cheap.
type trafficLog struct {
	dir string
	now func() time.Time

	mu   sync.Mutex
	file *os.File
	day  string
	// size is how many bytes the open file holds, which is the offset the next
	// appended line begins at.
	size int64
	// lines indexes the open day as it is written: a request id to the offsets
	// its lines begin at.
	lines map[string][]int64
	// closed caches the index of a finished day, most recently used first.
	closed []closedIndex
}

var _ server.TrafficLog = (*trafficLog)(nil)

// newTrafficLog opens the writer for the state's logs folder.
func newTrafficLog(home string) (*trafficLog, error) {
	if _, err := os.Stat(home); err != nil {
		return nil, fmt.Errorf("open the traffic log: %w", err)
	}
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("open the traffic log: %w", err)
	}
	return &trafficLog{dir: dir, now: time.Now}, nil
}

// AppendHalf writes one half of an exchange to today's file, opening the day's
// file and dropping the days past retention the first time it is written. The
// line's offset joins the open day's index, so reading this request back later
// never scans the file it was just written to.
func (t *trafficLog) AppendHalf(half server.TrafficHalf) error {
	line, err := json.Marshal(half)
	if err != nil {
		return fmt.Errorf("encode the traffic half: %w", err)
	}
	line = append(line, '\n')
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.open(); err != nil {
		return err
	}
	if _, err := t.file.Write(line); err != nil {
		return fmt.Errorf("write the traffic log: %w", err)
	}
	t.lines[half.RequestID] = append(t.lines[half.RequestID], t.size)
	t.size += int64(len(line))
	return nil
}

func (t *trafficLog) open() error {
	day := t.now().Format("20060102")
	if t.file != nil && t.day == day {
		return nil
	}
	if t.file != nil {
		if err := t.file.Close(); err != nil {
			return err
		}
		t.file = nil
	}
	if err := os.MkdirAll(t.dir, 0o700); err != nil {
		return fmt.Errorf("open the traffic log: %w", err)
	}
	if err := pruneAged(t.dir, trafficLogPrefix, trafficLogKeep, t.now()); err != nil {
		return err
	}
	path := filepath.Join(t.dir, trafficLogPrefix+day+trafficLogSuffix)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the traffic log: %w", err)
	}
	// A file the daemon already wrote today starts where it left off, and the
	// lines behind the appends are not indexed here: a request older than this
	// process is found through the closed-day index instead.
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat the traffic log: %w", err)
	}
	t.file, t.day, t.size, t.lines = file, day, info.Size(), map[string][]int64{}
	return nil
}

// Halves returns every half the log kept of one request. The open day is read
// from the index the writer built, and a finished day is indexed once and
// cached, so opening a drawer reads that request's lines rather than the whole
// fortnight. A request the log never wrote, or whose days have aged out,
// reports none: the request detail still shows what the state rows hold.
func (t *trafficLog) Halves(ctx context.Context, requestID string) ([]server.TrafficHalf, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	openFile, openSize, openOffsets := t.openDayHalves(requestID)
	if len(openOffsets) > 0 {
		halves, err := readHalvesAt(ctx, openFile, openSize, openOffsets, requestID)
		return halves, err
	}
	paths, err := t.dayFiles()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		offsets, size, err := t.closedDayHalves(path, requestID)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if len(offsets) == 0 {
			continue
		}
		file, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		halves, readErr := readHalvesAt(ctx, file, size, offsets, requestID)
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		return halves, nil
	}
	return nil, nil
}

// openDayHalves snapshots the offsets the open day's index holds for one
// request, and the file they were written to.
func (t *trafficLog) openDayHalves(requestID string) (*os.File, int64, []int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.file == nil {
		return nil, 0, nil
	}
	offsets := t.lines[requestID]
	if len(offsets) == 0 {
		return nil, 0, nil
	}
	return t.file, t.size, append([]int64(nil), offsets...)
}

// closedDayHalves answers the offsets one finished day holds for a request,
// indexing the day first if this is the first request read from it.
func (t *trafficLog) closedDayHalves(path, requestID string) ([]int64, int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	t.mu.Lock()
	for position, cached := range t.closed {
		if cached.path != path || !cached.modTime.Equal(info.ModTime()) || cached.size != info.Size() {
			continue
		}
		if position > 0 {
			t.closed = append([]closedIndex{cached}, append(t.closed[:position], t.closed[position+1:]...)...)
		}
		offsets := cached.lines[requestID]
		t.mu.Unlock()
		return offsets, cached.size, nil
	}
	t.mu.Unlock()

	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	lines, scanErr := indexDay(file, info.Size())
	closeErr := file.Close()
	if scanErr != nil {
		return nil, 0, scanErr
	}
	if closeErr != nil {
		return nil, 0, closeErr
	}

	t.mu.Lock()
	t.closed = append([]closedIndex{{
		path: path, modTime: info.ModTime(), size: info.Size(), lines: lines,
	}}, t.closed...)
	if len(t.closed) > closedIndexKeep {
		t.closed = t.closed[:closedIndexKeep]
	}
	t.mu.Unlock()
	return lines[requestID], info.Size(), nil
}

// dayFiles lists the day's files newest first, so a request that ran today is
// found without reading the whole fortnight.
func (t *trafficLog) dayFiles() ([]string, error) {
	entries, err := os.ReadDir(t.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, trafficLogPrefix) || !strings.HasSuffix(name, trafficLogSuffix) {
			continue
		}
		names = append(names, name)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	paths := make([]string, 0, len(names))
	for _, name := range names {
		paths = append(paths, filepath.Join(t.dir, name))
	}
	return paths, nil
}

// indexDay records where each request's lines begin in one finished day. It
// reads the file in chunks and reads only a line's head to find its request
// id, so a day of whole bodies is indexed without decoding any of them.
func indexDay(file *os.File, size int64) (map[string][]int64, error) {
	lines := make(map[string][]int64)
	if size == 0 {
		return lines, nil
	}
	chunk := make([]byte, 1<<20)
	var offset, lineStart int64 = 0, -1
	for offset < size {
		read, err := file.ReadAt(chunk, offset)
		if read > 0 {
			view := chunk[:read]
			cursor := 0
			for cursor < len(view) {
				if lineStart < 0 {
					lineStart = offset + int64(cursor)
				}
				newline := bytes.IndexByte(view[cursor:], '\n')
				if newline < 0 {
					break
				}
				if id, ok := requestIDAt(file, lineStart); ok {
					lines[id] = append(lines[id], lineStart)
				}
				cursor += newline + 1
				lineStart = -1
			}
		}
		offset += int64(read)
		if errors.Is(err, io.EOF) || read == 0 {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read the traffic log: %w", err)
		}
	}
	return lines, nil
}

// requestIDAt reads the request id at the head of one line, which is all an
// index needs of it.
func requestIDAt(file *os.File, offset int64) (string, bool) {
	head := make([]byte, indexReadBytes)
	read, err := file.ReadAt(head, offset)
	if read == 0 || (err != nil && !errors.Is(err, io.EOF)) {
		return "", false
	}
	head = head[:read]
	if !bytes.HasPrefix(head, []byte(requestIDPrefix)) {
		return "", false
	}
	quoted := head[len(requestIDPrefix):]
	if len(quoted) < 2 || quoted[0] != '"' {
		return "", false
	}
	closing := bytes.IndexByte(quoted[1:], '"')
	if closing < 0 {
		return "", false
	}
	return string(quoted[1 : 1+closing]), true
}

// readHalvesAt decodes the lines one request occupies, checking the caller's
// context between them so a drawer an operator browsed past stops reading.
func readHalvesAt(ctx context.Context, file *os.File, size int64, offsets []int64, requestID string) ([]server.TrafficHalf, error) {
	halves := make([]server.TrafficHalf, 0, len(offsets))
	for _, offset := range offsets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := readLineAt(file, offset, size)
		if err != nil {
			return nil, err
		}
		var half server.TrafficHalf
		// The id is checked again rather than trusted: a line the index
		// placed here is the line that has to answer for it.
		if json.Unmarshal(line, &half) == nil && half.RequestID == requestID {
			halves = append(halves, half)
		}
	}
	return halves, nil
}

// readLineAt returns the line that begins at one offset, taking it in chunks so
// a body of tens of megabytes is not allocated in one call.
func readLineAt(file *os.File, offset, size int64) ([]byte, error) {
	room := size - offset
	if room <= 0 {
		return nil, nil
	}
	line := make([]byte, 0, 4096)
	chunk := make([]byte, lineChunkBytes)
	for int64(len(line)) < room {
		want := int64(len(chunk))
		if want > room-int64(len(line)) {
			want = room - int64(len(line))
		}
		read, err := file.ReadAt(chunk[:want], offset+int64(len(line)))
		if read > 0 {
			line = append(line, chunk[:read]...)
			if newline := bytes.IndexByte(line, '\n'); newline >= 0 {
				return line[:newline], nil
			}
		}
		if errors.Is(err, io.EOF) || read == 0 {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read the traffic log: %w", err)
		}
	}
	return line, nil
}
