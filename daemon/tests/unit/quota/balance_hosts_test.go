package quota_test

import (
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/quota"
)

func TestBalanceHosts(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		match func(string) bool
	}{
		{"DeepSeek", "https://api.deepseek.com/v1", quota.DeepSeekHost},
		{"TeamoRouter China", "https://api.teamorouter.cn/v1", quota.TeamoRouterHost},
		{"TeamoRouter global", "https://api.teamorouter.com/v1", quota.TeamoRouterHost},
		{"OrcaRouter", "https://api.orcarouter.ai/v1", quota.OrcaRouterHost},
		{"Vercel AI Gateway", "https://ai-gateway.vercel.sh/v1", quota.VercelGatewayHost},
		{"Moonshot global", "https://api.moonshot.ai/v1", quota.MoonshotHost},
		{"Moonshot China", "https://api.moonshot.cn/v1", quota.MoonshotHost},
		{"SiliconFlow global", "https://api.siliconflow.com/v1", quota.SiliconFlowHost},
		{"SiliconFlow China", "https://api.siliconflow.cn/v1", quota.SiliconFlowHost},
		{"Novita", "https://api.novita.ai/openapi/v1", quota.NovitaHost},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !test.match(test.url) || !quota.MeteredAPIKey(test.url) {
				t.Fatalf("%q was not recognized as a metered API key host", test.url)
			}
			if test.match("https://example.com/v1") {
				t.Fatal("an unrelated host matched")
			}
		})
	}
}
