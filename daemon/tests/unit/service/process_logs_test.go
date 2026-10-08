package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	appstatus "github.com/jonaskahn/relo/internal/application/status"
)

func writeProcessLog(t *testing.T, home, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, "logs"), 0o700); err != nil {
		t.Fatalf("create the logs directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "logs", name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func processLogHome(t *testing.T) (string, *appstatus.Service) {
	t.Helper()
	home := t.TempDir()
	writeProcessLog(t, home, "daemon.log",
		`{"level":"info","msg":"http request","method":"GET","path":"/healthz","status":200,"duration_ms":0,"time":"2026-10-05T17:35:23.329568+07:00"}`+"\n"+
			`{"level":"warning","msg":"serving stale quota","provider":"deepseek","time":"2026-10-05T17:35:24.100000+07:00"}`+"\n"+
			`{"level":"info","msg":"http request","method":"POST","path":"/v1/chat/completions","status":200,"duration_ms":7585,"time":"2026-10-05T17:35:24.970648+07:00"}`+"\n"+
			`{"level":"error","msg":"upstream attempt failed","provider":"deepseek","status":429,"time":"2026-10-05T17:35:25.100000+07:00"}`+"\n"+
			`{"addr":"127.0.0.1:10101","level":"info","msg":"relo serving","openai":"127.0.0.1:10201","time":"2026-10-05T16:49:24.352649+07:00"}`+"\n")
	return home, appstatus.New(appstatus.Options{Home: home})
}

func TestDaemonLogsNewestFirst(t *testing.T) {
	ctx := context.Background()
	_, reads := processLogHome(t)
	page, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{})
	if err != nil {
		t.Fatalf("DaemonLogs() error = %v", err)
	}
	if len(page.Lines) != 5 {
		t.Fatalf("lines = %d, want 5", len(page.Lines))
	}
	if page.Lines[0].Message != "relo serving" {
		t.Fatalf("newest = %q, want the serving line", page.Lines[0].Message)
	}
	if page.Lines[0].Detail == "" || !strings.Contains(page.Lines[0].Detail, "addr=127.0.0.1:10101") {
		t.Fatalf("detail = %q, want the serving addresses", page.Lines[0].Detail)
	}
	request := page.Lines[1]
	if request.Message != "upstream attempt failed" || request.Level != "error" {
		t.Fatalf("second = %+v, want the failed attempt", request)
	}
	completion := page.Lines[2]
	if completion.Detail != "POST /v1/chat/completions · 200 · 7.6s" {
		t.Fatalf("detail = %q, want the request summary", completion.Detail)
	}
	if page.End == "" || page.End == "0" {
		t.Fatalf("end = %q, want the file size", page.End)
	}
	if page.NextCursor != "" {
		t.Fatalf("next = %q, want empty at the oldest line", page.NextCursor)
	}
}

func TestDaemonLogsLevels(t *testing.T) {
	ctx := context.Background()
	_, reads := processLogHome(t)
	warnings, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{Level: "warning"})
	if err != nil {
		t.Fatalf("DaemonLogs(warning) error = %v", err)
	}
	if len(warnings.Lines) != 2 {
		t.Fatalf("warnings = %d, want the stale quota and the failed attempt", len(warnings.Lines))
	}
	failures, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{Level: "error"})
	if err != nil {
		t.Fatalf("DaemonLogs(error) error = %v", err)
	}
	if len(failures.Lines) != 1 || failures.Lines[0].Message != "upstream attempt failed" {
		t.Fatalf("errors = %+v, want only the failed attempt", failures.Lines)
	}
	if _, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{Level: "verbose"}); !errors.Is(err, appstatus.ErrInvalidLogQuery) {
		t.Fatalf("level verbose error = %v, want ErrInvalidLogQuery", err)
	}
}

