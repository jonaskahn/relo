package platform

import (
	"fmt"
	"testing"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/quota"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/clock"
)

const deepSeekAccountBase = "https://api.deepseek.com/anthropic/v1"

// A DeepSeek account connection and the DeepSeek API-key connection share one
// host, so the host alone would send the account's grant to the key-balance
// endpoint. The account template is matched first for exactly that pair.
func TestQuotaProberSeparatesTheDeepSeekAccountFromTheAPIKey(t *testing.T) {
	t.Run("the API key keeps the balance prober", func(t *testing.T) {
		prober, found := apiKeyBalanceProber(deepSeekAccountBase)
		if !found {
			t.Fatal("no balance prober for the DeepSeek API key")
		}
		if _, ok := prober.(*quota.DeepSeekProber); !ok {
			t.Fatalf("prober = %T, want the balance prober", prober)
		}
	})

	// A per-credential refresh reaches the same prober the background worker
	// holds, so it also has to answer with the account prober.
	t.Run("the account resolves to its own prober", func(t *testing.T) {
		prober, found := namedQuotaProber(deepSeekAccountTemplate, deepSeekAccountBase, clock.New())
		if !found {
			t.Fatal("the resolver named no prober for the account")
		}
		if _, ok := prober.(*quota.DeepSeekAccountProber); !ok {
			t.Fatalf("prober = %T, want the account prober", prober)
		}
	})
}

// hostQuotaProber picks the prober for the connection's own row, so the
// template decides before the host does. Each case is a lane the worker has to
// answer with the prober that speaks it.
func TestHostQuotaProber(t *testing.T) {
	clk := clock.New()
	cases := []struct {
		name     string
		host     catalog.Provider
		want     activity.QuotaProber
		wantNone bool
	}{
		{
			name: "the DeepSeek account is matched by template, not host",
			host: catalog.Provider{
				ID: "dp-account", TemplateID: deepSeekAccountTemplate, BaseURL: deepSeekAccountBase,
			},
			want: new(quota.DeepSeekAccountProber),
		},
		{
			name: "the DeepSeek API key keeps the balance prober on the same host",
			host: catalog.Provider{ID: "dp-key", TemplateID: "deepseek", BaseURL: deepSeekAccountBase},
			want: new(quota.DeepSeekProber),
		},
		{
			name: "the OpenCode Go gateway is matched by path",
			host: catalog.Provider{ID: "oc", BaseURL: "https://opencode.ai/inference/go"},
			want: new(quota.OpenCodeGoProber),
		},
		{
			name: "OpenRouter is matched by host",
			host: catalog.Provider{ID: "or", BaseURL: "https://openrouter.ai/api/v1"},
			want: new(quota.OpenRouterProber),
		},
		{
			name: "Z.ai is matched by host",
			host: catalog.Provider{ID: "zai", BaseURL: "https://api.z.ai/api/paas/v4"},
			want: new(quota.ZAIProber),
		},
		{
			name: "a sign-in template on its own host falls through to the template",
			host: catalog.Provider{ID: "claude", TemplateID: claudeTemplate, BaseURL: "https://api.anthropic.com"},
			want: new(quota.ClaudeProber),
		},
		{
			name:     "a connection no lane speaks has no prober",
			host:     catalog.Provider{ID: "local", TemplateID: "ollama", BaseURL: "http://localhost:11434"},
			wantNone: true,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			prober, found := hostQuotaProber(tt.host, clk)
			if tt.wantNone {
				if found {
					t.Fatalf("hostQuotaProber() = %T, want no prober", prober)
				}
				return
			}
			if !found {
				t.Fatal("hostQuotaProber() found no prober")
			}
			// The concrete type is what makes two lanes distinguishable when
			// they share a host or an endpoint.
			if got, want := fmt.Sprintf("%T", prober), fmt.Sprintf("%T", tt.want); got != want {
				t.Fatalf("hostQuotaProber() = %s, want %s", got, want)
			}
		})
	}
}
