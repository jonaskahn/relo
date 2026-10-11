package platform

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/server"
)

func TestTrafficLogWritesOneLinePerRequest(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return day }}

	if err := traffic.Append(server.TrafficRecord{EventID: 7, RequestID: "req-1", Status: 200}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := traffic.Append(server.TrafficRecord{EventID: 8, RequestID: "req-2", Status: 500}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	content := readTrafficLines(t, home, day)
	if len(content) != 2 {
		t.Fatalf("file holds %d lines, want 2", len(content))
	}
	if !strings.Contains(content[0], `"event_id":7`) || !strings.Contains(content[1], `"request_id":"req-2"`) {
		t.Fatalf("lines = %q", content)
	}
}

func TestTrafficLogOpensANewFileEachDay(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 23, 0, 0, 0, time.UTC)
	clock := day
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return clock }}

	if err := traffic.Append(server.TrafficRecord{EventID: 1}); err != nil {
		t.Fatal(err)
	}
	clock = day.AddDate(0, 0, 1)
	if err := traffic.Append(server.TrafficRecord{EventID: 2}); err != nil {
		t.Fatal(err)
	}

	if len(readTrafficLines(t, home, day)) != 1 || len(readTrafficLines(t, home, day.AddDate(0, 0, 1))) != 1 {
		t.Fatal("each day should hold the one line written on it")
	}
}

func TestTrafficLogPrunesTheFortnightAndLeavesTheDaemonLog(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := &trafficLog{dir: dir, now: func() time.Time { return day }}

	aged := filepath.Join(dir, "traffic-20260920"+trafficLogSuffix)
	rotated := filepath.Join(dir, "daemon.log-20260920.gz")
	for _, path := range []string{aged, rotated} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		stale := day.Add(-trafficLogKeep).Add(-time.Hour)
		if err := os.Chtimes(path, stale, stale); err != nil {
			t.Fatal(err)
		}
	}

	if err := traffic.Append(server.TrafficRecord{EventID: 1}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(aged); !os.IsNotExist(err) {
		t.Fatalf("aged traffic file still present: %v", err)
	}
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("rotated daemon log was removed: %v", err)
	}
}

func TestTrafficLogFindsARequestAndReportsAMissingOne(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return day }}
	body := `{"model":"gpt-4o","messages":[]}`

	if err := traffic.Append(server.TrafficRecord{EventID: 41, AgentRequest: &server.TrafficMessage{Body: body}}); err != nil {
		t.Fatal(err)
	}

	record, found, err := traffic.Find(41)
	if err != nil || !found {
		t.Fatalf("Find(41) = %+v, %v, %v", record, found, err)
	}
	if record.AgentRequest.Body != body {
		t.Fatalf("body = %q, want the whole captured body", record.AgentRequest.Body)
	}
	if err := traffic.Append(server.TrafficRecord{EventID: 410, RequestID: "req-410"}); err != nil {
		t.Fatal(err)
	}
	if other, found, err := traffic.Find(41); err != nil || !found || other.RequestID == "req-410" {
		t.Fatalf("Find(41) = %+v, %v, %v, want only event 41", other, found, err)
	}
	if _, found, err := traffic.Find(42); err != nil || found {
		t.Fatalf("Find(42) found = %v, err = %v, want no record", found, err)
	}
}

func TestTrafficLogFindsARequestWentTheTrafficLogHasNotReached(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := &trafficLog{dir: dir, now: time.Now}
	line, err := json.Marshal(server.TrafficRecord{EventID: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "traffic-20260920"+trafficLogSuffix), append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	record, found, err := traffic.Find(3)
	if err != nil || !found {
		t.Fatalf("Find(3) = %+v, %v, %v", record, found, err)
	}
	if record.EventID != 3 {
		t.Fatalf("record = %+v, want the older day's line", record)
	}
}

func readTrafficLines(t *testing.T, home string, day time.Time) []string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(logDir(home), trafficLogPrefix+day.Format("20060102")+trafficLogSuffix))
	if err != nil {
		t.Fatalf("read the traffic file for %s: %v", day.Format("20060102"), err)
	}
	trimmed := strings.TrimSpace(string(content))
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