func TestDaemonLogsHideRequests(t *testing.T) {
	ctx := context.Background()
	_, reads := processLogHome(t)
	page, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{HideRequests: true})
	if err != nil {
		t.Fatalf("DaemonLogs() error = %v", err)
	}
	if len(page.Lines) != 3 {
		t.Fatalf("lines = %d, want 3 without the access lines", len(page.Lines))
	}
	for _, line := range page.Lines {
		if line.Message == "http request" {
			t.Fatalf("line = %+v, want no access line", line)
		}
	}
}

func TestDaemonLogsPagesBackwards(t *testing.T) {
	ctx := context.Background()
	_, reads := processLogHome(t)
	first, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{Limit: 2})
	if err != nil {
		t.Fatalf("DaemonLogs() error = %v", err)
	}
	if len(first.Lines) != 2 || first.NextCursor == "" {
		t.Fatalf("first = %+v, want 2 lines and a cursor", first)
	}
	second, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{Limit: 2, Before: first.NextCursor})
	if err != nil {
		t.Fatalf("DaemonLogs(before) error = %v", err)
	}
	if len(second.Lines) != 2 {
		t.Fatalf("second = %+v, want the next 2 lines", second)
	}
	seen := map[int64]bool{}
	for _, line := range append(first.Lines, second.Lines...) {
		if seen[line.Offset] {
			t.Fatalf("offset %d twice, want every line once", line.Offset)
		}
		seen[line.Offset] = true
	}
	third, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{Limit: 2, Before: second.NextCursor})
	if err != nil {
		t.Fatalf("DaemonLogs(before) error = %v", err)
	}
	if len(third.Lines) != 1 || third.NextCursor != "" {
		t.Fatalf("third = %+v, want the last line and no cursor", third)
	}
	if _, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{Before: "nope"}); !errors.Is(err, appstatus.ErrInvalidLogQuery) {
		t.Fatalf("cursor error = %v, want ErrInvalidLogQuery", err)
	}
}

func TestDaemonLogsTailsAfter(t *testing.T) {
	ctx := context.Background()
	home, reads := processLogHome(t)
	first, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{})
	if err != nil {
		t.Fatalf("DaemonLogs() error = %v", err)
	}
	writeProcessLog(t, home, "daemon.log",
		`{"level":"info","msg":"http request","method":"GET","path":"/healthz","status":200,"duration_ms":0,"time":"2026-10-05T17:35:23.329568+07:00"}`+"\n"+
			`{"level":"warning","msg":"serving stale quota","provider":"deepseek","time":"2026-10-05T17:35:24.100000+07:00"}`+"\n"+
			`{"level":"info","msg":"http request","method":"POST","path":"/v1/chat/completions","status":200,"duration_ms":7585,"time":"2026-10-05T17:35:24.970648+07:00"}`+"\n"+
			`{"level":"error","msg":"upstream attempt failed","provider":"deepseek","status":429,"time":"2026-10-05T17:35:25.100000+07:00"}`+"\n"+
			`{"addr":"127.0.0.1:10101","level":"info","msg":"relo serving","openai":"127.0.0.1:10201","time":"2026-10-05T16:49:24.352649+07:00"}`+"\n"+
			`{"level":"error","msg":"quota sweep failed","error":"context canceled","time":"2026-10-05T17:35:26.100000+07:00"}`+"\n")
	tail, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{After: first.End})
	if err != nil {
		t.Fatalf("DaemonLogs(after) error = %v", err)
	}
	if len(tail.Lines) != 1 || tail.Lines[0].Message != "quota sweep failed" {
		t.Fatalf("tail = %+v, want only the new line", tail.Lines)
	}
	if tail.End == first.End {
		t.Fatalf("end = %q, want the grown file size", tail.End)
	}
}

