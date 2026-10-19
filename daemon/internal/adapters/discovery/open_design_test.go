package discovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

const openDesignRoster = `{
  "data": [
    {
      "id": "image-model",
      "architecture": {"input_modalities": ["text", "image", "file"]},
      "metadata": {"context_limit": 1000000, "context_budget": 228000, "output_budget": 128000}
    },
    {
      "id": "text-model",
      "architecture": {"input_modalities": ["text"]},
      "metadata": {"context_limit": 1048000, "output_budget": 32000}
    },
    {"id": "plain-model"},
    {
      "id": "standard-first",
      "context_window": 200000,
      "max_output_tokens": 8000,
      "metadata": {"context_limit": 1000000, "output_budget": 128000}
    },
    {
      "id": "zero-limits",
      "metadata": {"context_limit": 0, "output_budget": 0}
    }
  ]
}`

func TestListOpenDesignReadsVendorLimitsAndVision(t *testing.T) {
	models := openDesignModels(t)
	if len(models) != 5 {
		t.Fatalf("listed = %d models, want 5", len(models))
	}

	image := models[0]
	assertInt(t, "image context", image.ContextWindow, 1_000_000)
	assertInt(t, "image output", image.MaxOutput, 128_000)
	assertBool(t, "image vision", image.SupportsVision, true)

	text := models[1]
	assertInt(t, "text context", text.ContextWindow, 1_048_000)
	assertInt(t, "text output", text.MaxOutput, 32_000)
	assertBool(t, "text vision", text.SupportsVision, false)

	plain := models[2]
	if plain.ContextWindow != nil || plain.MaxOutput != nil || plain.SupportsVision != nil {
		t.Errorf("plain model = %+v, want unstated limits and vision", plain)
	}

	standard := models[3]
	assertInt(t, "standard context", standard.ContextWindow, 200_000)
	assertInt(t, "standard output", standard.MaxOutput, 8_000)

	zero := models[4]
	if zero.ContextWindow != nil || zero.MaxOutput != nil {
		t.Errorf("zero limits = %+v, want them unstated", zero)
	}

	if image.Prices != nil {
		t.Errorf("prices = %+v, want the vendor's absent prices", image.Prices)
	}
}

func openDesignModels(t *testing.T) []Listed {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(openDesignRoster))
	}))
	t.Cleanup(server.Close)

	models, err := New(server.Client()).List(context.Background(), Target{
		Format: catalog.ModelsOpenAI, BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("List(openai) error = %v", err)
	}
	return models
}

func assertInt(t *testing.T, name string, got *int64, want int64) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %d", name, got, want)
	}
}

func assertBool(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil || *got != want {
		t.Errorf("%s = %v, want %t", name, got, want)
	}
}
