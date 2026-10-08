package formats_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

// TestEveryAdvertisedFormatResolves pins the one thing an operator relies on:
// a format the catalog offers is a format this build can actually speak. Every
// identifier here is persisted in the database, so a rename would strand a
// stored connection.
func TestEveryAdvertisedFormatResolves(t *testing.T) {
	registry := formats.New()
	for _, format := range []catalog.APIFormat{
		catalog.FormatOpenAIChat,
		catalog.FormatOpenAIResp,
		catalog.FormatAnthropic,
		catalog.FormatVertexAnthropic,
		catalog.FormatGemini,
		catalog.FormatVertex,
		catalog.FormatCloudCode,
		catalog.FormatBedrockConverse,
		catalog.FormatKiro,
	} {
		t.Run(string(format), func(t *testing.T) {
			codec, supported := registry.Outbound(format)
			if !supported {
				t.Fatalf("Outbound(%q) reported no codec", format)
			}
			if codec == nil {
				t.Fatalf("Outbound(%q) returned a nil codec for a supported format", format)
			}
			if !registry.Supports(format) {
				t.Fatalf("Supports(%q) = false, want true", format)
			}
			if !registry.Streams(format) {
				t.Fatalf("Streams(%q) = false, want true", format)
			}
		})
	}
}

// TestEverySurfaceResolves pins the inbound half of the registry: a surface
// Relo serves is a surface this build can decode, and nothing else is.
func TestEverySurfaceResolves(t *testing.T) {
	registry := formats.New()
	for _, surface := range []string{inference.SurfaceChatCompletions, inference.SurfaceResponses, inference.SurfaceMessages} {
		t.Run(surface, func(t *testing.T) {
			codec, found := registry.Inbound(surface)
			if !found {
				t.Fatalf("Inbound(%q) reported no codec", surface)
			}
			if codec == nil {
				t.Fatalf("Inbound(%q) returned a nil codec for a served surface", surface)
			}
		})
	}
}

func TestUnknownSurfaceIsRefused(t *testing.T) {
	registry := formats.New()
	if codec, found := registry.Inbound("gemini"); found || codec != nil {
		t.Fatalf("Inbound(gemini) = %v, %v, want no codec", codec, found)
	}
}

func TestAnthropicRequestRecognition(t *testing.T) {
	registry := formats.New()
	anthropic := http.Header{"Anthropic-Version": []string{"2023-06-01"}}
	if !registry.IsAnthropicRequest(anthropic) {
		t.Fatal("IsAnthropicRequest with the version header = false, want true")
	}
	if registry.IsAnthropicRequest(http.Header{}) {
		t.Fatal("IsAnthropicRequest without the header = true, want false")
	}
}

func TestFailureDocumentNamesTheRefusal(t *testing.T) {
	registry := formats.New()
	document := registry.FailureDocument(http.StatusTooManyRequests, "slow down")
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(document, &payload); err != nil {
		t.Fatalf("FailureDocument did not return JSON: %v", err)
	}
	if payload.Error.Type != "rate_limit_error" {
		t.Fatalf("FailureDocument type = %q, want rate_limit_error", payload.Error.Type)
	}
	if payload.Error.Message != "slow down" {
		t.Fatalf("FailureDocument message = %q, want the refusal words", payload.Error.Message)
	}
}

func TestUnsupportedFormatIsRefused(t *testing.T) {
	registry := formats.New()
	if codec, supported := registry.Outbound(catalog.FormatUnsupported); supported || codec != nil {
		t.Fatalf("Outbound(unsupported) = %v, %v, want no codec", codec, supported)
	}
	if registry.Streams(catalog.FormatUnsupported) {
		t.Fatal("Streams(unsupported) = true, want false")
	}
}

// TestGoogleFormatsNameTheirEndpointShape keeps the endpoint shape a format
// speaks out of the connection identifier an operator typed.
func TestGoogleFormatsNameTheirEndpointShape(t *testing.T) {
	registry := formats.New()
	tests := []struct {
		name   string
		format catalog.APIFormat
		want   string
	}{
		{"gemini uses the studio endpoint", catalog.FormatGemini, "ai-studio"},
		{"vertex uses the vertex endpoint", catalog.FormatVertex, "vertex"},
		{"cloud code uses the assist endpoint", catalog.FormatCloudCode, "cloud-code-assist"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := registry.ModeFor(test.format); got != test.want {
				t.Fatalf("ModeFor(%q) = %q, want %q", test.format, got, test.want)
			}
		})
	}
}
