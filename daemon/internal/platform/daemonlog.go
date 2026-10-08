// The daemon log is the JSONL file every Relo process appends to. It
// rotates the way a web server rotates its error log: one current file,
// a dated gzip when the day changes or the file outgrows its cap, and a
// history that does not keep more than a fortnight.
package platform

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	daemonLogMaxBytes = 100 << 20
	daemonLogKeep     = 14 * 24 * time.Hour
	startupLogKeep    = 14 * 24 * time.Hour
	logDirName        = "logs"
)

type daemonLog struct {
	dir      string
	path     string
	lockPath string
	maxBytes int64
	keep     time.Duration
	now      func() time.Time

	mu   sync.Mutex
	lock *os.File
	file *os.File
	day  string
	size int64
}

func openDaemonLog(home string) (*daemonLog, error) {
	return openDaemonLogAt(home, daemonLogMaxBytes, daemonLogKeep, time.Now)
}

func openDaemonLogAt(home string, maxBytes int64, keep time.Duration, now func() time.Time) (*daemonLog, error) {
	if _, err := os.Stat(home); err != nil {
		return nil, fmt.Errorf("open the daemon log: %w", err)
	}
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("open the daemon log: %w", err)
	}
	log := &daemonLog{
		dir: dir, path: filepath.Join(dir, daemonLogName),
		lockPath: filepath.Join(dir, "daemon.log.lock"),
		maxBytes: maxBytes, keep: keep, now: now,
	}
	return log, nil
}

// Write appends one log line under the inter-process lock, so two daemons
// never interleave on the same file.
func (d *daemonLog) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.hold(); err != nil {
		return 0, err
	}
	defer d.release()
	if err := d.prepare(int64(len(p))); err != nil {
		return 0, err
	}
	n, err := d.file.Write(p)
	d.size += int64(n)
	if err != nil {
		return n, fmt.Errorf("write the daemon log: %w", err)
	}
	return n, nil
}

func (d *daemonLog) hold() error {
	if d.lock != nil {
		return lockDaemon(d.lock)
	}
	file, err := os.OpenFile(d.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open the daemon log lock: %w", err)
	}
	if err := lockDaemon(file); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}
	d.lock = file
	return nil
}

func (d *daemonLog) release() {
	if d.lock == nil {
		return
	}
	if err := unlockDaemon(d.lock); err != nil {
		return
	}
}

func (d *daemonLog) prepare(next int64) error {
	now := d.now()
	day := now.Format("20060102")
	switch {
	case d.file == nil:
		if err := d.archiveStale(now); err != nil {
			return err
		}
		return d.open(now)
	case !sameOpenFile(d.file, d.path):
		if err := d.file.Close(); err != nil {
			return err
		}
		d.file = nil
		return d.open(now)
	case d.day == day && d.size+next <= d.maxBytes:
		return nil
	default:
		if err := d.file.Close(); err != nil {
			return err
		}
		d.file = nil
		if d.size > 0 {
			if err := d.archive(d.day); err != nil {
				return err
			}
		}
		return d.open(now)
	}
}

func (d *daemonLog) archiveStale(now time.Time) error {
	info, err := os.Stat(d.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	day := info.ModTime().Format("20060102")
	if day == now.Format("20060102") && info.Size() < d.maxBytes {
		return nil
	}
	return d.archive(day)
}

func (d *daemonLog) archive(day string) error {
	info, err := os.Stat(d.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	destination := d.rotatedPath(day)
	if err := gzipFile(d.path, destination); err != nil {
		return err
	}
	return pruneAged(d.dir, "daemon.log-", d.keep, d.now())
}

func (d *daemonLog) rotatedPath(day string) string {
	base := "daemon.log-" + day
	for n := 0; ; n++ {
		name := base + ".gz"
		if n > 0 {
			name = fmt.Sprintf("%s-%d.gz", base, n+1)
		}
		path := filepath.Join(d.dir, name)
		if _, err := os.Stat(path); err != nil {
			return path
		}
		if n == 100 {
			return filepath.Join(d.dir, fmt.Sprintf("%s-%d.gz", base, d.now().UnixNano()))
		}
	}
}

func (d *daemonLog) open(now time.Time) error {
	file, err := os.OpenFile(d.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the daemon log: %w", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	d.file = file
	d.size = info.Size()
	d.day = now.Format("20060102")
	return nil
}

func sameOpenFile(file *os.File, path string) bool {
	openInfo, err := file.Stat()
	if err != nil {
		return false
	}
	pathInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return os.SameFile(openInfo, pathInfo)
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("read the daemon log for rotation: %w", err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create the rotated daemon log: %w", err)
	}
	if err := compressLog(in, out, dst); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("compress the daemon log: %w", err)
	}
	if err := os.Remove(src); err != nil {
		return fmt.Errorf("remove the rotated daemon log: %w", err)
	}
	return nil
}

func compressLog(in, out *os.File, dst string) error {
	compressed := gzip.NewWriter(out)
	if _, err := io.Copy(compressed, in); err != nil {
		_ = compressed.Close()
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("compress the daemon log: %w", err)
	}
	if err := compressed.Close(); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("compress the daemon log: %w", err)
	}
	return nil
}

func pruneAged(dir, prefix string, keep time.Duration, now time.Time) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	cutoff := now.Add(-keep)
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || name == daemonLogName {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// CreateStartupLog opens a new timestamped boot transcript in the state
// directory's logs folder and drops transcripts older than a fortnight.
func CreateStartupLog(home string) (string, *os.File, error) {
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, fmt.Errorf("create the startup log: %w", err)
	}
	if err := pruneAged(dir, "startup-", startupLogKeep, time.Now()); err != nil {
		return "", nil, err
	}
	stamp := time.Now().Format("20060102-150405")
	for n := 0; n < 100; n++ {
		name := "startup-" + stamp + ".log"
		if n > 0 {
			name = fmt.Sprintf("startup-%s-%d.log", stamp, n+1)
		}
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create the startup log: %w", err)
		}
		return path, file, nil
	}
	return "", nil, fmt.Errorf("create the startup log: %w", os.ErrExist)
}

// AppendStartupLog adds one line to a boot transcript. An empty path is a
// start that never opened a file.
func AppendStartupLog(path, line string) error {
	if path == "" {
		return nil
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("open the startup log: %w", err)
	}
	if _, err := fmt.Fprintln(file, line); err != nil {
		_ = file.Close()
		return fmt.Errorf("write the startup log: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("write the startup log: %w", err)
	}
	return nil
}
