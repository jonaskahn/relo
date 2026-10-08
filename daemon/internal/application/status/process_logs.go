// Process logs reads the files the daemon writes about itself: the
// current JSONL log and the boot transcripts of recent starts. The console
// shows them beside the request log, so an operator never opens the state
// directory by hand.
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrInvalidLogQuery reports a log read no caller can run: an unknown
	// level, a cursor that is not a byte offset, or a transcript name that
	// does not name a transcript.
	ErrInvalidLogQuery = errors.New("invalid log query")
	// ErrLogNotFound reports a transcript no start left behind.
	ErrLogNotFound = errors.New("log not found")
)

const (
	// StartupStarted is a boot that reached serving.
	StartupStarted = "started"
	// StartupFailed is a boot that left an error instead.
	StartupFailed = "failed"
)

const (
	processLogDir   = "logs"
	daemonLogName   = "daemon.log"
	startupPrefix   = "startup-"
	startupSuffix   = ".log"
	daemonPageSize  = 200
	daemonPageMax   = 500
	daemonReadChunk = 64 << 10
	daemonTailCap   = 4 << 20
	startupReadCap  = 64 << 10
)

// DaemonLogQuery is one page request against the daemon's own log. Before
// names the byte offset older lines start from, After names the offset newer
// lines start from, Level is the lowest severity to keep, and HideRequests
// drops the per-request access lines.
type DaemonLogQuery struct {
	Limit        int
	Before       string
	After        string
	Level        string
	HideRequests bool
}

// DaemonLine is one parsed log line. Offset is the byte offset the line
// starts at, which is what the next page continues from.
type DaemonLine struct {
	Offset      int64
	TimestampMs int64
	Level       string
	Message     string
	Detail      string
}

// DaemonPage is one page of the daemon log, newest first. NextCursor pages
// backwards and is empty at the oldest line; End is the file size the read
// saw, which a live tail passes back as After.
type DaemonPage struct {
	Lines      []DaemonLine
	NextCursor string
	End        string
}

// StartupEntry is one boot transcript.
type StartupEntry struct {
	Name      string
	StartedAt time.Time
	Outcome   string
	Error     string
	Current   bool
	Size      int64
}

// StartupTranscript is one boot transcript with its lines in file order.
type StartupTranscript struct {
	Entry     StartupEntry
	Lines     []DaemonLine
	Truncated bool
}

// DaemonLogs returns one page of the daemon log, newest first. A missing log
// reads as an empty page: the daemon simply has not written one yet.
func (s *Service) DaemonLogs(ctx context.Context, query DaemonLogQuery) (DaemonPage, error) {
	_ = ctx
	parsed, err := parseDaemonQuery(query)
	if err != nil {
		return DaemonPage{}, err
	}
	if s.home == "" {
		return DaemonPage{}, nil
	}
	file, size, empty, err := openDaemonLog(s.home)
	if err != nil {
		return DaemonPage{}, err
	}
	if file == nil {
		if empty {
			return DaemonPage{End: "0"}, nil
		}
		return DaemonPage{}, nil
	}
	defer func() { _ = file.Close() }()
	if parsed.after > 0 && parsed.after < size {
		return readAfterLines(file, size, parsed, query)
	}
	from := size
	if parsed.before > 0 && parsed.before < size {
		from = parsed.before
	}
	return readBeforeLines(file, size, from, parsed, query)
}

func openDaemonLog(home string) (*os.File, int64, bool, error) {
	path := filepath.Join(home, processLogDir, daemonLogName)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	if info.Size() == 0 {
		return nil, 0, true, nil
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	return file, info.Size(), false, nil
}

// Startups lists every boot transcript, newest first. A missing logs
// directory reads as an empty list.
func (s *Service) Startups(ctx context.Context) ([]StartupEntry, error) {
	_ = ctx
	entries := []StartupEntry{}
	if s.home == "" {
		return entries, nil
	}
	dir := filepath.Join(s.home, processLogDir)
	files, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return entries, nil
		}
		return nil, err
	}
	for _, file := range files {
		if file.IsDir() || !isStartupName(file.Name()) {
			continue
		}
		entry, err := describeStartup(filepath.Join(dir, file.Name()), file.Name(), s.startupLog)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	sortStartups(entries)
	return entries, nil
}

