// The traffic log is the file of JSON lines that holds whole agent and provider
// exchanges when the operator turns the traffic log on. One line per request,
// one file per day, pruned with the daemon log's own retention.
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

// Append writes one exchange to today's file, opening the day's file and
// dropping the days past retention the first time it is written.
func (t *trafficLog) Append(record server.TrafficRecord) error {
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode the traffic record: %w", err)
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

// Find returns the whole exchange of one request, read back from the day
// files newest first. A request the traffic log never wrote, or whose days
// have aged out, reports false rather than an error: the request detail page
// still shows what the state rows hold.
func (t *trafficLog) Find(eventID int64) (server.TrafficRecord, bool, error) {
	paths, err := t.dayFiles()
	if err != nil {
		return server.TrafficRecord{}, false, err
	}
	for _, path := range paths {
		file, openErr := os.Open(path)
		if openErr != nil {
			if os.IsNotExist(openErr) {
				continue
			}
			return server.TrafficRecord{}, false, openErr
		}
		found, scanErr := scanTrafficFile(file, eventID)
		closeErr := file.Close()
		if scanErr != nil {
			return server.TrafficRecord{}, false, scanErr
		}
		if closeErr != nil {
			return server.TrafficRecord{}, false, closeErr
		}
		if found.EventID == eventID {
			return found, true, nil
		}
	}
	return server.TrafficRecord{}, false, nil
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

// scanTrafficFile finds one request's line by its leading event_id, so the
// lines it skips are read but never decoded.
func scanTrafficFile(file *os.File, eventID int64) (server.TrafficRecord, error) {
	prefix := []byte(`{"event_id":` + strconv.FormatInt(eventID, 10) + `,`)
	reader := bufio.NewReaderSize(file, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if bytes.HasPrefix(line, prefix) {
			var record server.TrafficRecord
			if json.Unmarshal(line, &record) == nil && record.EventID == eventID {
				return record, nil
			}
		}
		if errors.Is(err, io.EOF) {
			return server.TrafficRecord{}, nil
		}
		if err != nil {
			return server.TrafficRecord{}, fmt.Errorf("read the traffic log: %w", err)
		}
	}
}
