package platform

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewLoggerWritesJSONL(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewLogger(dir, slog.LevelInfo, "")
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.Info("relo serving", "addr", "127.0.0.1:1")
	FinishStartupLog()

	content, err := os.ReadFile(DaemonLogPath(dir))
	if err != nil {
		t.Fatalf("read daemon log: %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(content), &record); err != nil {
		t.Fatalf("log = %s: %v", content, err)
	}
	if record["msg"] != "relo serving" || record["addr"] != "127.0.0.1:1" || record["level"] != "info" {
		t.Fatalf("record = %#v", record)
	}
}

func TestStartupLogStopsAfterBoot(t *testing.T) {
	dir := t.TempDir()
	startup := filepath.Join(dir, "startup-test.log")
	if err := os.WriteFile(startup, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	logger, err := NewLogger(dir, slog.LevelInfo, startup)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	logger.Info("boot line")
	FinishStartupLog()
	logger.Info("later line")

	content, err := os.ReadFile(startup)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "boot line") || strings.Contains(text, "later line") {
		t.Fatalf("startup log = %s", text)
	}
}

func TestDaemonLogRotatesBySizeAndDay(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	log, err := openDaemonLogAt(dir, 40, daemonLogKeep, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := log.Write([]byte("first line that is long enough to rotate\n")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(24 * time.Hour)
	if _, err := log.Write([]byte("next day\n")); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, "logs", "daemon.log-*.gz"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("rotated = %v", matches)
	}
	file, err := os.Open(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("first line")) {
		t.Fatalf("rotated body = %s", body)
	}
	current, err := os.ReadFile(DaemonLogPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(current, []byte("next day")) {
		t.Fatalf("current = %s", current)
	}
}

func TestCreateStartupLogDropsOldTranscripts(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logs, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(logs, "startup-20000101-000000.log")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-startupLogKeep - time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	path, file, err := CreateStartupLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old transcript stat = %v", err)
	}
	if filepath.Dir(path) != logs || !strings.HasPrefix(filepath.Base(path), "startup-") {
		t.Fatalf("path = %s", path)
	}
}