func sortStartups(entries []StartupEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].StartedAt.Equal(entries[j].StartedAt) {
			return entries[i].Name > entries[j].Name
		}
		return entries[i].StartedAt.After(entries[j].StartedAt)
	})
}

// StartupTranscript reads one boot transcript in full. The name has to be a
// transcript file name, never a path, so a request cannot climb out of the
// logs directory.
func (s *Service) StartupTranscript(ctx context.Context, name string) (StartupTranscript, error) {
	_ = ctx
	if !isStartupName(name) || name != filepath.Base(name) {
		return StartupTranscript{}, fmt.Errorf("%w: %q", ErrInvalidLogQuery, name)
	}
	if s.home == "" {
		return StartupTranscript{}, ErrLogNotFound
	}
	path := filepath.Join(s.home, processLogDir, name)
	content, truncated, err := readCappedLines(path, startupReadCap)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StartupTranscript{}, ErrLogNotFound
		}
		return StartupTranscript{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return StartupTranscript{}, ErrLogNotFound
	}
	entry := startupEntry(name, info, s.startupLog)
	entry.Outcome, entry.Error = startupOutcome(content)
	return StartupTranscript{Entry: entry, Lines: transcriptLines(content), Truncated: truncated}, nil
}

func transcriptLines(content []string) []DaemonLine {
	lines := make([]DaemonLine, 0, len(content))
	for index, raw := range content {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		lines = append(lines, parseDaemonLine(int64(index), raw))
	}
	return lines
}

func daemonRank(level string) int {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return 2
	case "warning", "warn":
		return 1
	default:
		return 0
	}
}

type parsedDaemonQuery struct {
	limit  int
	before int64
	after  int64
	rank   int
}

func parseDaemonQuery(query DaemonLogQuery) (parsedDaemonQuery, error) {
	parsed := parsedDaemonQuery{limit: daemonPageSize}
	if query.Limit > 0 {
		parsed.limit = query.Limit
	}
	if parsed.limit > daemonPageMax {
		parsed.limit = daemonPageMax
	}
	rank, err := daemonQueryRank(query.Level)
	if err != nil {
		return parsedDaemonQuery{}, err
	}
	parsed.rank = rank
	if err := parseDaemonCursors(query, &parsed); err != nil {
		return parsedDaemonQuery{}, err
	}
	return parsed, nil
}

func daemonQueryRank(level string) (int, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", "info":
		return 0, nil
	case "warning", "warn":
		return 1, nil
	case "error":
		return 2, nil
	default:
		return 0, fmt.Errorf("%w: level %q", ErrInvalidLogQuery, level)
	}
}

func parseDaemonCursors(query DaemonLogQuery, parsed *parsedDaemonQuery) error {
	for _, cursor := range []struct {
		raw   string
		apply func(int64)
	}{
		{query.Before, func(offset int64) { parsed.before = offset }},
		{query.After, func(offset int64) { parsed.after = offset }},
	} {
		if cursor.raw == "" {
			continue
		}
		offset, err := strconv.ParseInt(cursor.raw, 10, 64)
		if err != nil || offset < 0 {
			return fmt.Errorf("%w: cursor %q", ErrInvalidLogQuery, cursor.raw)
		}
		cursor.apply(offset)
	}
	return nil
}

func readBeforeLines(file *os.File, size, from int64, parsed parsedDaemonQuery, query DaemonLogQuery) (DaemonPage, error) {
	scan := &beforeScan{file: file, size: size, from: from, parsed: parsed, query: query}
	if err := scan.run(); err != nil {
		return DaemonPage{}, err
	}
	return scan.page(), nil
}

type beforeScan struct {
	file   *os.File
	size   int64
	from   int64
	parsed parsedDaemonQuery
	query  DaemonLogQuery
	lines  []DaemonLine
	next   string
}

func (s *beforeScan) page() DaemonPage {
	return DaemonPage{Lines: s.lines, NextCursor: s.next, End: strconv.FormatInt(s.size, 10)}
}

