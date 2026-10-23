package platform

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/quota"
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
