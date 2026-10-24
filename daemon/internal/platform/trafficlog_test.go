package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/server"
)

func TestTrafficLogWritesOneLinePerHalf(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return day }}
	ordinal := 0

	halves := []server.TrafficHalf{
		{RequestID: "req-1", Timestamp: "2026-10-11T09:00:00Z", Kind: activity.CaptureAgentRequest},
		{RequestID: "req-1", Timestamp: "2026-10-11T09:00:01Z", Kind: activity.CaptureProviderRequest, Ordinal: &ordinal},
		{RequestID: "req-1", Timestamp: "2026-10-11T09:00:02Z", Kind: activity.CaptureProviderResponse, Ordinal: &ordinal, Status: 200},
	}
	for _, half := range halves {
		if err := traffic.AppendHalf(half); err != nil {
			t.Fatalf("AppendHalf() error = %v", err)
		}
	}

	content := readTrafficLines(t, home, day)
	if len(content) != len(halves) {
		t.Fatalf("file holds %d lines, want one per half (%d)", len(content), len(halves))
	}
	if !strings.HasPrefix(content[0], `{"request_id":"req-1",`) {
		t.Fatalf("line = %q, want the request id first", content[0])
	}
	if !strings.Contains(content[1], `"kind":"provider_request"`) || !strings.Contains(content[1], `"ordinal":0`) {
		t.Fatalf("line = %q, want the attempt's request named", content[1])
	}
}

func TestTrafficLogOpensANewFileEachDay(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 23, 0, 0, 0, time.UTC)
	clock := day
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return clock }}

	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: activity.CaptureAgentRequest}); err != nil {
		t.Fatal(err)
	}
	clock = day.AddDate(0, 0, 1)
	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-2", Kind: activity.CaptureAgentRequest}); err != nil {
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

	if err := traffic.AppendHalf(server.TrafficHalf{RequestID: "req-1", Kind: activity.CaptureAgentRequest}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(aged); !os.IsNotExist(err) {
		t.Fatalf("aged traffic file still present: %v", err)
	}
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("rotated daemon log was removed: %v", err)
	}
}

func TestTrafficLogReadsBackTheHalvesOfOneRequest(t *testing.T) {
	home := t.TempDir()
	day := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	traffic := &trafficLog{dir: logDir(home), now: func() time.Time { return day }}
	ordinal := 0
	body := `{"model":"gpt-4o","messages":[]}`

	halves := []server.TrafficHalf{
		{RequestID: "req-41", Kind: activity.CaptureAgentRequest, Message: &server.TrafficMessage{Body: body}},
		{RequestID: "req-other", Kind: activity.CaptureAgentRequest, Message: &server.TrafficMessage{Body: "other"}},
		{RequestID: "req-41", Kind: activity.CaptureProviderResponse, Ordinal: &ordinal, Status: 200},
	}
	for _, half := range halves {
		if err := traffic.AppendHalf(half); err != nil {
			t.Fatal(err)
		}
	}

	found, err := traffic.Halves(context.Background(), "req-41")
	if err != nil || len(found) != 2 {
		t.Fatalf("Halves(req-41) = %+v, %v, want only that request's halves", found, err)
	}
	if found[0].Message == nil || found[0].Message.Body != body {
		t.Fatalf("first half = %+v, want the whole captured body", found[0])
	}
	// The fields the request detail assembles by are what comes back, so a
	// half read here is the half that was written.
	if found[1].Kind != activity.CaptureProviderResponse || found[1].Ordinal == nil ||
		*found[1].Ordinal != ordinal || found[1].Status != 200 {
		t.Fatalf("second half = %+v, want the attempt it was written for", found[1])
	}
	if missing, err := traffic.Halves(context.Background(), "req-42"); err != nil || len(missing) != 0 {
		t.Fatalf("Halves(req-42) = %+v, %v, want no halves", missing, err)
	}
}

func TestTrafficLogReadsBackARequestOlderThanTheNewestFile(t *testing.T) {
	home := t.TempDir()
	dir := logDir(home)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	traffic := &trafficLog{dir: dir, now: time.Now}
	line, err := json.Marshal(server.TrafficHalf{RequestID: "req-3", Kind: activity.CaptureAgentRequest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "traffic-20260920"+trafficLogSuffix), append(line, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	newer := []byte(`{"request_id":"req-9","kind":"agent_request"}` + "\n")
	if err := os.WriteFile(filepath.Join(dir, "traffic-20261011"+trafficLogSuffix), newer, 0o600); err != nil {
		t.Fatal(err)
	}

	halves, err := traffic.Halves(context.Background(), "req-3")
	if err != nil || len(halves) != 1 || halves[0].RequestID != "req-3" {
		t.Fatalf("Halves(req-3) = %+v, %v, want the older day's half", halves, err)
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
