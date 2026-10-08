package upstream

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/inference"
)

// streamlessCodec implements wire.Codec without streaming, which is what a
// vendor codec without a stream decoder looks like.
type streamlessCodec struct{}

func (streamlessCodec) EncodeRequest(*inference.Request, wire.CodecOpts) (*http.Request, error) {
	return nil, nil
}

func (streamlessCodec) DecodeResponseEvent(wire.SSEEvent) ([]inference.Event, error) {
	return nil, nil
}

func (streamlessCodec) DecodeResponse([]byte) ([]inference.Event, error) {
	return nil, nil
}

// TestStreamlessCodecIsInspectable covers the stream-decoder refusal with
// errors.Is, so a caller can tell an unsupported codec from a failed one.
func TestStreamlessCodecIsInspectable(t *testing.T) {
	if _, err := streamDecoder(streamlessCodec{}); !errors.Is(err, ErrCodecStreamless) {
		t.Fatalf("streamDecoder(streamless) = %v, want ErrCodecStreamless", err)
	}
}

// TestVendorFailureKeepsItsWords covers the vendor-failure wrapper: the client
// reads the vendor's own words, unchanged.
func TestVendorFailureKeepsItsWords(t *testing.T) {
	err := &vendorFailure{message: "overloaded"}
	if err.Error() != "overloaded" {
		t.Fatalf("vendorFailure.Error() = %q, want the vendor message verbatim", err.Error())
	}
}
