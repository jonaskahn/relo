// SSE pipeline: transforms, end-of-stream, and error encoders.
package sse

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/jonaskahn/relo/internal/adapters/wire"
)

// Transform converts one upstream event into the frames the client receives.
type Transform func(wire.SSEEvent) ([]wire.SSEEvent, error)

// OnEOF produces the frames that close a stream which ended without a
// terminal event of its own.
type OnEOF func() ([]wire.SSEEvent, error)

// ErrorEncoder renders a relay failure as client frames.
type ErrorEncoder func(error) []wire.SSEEvent

// RunConfig configures one relay from an upstream stream to a client.
type RunConfig struct {
	Upstream  io.Reader
	Writer    http.ResponseWriter
	Transform Transform
	OnEOF     OnEOF
	OnError   ErrorEncoder
}

// Run relays the upstream stream to the client one event at a time and
// always closes the client stream, even when the upstream fails.
func Run(cfg RunConfig) error {
	if err := RequireFlusher(cfg.Writer); err != nil {
		return err
	}
	reader := NewReader(cfg.Upstream)
	for {
		event, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return closeStream(cfg)
		}
		if err != nil {
			return failStream(cfg, err)
		}
		if err := relay(cfg, event); err != nil {
			return failStream(cfg, err)
		}
	}
}

func relay(cfg RunConfig, event wire.SSEEvent) error {
	if cfg.Transform == nil {
		return WriteEvent(cfg.Writer, event)
	}
	frames, err := cfg.Transform(event)
	if err != nil {
		return err
	}
	return WriteEvents(cfg.Writer, frames)
}

func closeStream(cfg RunConfig) error {
	if cfg.OnEOF == nil {
		return WriteEvents(cfg.Writer, defaultFrames())
	}
	frames, err := cfg.OnEOF()
	if err != nil {
		return failStream(cfg, err)
	}
	return WriteEvents(cfg.Writer, frames)
}

func failStream(cfg RunConfig, cause error) error {
	frames := defaultFramesFor(cause)
	if cfg.OnError != nil {
		frames = cfg.OnError(cause)
	}
	if err := WriteEvents(cfg.Writer, frames); err != nil {
		return fmt.Errorf("write stream error frame: %w", err)
	}
	return nil
}

func defaultFrames() []wire.SSEEvent {
	return []wire.SSEEvent{errorFrame("upstream ended the stream without a terminal event")}
}

func defaultFramesFor(cause error) []wire.SSEEvent {
	return []wire.SSEEvent{errorFrame(cause.Error())}
}

func errorFrame(message string) wire.SSEEvent {
	return wire.SSEEvent{
		Name: "error",
		Data: fmt.Sprintf(`{"error":"stream_error","message":%q}`, message),
	}
}
