// Stream transforms: frames, closing, and error encoding.
package upstream

import (
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/adapters/wire/sse"
	"github.com/jonaskahn/relo/internal/inference"
)

func transformFrames(inbound wire.InboundCodec, decoder wire.StreamDecoder, collector *usageCollector) sse.Transform {
	return func(event wire.SSEEvent) ([]wire.SSEEvent, error) {
		canonical, err := decoder.Push(event)
		if err != nil {
			return nil, err
		}
		return encodeFrames(inbound, canonical, collector)
	}
}

func closingFrames(inbound wire.InboundCodec, decoder wire.StreamDecoder, collector *usageCollector) sse.OnEOF {
	return func() ([]wire.SSEEvent, error) {
		canonical, err := decoder.Finish()
		if err != nil {
			return nil, err
		}
		return encodeFrames(inbound, canonical, collector)
	}
}

func errorFrames(inbound wire.InboundCodec) sse.ErrorEncoder {
	encoder, ok := inbound.(wire.ErrorEncoder)
	if !ok {
		return nil
	}
	return encoder.EncodeError
}

func encodeFrames(inbound wire.InboundCodec, canonical []inference.Event, collector *usageCollector) ([]wire.SSEEvent, error) {
	frames := make([]wire.SSEEvent, 0, len(canonical))
	for _, event := range canonical {
		collector.observe(event)
		encoded, err := inbound.EncodeResponseEvent(event)
		if err != nil {
			return nil, err
		}
		frames = append(frames, encoded...)
	}
	return frames, nil
}

type usageCollector struct {
	usage   inference.UsageReport
	failure *inference.ErrorInfo
}

func newUsageCollector() *usageCollector {
	return &usageCollector{}
}

func (c *usageCollector) observe(event inference.Event) {
	if event.Kind == inference.EventUsage && event.Usage != nil {
		c.usage = *event.Usage
	}
	if event.Kind == inference.EventError && event.Error != nil {
		c.failure = event.Error
	}
}

func (c *usageCollector) observeAll(events []inference.Event) {
	for _, event := range events {
		c.observe(event)
	}
}

func (c *usageCollector) report() inference.UsageReport {
	return c.usage
}

func (c *usageCollector) failed() *inference.ErrorInfo {
	return c.failure
}

func (c *usageCollector) reported() bool {
	return c.usage.InputTokens+c.usage.OutputTokens+
		c.usage.CacheReadTokens+c.usage.CacheWriteTokens > 0
}
