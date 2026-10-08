// SSE writer: framing events for streaming responses.
package sse

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// ErrNoFlusher reports a response writer that cannot stream.
var ErrNoFlusher = errors.New("ResponseWriter does not implement http.Flusher")

// WriteEvent writes one event and flushes it to the client.
func WriteEvent(w http.ResponseWriter, event wire.SSEEvent) error {
	if err := writeFields(w, event); err != nil {
		return err
	}
	flush(w)
	return nil
}

// WriteEvents writes a batch of events in order.
func WriteEvents(w http.ResponseWriter, events []wire.SSEEvent) error {
	for _, event := range events {
		if err := WriteEvent(w, event); err != nil {
			return err
		}
	}
	return nil
}

// RequireFlusher reports whether the writer can flush each event.
func RequireFlusher(w http.ResponseWriter) error {
	if _, ok := w.(http.Flusher); !ok {
		return ErrNoFlusher
	}
	return nil
}

func writeFields(w http.ResponseWriter, event wire.SSEEvent) error {
	if event.Name != "" {
		if _, err := fmt.Fprintf(w, "event: %s\n", event.Name); err != nil {
			return fmt.Errorf("write SSE event name: %w", err)
		}
	}
	if event.ID != "" {
		if _, err := fmt.Fprintf(w, "id: %s\n", event.ID); err != nil {
			return fmt.Errorf("write SSE event id: %w", err)
		}
	}
	for _, line := range strings.Split(event.Data, "\n") {
		if _, err := fmt.Fprintf(w, "data: %s\n", line); err != nil {
			return fmt.Errorf("write SSE event data: %w", err)
		}
	}
	if _, err := fmt.Fprint(w, "\n"); err != nil {
		return fmt.Errorf("write SSE event terminator: %w", err)
	}
	return nil
}

func flush(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}
