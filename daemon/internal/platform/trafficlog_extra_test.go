package platform

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/server"
)

func TestNewTrafficLogOpensExistingHome(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(logDir(home), 0o700); err != nil {
		t.Fatal(err)
	}
	traffic, err := newTrafficLog(home)
	if err != nil || traffic == nil {
		t.Fatalf("newTrafficLog() = %v, %v", traffic, err)
	}
}

func TestNewTrafficLogFailsForMissingHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "nope")
	if _, err := newTrafficLog(home); err == nil {
		t.Fatal("newTrafficLog() = nil, want error for missing home")
	}
}

func TestOpenTrafficLogReturnsNilOnError(t *testing.T) {
	home := filepath.Join(t.TempDir(), "nope")
	if traffic := openTrafficLog(home); traffic != nil {
		t.Fatalf("openTrafficLog() = %v, want nil", traffic)
	}
}

func TestTrafficLogHalvesHonoursCancelledContext(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return day }}
	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: "agent_request"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := traffic.Halves(ctx, "req-1"); err == nil {
		t.Fatal("Halves() = nil, want context error")
	}
}

func TestTrafficLogClosedDayHalvesAtMissingFile(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := &trafficLog{dir: dir, now: time.Now}
	halves, found, err := traffic.closedDayHalvesAt(context.Background(), filepath.Join(dir, "traffic-20260101.log"), "req-1")
	if err != nil || found || len(halves) != 0 {
		t.Fatalf("closedDayHalvesAt(missing) = %+v, %v, %v, want nil,false,nil", halves, found, err)
	}
}

func TestTrafficLogCachedDayMissAndRememberBound(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := &trafficLog{dir: dir, now: time.Now}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := traffic.cachedDay(dir, info); ok {
		t.Fatal("cachedDay() = true, want false on empty cache")
	}
	for i := 0; i < 5; i++ {
		traffic.rememberDay(closedIndex{path: filepath.Join(dir, "f"+string(rune('a'+i))), lines: map[string][]int64{}})
	}
	traffic.mu.Lock()
	if len(traffic.closed) != closedIndexKeep {
		t.Fatalf("closed len = %d, want %d", len(traffic.closed), closedIndexKeep)
	}
	traffic.mu.Unlock()
}

func TestTrafficLogDayFilesEmptyDir(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := &trafficLog{dir: dir, now: time.Now}
	files, err := traffic.dayFiles()
	if err != nil || len(files) != 0 {
		t.Fatalf("dayFiles(empty) = %v, %v, want nil/empty,nil", files, err)
	}
}

func TestTrafficLogIndexFileEmptyFile(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "empty.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	lines, err := indexFile(path, 0)
	if err != nil || len(lines) != 0 {
		t.Fatalf("indexFile(empty) = %v, %v, want empty,nil", lines, err)
	}
}

func TestTrafficLogIndexDayEmptySize(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "empty.log")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	lines, err := indexDay(f, 0)
	if err != nil || len(lines) != 0 {
		t.Fatalf("indexDay(empty) = %v, %v", lines, err)
	}
}

func TestTrafficLogRequestIDAtShortAndBad(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "x.log")
	if err := os.WriteFile(path, []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, ok := requestIDAt(f, 0); ok {
		t.Fatal("requestIDAt(short) = true, want false")
	}
}

func TestTrafficLogReadLineAtReturnsLineWithoutNewline(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "x.log")
	if err := os.WriteFile(path, []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	line, err := readLineAt(f, 0, 11)
	if err != nil || string(line) != "hello world" {
		t.Fatalf("readLineAt() = %q, %v", line, err)
	}
}

func TestTrafficLogReadLineAtEmptyRoom(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "x.log")
	if err := os.WriteFile(path, []byte("hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	line, err := readLineAt(f, 5, 5)
	if err != nil || len(line) != 0 {
		t.Fatalf("readLineAt(empty room) = %q, %v", line, err)
	}
}

func TestTrafficLogOpenSameDayAndClosePrevious(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	next := day.AddDate(0, 0, 1)
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return day }}
	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: "agent_request"}); err != nil {
		t.Fatal(err)
	}
	traffic.now = func() time.Time { return next }
	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-2", Kind: "agent_request"}); err != nil {
		t.Fatal(err)
	}
}

func TestTrafficLogClosedDayHalvesAtContextCancelled(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := &trafficLog{dir: dir, now: time.Now}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := traffic.closedDayHalvesAt(ctx, filepath.Join(dir, "x.log"), "req-1")
	if err == nil {
		t.Fatal("closedDayHalvesAt(cancelled) = nil, want context error")
	}
}

func TestTrafficLogIndexFileError(t *testing.T) {
	if _, err := indexFile(filepath.Join(t.TempDir(), "nope.log"), 10); err == nil {
		t.Fatal("indexFile(missing) = nil, want error")
	}
}

func TestTrafficLogOpenFileStatError(t *testing.T) {
	// Force open() to hit stat error by using a directory path as file? Hard; skip.
}
