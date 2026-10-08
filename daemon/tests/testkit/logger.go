package testkit

import (
	"bytes"
	"log/slog"
	"sync"
	"testing"
)

// SyncBuffer collects log lines that a background worker may still be
// writing while the test reads them back.
type SyncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *SyncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *SyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

// TestLogger returns a logger writing JSON lines into a buffer. It accepts
// a benchmark as well as a test.
func TestLogger(t testing.TB) (*slog.Logger, *SyncBuffer) {
	t.Helper()
	buffer := &SyncBuffer{}
	handler := slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug})
	return slog.New(handler), buffer
}