func (s *beforeScan) run() error {
	lines := []DaemonLine{}
	position := s.from
	carry := []byte{}
	first := true
	for position > 0 && len(lines) < s.parsed.limit {
		var head []byte
		var err error
		lines, head, position, first, err = s.scanChunk(lines, carry, position, first)
		if err != nil {
			return err
		}
		carry = head
	}
	s.lines = lines
	if len(lines) == s.parsed.limit && lines[len(lines)-1].Offset != 0 {
		// The limit stopped the scan while older lines remain: the oldest
		// line kept names where the next page continues. A first line kept
		// means the whole file was read, so there is no next page.
		s.next = strconv.FormatInt(lines[len(lines)-1].Offset, 10)
	}
	return nil
}

func (s *beforeScan) scanChunk(lines []DaemonLine, carry []byte, position int64, first bool) ([]DaemonLine, []byte, int64, bool, error) {
	start := position - daemonReadChunk
	if start < 0 {
		start = 0
	}
	chunk := make([]byte, position-start)
	if _, err := s.file.ReadAt(chunk, start); err != nil {
		return lines, carry, position, first, err
	}
	data := append(chunk, carry...)
	kept, offsets, head := splitData(data, start, start > 0, first && !endsNewline(data, position == s.size))
	for i := len(kept) - 1; i >= 0 && len(lines) < s.parsed.limit; i-- {
		if line, ok := keepDaemonLine(offsets[i], string(kept[i]), s.parsed, s.query); ok {
			lines = append(lines, line)
		}
	}
	return lines, head, start, false, nil
}

func splitData(data []byte, base int64, dropHead, dropTail bool) ([][]byte, []int64, []byte) {
	segments := splitNewlines(data)
	head := []byte{}
	if dropHead && len(segments) > 0 {
		head = segments[0]
		segments = segments[1:]
	}
	if dropTail && len(segments) > 0 {
		segments = segments[:len(segments)-1]
	}
	offsets := make([]int64, len(segments))
	running := base
	if dropHead {
		running += int64(len(head) + 1)
	}
	for i, segment := range segments {
		offsets[i] = running
		running += int64(len(segment) + 1)
	}
	return segments, offsets, head
}

func endsNewline(data []byte, atEnd bool) bool {
	return atEnd && len(data) > 0 && data[len(data)-1] == '\n'
}

func splitNewlines(data []byte) [][]byte {
	segments := [][]byte{}
	start := 0
	for i, b := range data {
		if b == '\n' {
			segments = append(segments, data[start:i])
			start = i + 1
		}
	}
	return append(segments, data[start:])
}

func readAfterLines(file *os.File, size int64, parsed parsedDaemonQuery, query DaemonLogQuery) (DaemonPage, error) {
	start := parsed.after - daemonReadChunk
	if start < 0 {
		start = 0
	}
	if size-start > daemonTailCap+daemonReadChunk {
		start = size - daemonTailCap
	}
	window := make([]byte, size-start)
	if _, err := file.ReadAt(window, start); err != nil {
		return DaemonPage{}, err
	}
	// The window opens before After so a line torn by the previous read is
	// whole here; only lines ending past After are new.
	kept, offsets, _ := splitData(window, start, start > 0, !endsNewline(window, true))
	lines := collectAfterLines(kept, offsets, parsed, query)
	if len(lines) > parsed.limit {
		lines = lines[len(lines)-parsed.limit:]
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return DaemonPage{Lines: lines, End: strconv.FormatInt(size, 10)}, nil
}

func collectAfterLines(kept [][]byte, offsets []int64, parsed parsedDaemonQuery, query DaemonLogQuery) []DaemonLine {
	lines := make([]DaemonLine, 0, parsed.limit)
	for i, segment := range kept {
		if offsets[i]+int64(len(segment)+1) <= parsed.after {
			continue
		}
		if line, ok := keepDaemonLine(offsets[i], string(segment), parsed, query); ok {
			lines = append(lines, line)
		}
	}
	return lines
}

func keepDaemonLine(offset int64, raw string, parsed parsedDaemonQuery, query DaemonLogQuery) (DaemonLine, bool) {
	if strings.TrimSpace(raw) == "" {
		return DaemonLine{}, false
	}
	line := parseDaemonLine(offset, raw)
	if daemonRank(line.Level) < parsed.rank {
		return DaemonLine{}, false
	}
	if query.HideRequests && line.Message == "http request" {
		return DaemonLine{}, false
	}
	return line, true
}

func parseDaemonLine(offset int64, raw string) DaemonLine {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return DaemonLine{Offset: offset, Level: "error", Message: trimmed}
	}
	var fields map[string]any
	if err := json.Unmarshal([]byte(trimmed), &fields); err != nil {
		return DaemonLine{Offset: offset, Level: "error", Message: trimmed}
	}
	line := DaemonLine{Offset: offset, Level: "info", Message: stringOf(fields["msg"])}
	if level, ok := fields["level"].(string); ok && level != "" {
		line.Level = level
	}
	if stamp, ok := fields["time"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, stamp); err == nil {
			line.TimestampMs = parsed.UnixMilli()
		}
	}
	line.Detail = daemonDetail(fields)
	return line
}

