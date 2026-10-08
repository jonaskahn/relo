// Provider host matching for quota endpoints.
package quota

import (
	"net/url"
	"strings"
)

// OpenRouterHost reports whether baseURL is an OpenRouter endpoint.
func OpenRouterHost(baseURL string) bool {
	return hostNamed(baseURL, "openrouter.ai")
}

// ZAIHost reports whether baseURL is a Z.ai or Zhipu endpoint.
func ZAIHost(baseURL string) bool {
	return hostNamed(baseURL, "api.z.ai", "z.ai", "open.bigmodel.cn", "bigmodel.cn")
}

// DeepSeekHost reports whether baseURL is a DeepSeek API endpoint.
func DeepSeekHost(baseURL string) bool {
	return hostNamed(baseURL, "api.deepseek.com")
}

// TeamoRouterHost reports whether baseURL is a TeamoRouter API endpoint.
func TeamoRouterHost(baseURL string) bool {
	return hostNamed(baseURL, "api.teamorouter.cn", "api.teamorouter.com")
}

// OrcaRouterHost reports whether baseURL is an OrcaRouter API endpoint.
func OrcaRouterHost(baseURL string) bool {
	return hostNamed(baseURL, "api.orcarouter.ai")
}

// VercelGatewayHost reports whether baseURL is a Vercel AI Gateway endpoint.
func VercelGatewayHost(baseURL string) bool {
	return hostNamed(baseURL, "ai-gateway.vercel.sh")
}

// MoonshotHost reports whether baseURL is a Moonshot API endpoint.
func MoonshotHost(baseURL string) bool {
	return hostNamed(baseURL, "api.moonshot.ai", "api.moonshot.cn")
}

// SiliconFlowHost reports whether baseURL is a SiliconFlow API endpoint.
func SiliconFlowHost(baseURL string) bool {
	return hostNamed(baseURL, "api.siliconflow.com", "api.siliconflow.cn")
}

// NovitaHost reports whether baseURL is a Novita API endpoint.
func NovitaHost(baseURL string) bool {
	return hostNamed(baseURL, "api.novita.ai")
}

func hostNamed(baseURL string, names ...string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	for _, name := range names {
		if host == name || strings.HasSuffix(host, "."+name) {
			return true
		}
	}
	return false
}

// MeteredAPIKey reports whether an API-key connection publishes a quota
// endpoint Relo can ask, which is what decides whether the key is probed.
func MeteredAPIKey(baseURL string) bool {
	return OpenRouterHost(baseURL) || ZAIHost(baseURL) ||
		DeepSeekHost(baseURL) || TeamoRouterHost(baseURL) ||
		OrcaRouterHost(baseURL) || VercelGatewayHost(baseURL) ||
		MoonshotHost(baseURL) || SiliconFlowHost(baseURL) ||
		NovitaHost(baseURL)
}
