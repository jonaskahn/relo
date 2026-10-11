// The traffic log is the file of JSON lines that holds the halves of every
// exchange when the operator turns the traffic log on. A half is appended the
// moment it exists, one file per day, pruned with the daemon log's own
// retention.
package platform

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
)

type trafficLog struct {
	dir string
	now func() time.Time

	mu   sync.Mutex
	file *os.File
	day  string
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
// file and dropping the days past retention the first time it is written.
func (t *trafficLog) AppendHalf(half server.TrafficHalf) error {
	line, err := json.Marshal(half)
	if err != nil {
		return fmt.Errorf("encode the traffic half: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := t.open(); err != nil {
		return err
	}
	if _, err := t.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write the traffic log: %w", err)
	}
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
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the traffic log: %w", err)
	}
	t.file, t.day = file, day
	return nil
}

// Halves returns every half the log kept of one request, read back from the day
// files newest first. One request's halves land in the day it ran, so the first
// file that holds any of them holds them all, but for a request that crossed
// midnight. A request the log never wrote, or whose days have aged out, reports
// none: the request detail still shows what the state rows hold.
func (t *trafficLog) Halves(requestID string) ([]server.TrafficHalf, error) {
	paths, err := t.dayFiles()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		file, openErr := os.Open(path)
		if openErr != nil {
			if os.IsNotExist(openErr) {
				continue
			}
			return nil, openErr
		}
		halves, scanErr := scanTrafficFile(file, requestID)
		closeErr := file.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(halves) > 0 {
			return halves, nil
		}
	}
	return nil, nil
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

// scanTrafficFile reads back the halves of one request, matching the request id
// at the head of each line, so the lines it skips are read but never decoded.
// A request id is generated, so JSON writes it exactly as it is spelled here.
func scanTrafficFile(file *os.File, requestID string) ([]server.TrafficHalf, error) {
	prefix := []byte(`{"request_id":` + strconv.Quote(requestID) + `,`)
	reader := bufio.NewReaderSize(file, 64<<10)
	var halves []server.TrafficHalf
	for {
		line, err := reader.ReadBytes('\n')
		if bytes.HasPrefix(line, prefix) {
			var half server.TrafficHalf
			if json.Unmarshal(line, &half) == nil && half.RequestID == requestID {
				halves = append(halves, half)
			}
		}
		if errors.Is(err, io.EOF) {
			return halves, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read the traffic log: %w", err)
		}
	}
}