func daemonDetail(fields map[string]any) string {
	if stringOf(fields["msg"]) == "http request" {
		return fmt.Sprintf("%s %s · %v · %s",
			stringOf(fields["method"]), stringOf(fields["path"]),
			fields["status"], daemonDuration(fields["duration_ms"]))
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if key == "msg" || key == "level" || key == "time" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+stringOf(fields[key]))
	}
	return strings.Join(parts, " ")
}

func daemonDuration(value any) string {
	var ms int64
	switch number := value.(type) {
	case float64:
		ms = int64(number)
	case int64:
		ms = number
	case int:
		ms = int64(number)
	case json.Number:
		parsed, _ := number.Int64()
		ms = parsed
	default:
		return stringOf(value)
	}
	if ms >= 1000 {
		return fmt.Sprintf("%.1fs", float64(ms)/1000)
	}
	return fmt.Sprintf("%dms", ms)
}

func stringOf(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprintf("%v", value)
}

func isStartupName(name string) bool {
	if !strings.HasPrefix(name, startupPrefix) || !strings.HasSuffix(name, startupSuffix) {
		return false
	}
	return len(name) > len(startupPrefix)+len(startupSuffix)
}

func describeStartup(path, name, current string) (StartupEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return StartupEntry{}, err
	}
	content, _, err := readCappedLines(path, startupReadCap)
	if err != nil {
		return StartupEntry{}, err
	}
	entry := startupEntry(name, info, current)
	entry.Outcome, entry.Error = startupOutcome(content)
	return entry, nil
}

func startupEntry(name string, info os.FileInfo, current string) StartupEntry {
	entry := StartupEntry{Name: name, StartedAt: info.ModTime(), Size: info.Size(), Outcome: StartupStarted}
	if stamp, ok := startupStamp(name); ok {
		entry.StartedAt = stamp
	}
	entry.Current = current != "" && filepath.Base(current) == name
	return entry
}

func startupStamp(name string) (time.Time, bool) {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(name, startupPrefix), startupSuffix)
	if len(trimmed) < 15 {
		return time.Time{}, false
	}
	parsed, err := time.ParseInLocation("20060102-150405", trimmed[:15], time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func startupOutcome(content []string) (string, string) {
	for _, raw := range content {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "{") {
			continue
		}
		if len(trimmed) > 300 {
			trimmed = trimmed[:300]
		}
		return StartupFailed, trimmed
	}
	return StartupStarted, ""
}

func readCappedLines(path string, cap int64) ([]string, bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = file.Close() }()
	size := info.Size()
	truncated := false
	if size > cap {
		size = cap
		truncated = true
	}
	content := readCappedContent(file, size)
	lines := make([]string, 0, len(content))
	for _, part := range splitNewlines(content) {
		lines = append(lines, string(part))
	}
	return lines, truncated, nil
}

func readCappedContent(file *os.File, size int64) []byte {
	content := make([]byte, size)
	total := 0
	for total < len(content) {
		n, err := file.Read(content[total:])
		total += n
		if err != nil {
			break
		}
	}
	return content[:total]
}
