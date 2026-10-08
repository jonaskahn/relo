package modelsdev

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonaskahn/relo/internal/catalog"
)

const sampleJSON = `{
  "openai": {
    "id": "openai",
    "name": "OpenAI",
    "npm": "@ai-sdk/openai",
    "api": "https://api.openai.com/v1",
    "models": {
      "gpt-4o": {
        "id": "gpt-4o",
        "name": "GPT-4o",
        "cost": {
          "input": 2.5,
          "output": 10.0,
          "cache_read": 1.25
        },
        "limit": {
          "context": 128000,
          "output": 16384
        },
        "tool_call": true,
        "reasoning": false
      }
    }
  },
  "anthropic": {
    "id": "anthropic",
    "name": "Anthropic",
    "npm": "@ai-sdk/anthropic",
    "models": {
      "claude-3-5-sonnet-20241022": {
        "id": "claude-3-5-sonnet-20241022",
        "name": "Claude 3.5 Sonnet",
        "cost": {
          "input": 3.0,
          "output": 15.0
        }
      }
    }
  }
}`

func TestParseAndMatch(t *testing.T) {
	idx, err := parseIndex([]byte(sampleJSON))
	if err != nil {
		t.Fatalf("parseIndex() error: %v", err)
	}

	if len(idx.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(idx.Providers))
	}

	// 1. Exact match
	m, matchType, ok := idx.Match("openai", "gpt-4o")
	if !ok || matchType != MatchExact || m.ID != "gpt-4o" {
		t.Fatalf("expected exact match, got %v, %v, %v", ok, matchType, m)
	}
	if m.Prices.Input == nil || *m.Prices.Input != 2500000 {
		t.Errorf("expected input price 2500000, got %v", m.Prices.Input)
	}

	// 2. Normalized match (with models/ prefix)
	m, matchType, ok = idx.Match("openai", "models/gpt-4o")
	if !ok || matchType != MatchNormalized {
		t.Fatalf("expected normalized match, got %v, %v", ok, matchType)
	}

	// 3. Region prefix normalized match
	m, matchType, ok = idx.Match("anthropic", "us.claude-3-5-sonnet-20241022")
	if !ok || matchType != MatchNormalized {
		t.Fatalf("expected normalized match for region prefix, got %v, %v", ok, matchType)
	}

	// 4. Vendor match (from another provider id)
	m, matchType, ok = idx.Match("openrouter", "anthropic/claude-3-5-sonnet-20241022")
	if !ok || matchType != MatchVendor {
		t.Fatalf("expected vendor match, got %v, %v", ok, matchType)
	}

	// 5. Fallback vendor match
	m, matchType, ok = idx.Match("custom-gateway", "gpt-4o")
	if !ok || matchType != MatchVendor {
		t.Fatalf("expected fallback vendor match, got %v, %v", ok, matchType)
	}

	// 6. No match
	_, matchType, ok = idx.Match("openai", "non-existent-model")
	if ok || matchType != MatchNone {
		t.Fatalf("expected no match, got %v, %v", ok, matchType)
	}
}

func TestTokenCountsKeepASuffixAsThousands(t *testing.T) {
	idx, err := parseIndex([]byte(`{
		"openai": {
			"id": "openai",
			"models": {
				"sized": {
					"id": "sized",
					"limit": { "context": "200k", "input": 262144, "output": "16k" },
					"max_output": "1.5M"
				}
			}
		}
	}`))
	if err != nil {
		t.Fatalf("parseIndex() error: %v", err)
	}
	model := idx.Providers["openai"].Models["sized"]
	if model.ContextWindow == nil || *model.ContextWindow != 200_000 {
		t.Fatalf("context = %v, want 200000", model.ContextWindow)
	}
	if model.MaxInput == nil || *model.MaxInput != 262_000 {
		t.Fatalf("max input = %v, want 262000", model.MaxInput)
	}
	if model.MaxOutput == nil || *model.MaxOutput != 16_000 {
		t.Fatalf("max output = %v, want the limit output 16000, not the sibling 1.5M", model.MaxOutput)
	}
}

