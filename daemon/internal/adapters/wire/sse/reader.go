// Package sse parses and writes server-sent events and relays one stream
// through a codec without ever holding the whole response in memory.
package sse

import (
	"bufio"
	"errors"
	"io"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// ErrEventTooLarge reports an event that exceeds the maximum frame size.
var ErrEventTooLarge = errors.New("SSE event exceeds the maximum size")

const (
	maxEventBytes = 1 << 20
	doneSentinel  = "[DONE]"
)

// Reader parses server-sent events from an io.Reader.
type Reader struct {
	scanner *bufio.Scanner
}

// NewReader returns a reader over the given stream.
func NewReader(r io.Reader) *Reader {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxEventBytes)
	return &Reader{scanner: scanner}
}

// Next returns the next event. It reports io.EOF at the end of the stream
// and when the [DONE] sentinel arrives.
func (r *Reader) Next() (wire.SSEEvent, error) {
	var event wire.SSEEvent
	var data []string
	for r.scanner.Scan() {
		line := r.scanner.Text()
		if line == "" {
			if len(data) == 0 && event.Name == "" && event.ID == "" {
				continue
			}
			return finishEvent(event, data)
		}
		if err := parseLine(line, &event, &data); err != nil {
			return wire.SSEEvent{}, err
		}
	}
	if err := r.scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return wire.SSEEvent{}, ErrEventTooLarge
		}
		return wire.SSEEvent{}, err
	}
	if len(data) > 0 || event.Name != "" || event.ID != "" {
		return finishEvent(event, data)
	}
	return wire.SSEEvent{}, io.EOF
}

func finishEvent(event wire.SSEEvent, data []string) (wire.SSEEvent, error) {
	event.Data = strings.Join(data, "\n")
	if strings.TrimSpace(event.Data) == doneSentinel {
		return wire.SSEEvent{}, io.EOF
	}
	return event, nil
}

func parseLine(line string, event *wire.SSEEvent, data *[]string) error {
	switch {
	case strings.HasPrefix(line, "data:"):
		*data = append(*data, trimField(line, "data:"))
	case strings.HasPrefix(line, "event:"):
		event.Name = trimField(line, "event:")
	case strings.HasPrefix(line, "id:"):
		event.ID = trimField(line, "id:")
	}
	return nil
}

func trimField(line, prefix string) string {
	return strings.TrimPrefix(strings.TrimPrefix(line, prefix), " ")
}