func TestDaemonLogsMissingReadsEmpty(t *testing.T) {
	ctx := context.Background()
	reads := appstatus.New(appstatus.Options{Home: t.TempDir()})
	page, err := reads.DaemonLogs(ctx, appstatus.DaemonLogQuery{})
	if err != nil {
		t.Fatalf("DaemonLogs() error = %v", err)
	}
	if len(page.Lines) != 0 {
		t.Fatalf("lines = %+v, want none without a log", page.Lines)
	}
	homeless := appstatus.New(appstatus.Options{})
	if _, err := homeless.DaemonLogs(ctx, appstatus.DaemonLogQuery{}); err != nil {
		t.Fatalf("DaemonLogs() error = %v", err)
	}
	if _, err := homeless.Startups(ctx); err != nil {
		t.Fatalf("Startups() error = %v", err)
	}
	if _, err := homeless.StartupTranscript(ctx, "startup-20261005-164924.log"); !errors.Is(err, appstatus.ErrLogNotFound) {
		t.Fatalf("transcript error = %v, want ErrLogNotFound", err)
	}
}

func TestStartupsListOutcomes(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	current := filepath.Join(home, "logs", "startup-20261005-164924.log")
	writeProcessLog(t, home, "startup-20261005-113533.log",
		`{"addr":"127.0.0.1:10101","level":"info","msg":"relo serving","time":"2026-10-05T11:35:33.376019+07:00"}`+"\n"+
			"relo: context deadline exceeded\n")
	writeProcessLog(t, home, "startup-20261005-164924.log",
		`{"addr":"127.0.0.1:10101","level":"info","msg":"relo serving","time":"2026-10-05T16:49:24.352649+07:00"}`+"\n")
	reads := appstatus.New(appstatus.Options{Home: home, StartupLog: current})
	entries, err := reads.Startups(ctx)
	if err != nil {
		t.Fatalf("Startups() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want 2 transcripts", entries)
	}
	if entries[0].Name != "startup-20261005-164924.log" || !entries[0].Current {
		t.Fatalf("newest = %+v, want the current start first", entries[0])
	}
	if entries[0].Outcome != appstatus.StartupStarted {
		t.Fatalf("outcome = %q, want started", entries[0].Outcome)
	}
	failed := entries[1]
	if failed.Outcome != appstatus.StartupFailed || failed.Error != "relo: context deadline exceeded" {
		t.Fatalf("failed = %+v, want the failure line", failed)
	}
	if failed.Current {
		t.Fatalf("failed current = true, want false")
	}
}

func TestStartupTranscriptReadsInOrder(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	writeProcessLog(t, home, "startup-20261005-113533.log",
		`{"addr":"127.0.0.1:10101","level":"info","msg":"relo serving","time":"2026-10-05T11:35:33.376019+07:00"}`+"\n"+
			"relo: context deadline exceeded\n")
	reads := appstatus.New(appstatus.Options{Home: home})
	transcript, err := reads.StartupTranscript(ctx, "startup-20261005-113533.log")
	if err != nil {
		t.Fatalf("StartupTranscript() error = %v", err)
	}
	if transcript.Entry.Outcome != appstatus.StartupFailed {
		t.Fatalf("outcome = %q, want failed", transcript.Entry.Outcome)
	}
	if len(transcript.Lines) != 2 || transcript.Lines[0].Message != "relo serving" {
		t.Fatalf("lines = %+v, want file order", transcript.Lines)
	}
	if transcript.Lines[1].Level != "error" || transcript.Truncated {
		t.Fatalf("transcript = %+v, want the error tail untruncated", transcript)
	}
	for _, name := range []string{"", "daemon.log", "../daemon.log", "startup-../x.log", "startup.log"} {
		if _, err := reads.StartupTranscript(ctx, name); !errors.Is(err, appstatus.ErrInvalidLogQuery) {
			t.Fatalf("name %q error = %v, want ErrInvalidLogQuery", name, err)
		}
	}
	if _, err := reads.StartupTranscript(ctx, "startup-20261005-000000.log"); !errors.Is(err, appstatus.ErrLogNotFound) {
		t.Fatalf("missing error = %v, want ErrLogNotFound", err)
	}
}