func TestEffortLadder(t *testing.T) {
	entry := func(raw string) catalog.ModelsDevModel {
		t.Helper()
		idx, err := parseIndex([]byte(fmt.Sprintf(`{"xiaomi": {"id": "xiaomi", "models": {"m": %s}}}`, raw)))
		if err != nil {
			t.Fatalf("parseIndex() error: %v", err)
		}
		return idx.Providers["xiaomi"].Models["m"]
	}

	t.Run("a toggle-only entry states an empty ladder", func(t *testing.T) {
		model := entry(`{"reasoning": true, "reasoning_options": [{"type": "toggle"}]}`)
		if model.ReasoningEfforts == nil || len(model.ReasoningEfforts) != 0 {
			t.Fatalf("ReasoningEfforts = %v, want an empty ladder", model.ReasoningEfforts)
		}
	})

	t.Run("an effort entry states its values", func(t *testing.T) {
		model := entry(`{"reasoning": true, "reasoning_options": [{"type": "effort", "values": ["low", "xhigh"]}]}`)
		if len(model.ReasoningEfforts) != 2 || model.ReasoningEfforts[0] != "low" || model.ReasoningEfforts[1] != "xhigh" {
			t.Fatalf("ReasoningEfforts = %v, want the declared values", model.ReasoningEfforts)
		}
	})

	t.Run("a model without options has no ladder", func(t *testing.T) {
		model := entry(`{"reasoning": true}`)
		if model.ReasoningEfforts != nil {
			t.Fatalf("ReasoningEfforts = %v, want nil", model.ReasoningEfforts)
		}
	})

	t.Run("a budget is not stored as a toggle", func(t *testing.T) {
		model := entry(`{"reasoning": true, "reasoning_options": [{"type": "budget_tokens", "min": 1024, "max": 8192}]}`)
		if model.ReasoningToggle || !model.ReasoningBudget {
			t.Fatalf("toggle %v budget %v, want a budget and no toggle", model.ReasoningToggle, model.ReasoningBudget)
		}
		if model.ReasoningEfforts == nil || len(model.ReasoningEfforts) != 0 {
			t.Fatalf("ReasoningEfforts = %v, want an empty effort list", model.ReasoningEfforts)
		}
		if model.ReasoningBudgetMin == nil || *model.ReasoningBudgetMin != 1024 || model.ReasoningBudgetMax == nil || *model.ReasoningBudgetMax != 8192 {
			t.Fatalf("bounds = %v %v, want 1024 and 8192", model.ReasoningBudgetMin, model.ReasoningBudgetMax)
		}
	})

	t.Run("a model that does not reason has no ladder", func(t *testing.T) {
		model := entry(`{"reasoning": false, "reasoning_options": [{"type": "effort", "values": ["low"]}]}`)
		if model.ReasoningEfforts != nil {
			t.Fatalf("ReasoningEfforts = %v, want nil", model.ReasoningEfforts)
		}
	})
}

func TestDirectory(t *testing.T) {
	etag := "12345"
	requests := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleJSON))
	}))
	defer server.Close()

	tempDir := t.TempDir()
	dir := NewDirectory(tempDir, server.URL, server.Client())

	// Fetch online
	idx, err := dir.Get(context.Background(), ModeOnlineFirst)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if !idx.State.Available || idx.State.Stale {
		t.Errorf("expected available and fresh, got %+v", idx.State)
	}

	// Check cache file created
	cacheData, err := os.ReadFile(filepath.Join(tempDir, "modelsdev.json"))
	if err != nil || len(cacheData) == 0 {
		t.Fatalf("cache file not written: %v", err)
	}

	// Server down -> fallback to cache
	server.Close()
	dirOffline := NewDirectory(tempDir, "http://invalid.local/api.json", &http.Client{})
	idxOffline, err := dirOffline.Get(context.Background(), ModeOnlineFirst)
	if err == nil {
		t.Fatalf("expected error from offline server, got nil")
	}
	if idxOffline == nil || !idxOffline.State.Available || !idxOffline.State.Stale {
		t.Errorf("expected available from stale cache, got %+v", idxOffline.State)
	}
}

// TestFetchFailureStatusIsInspectable covers a failed dataset fetch with
// errors.Is, so a caller can tell a vendor failure from a transport error.
func TestFetchFailureStatusIsInspectable(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(failing.Close)

	dir := NewDirectory(t.TempDir(), failing.URL, failing.Client())
	if _, err := dir.Get(context.Background(), ModeOnlineFirst); !errors.Is(err, ErrFetchStatus) {
		t.Fatalf("Get(failing) error = %v, want ErrFetchStatus", err)
	}
}
