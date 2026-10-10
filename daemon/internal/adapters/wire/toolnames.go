// Restoring the tool names a lane had to shorten before sending.
package wire

import "github.com/jonaskahn/relo/internal/inference"

// RestoreToolNames returns a codec that reports each tool call under the name
// the client knows, which is the name the request went out under until an
// upstream limit shortened it. A lane that shortened no name passes its codec
// through unchanged, so an ordinary attempt adds no layer.
func RestoreToolNames(module CodecModule, originals map[string]string) CodecModule {
	if len(originals) == 0 {
		return module
	}
	return restoredNames{CodecModule: module, originals: originals}
}

var _ CodecModule = restoredNames{}

type restoredNames struct {
	CodecModule
	originals map[string]string
}

func (r restoredNames) DecodeResponseEvent(event SSEEvent) ([]inference.Event, error) {
	events, err := r.CodecModule.DecodeResponseEvent(event)
	return restoreNames(events, r.originals), err
}

func (r restoredNames) DecodeResponse(body []byte) ([]inference.Event, error) {
	events, err := r.CodecModule.DecodeResponse(body)
	return restoreNames(events, r.originals), err
}

func (r restoredNames) NewStreamDecoder() StreamDecoder {
	return restoredStream{StreamDecoder: r.CodecModule.NewStreamDecoder(), originals: r.originals}
}

var _ StreamDecoder = restoredStream{}

type restoredStream struct {
	StreamDecoder
	originals map[string]string
}

func (r restoredStream) Push(event SSEEvent) ([]inference.Event, error) {
	events, err := r.StreamDecoder.Push(event)
	return restoreNames(events, r.originals), err
}

func (r restoredStream) Finish() ([]inference.Event, error) {
	events, err := r.StreamDecoder.Finish()
	return restoreNames(events, r.originals), err
}

func restoreNames(events []inference.Event, originals map[string]string) []inference.Event {
	for index := range events {
		call := events[index].ToolCall
		if call == nil {
			continue
		}
		if name, found := originals[call.Name]; found {
			call.Name = name
		}
	}
	return events
}
