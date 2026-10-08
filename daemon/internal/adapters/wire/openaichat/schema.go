// DeepSeek's chat API refuses a function schema whose pattern bans a null
// byte, so those patterns are removed on the way out.
package openaichat

import (
	"encoding/json"
	"net/url"
	"strings"
)

func deepSeekAPI(baseURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "api.deepseek.com" || strings.HasSuffix(host, ".api.deepseek.com")
}

func dropNullBytePatterns(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return raw
	}
	cleaned, err := json.Marshal(withoutNullBytePatterns(decoded))
	if err != nil {
		return raw
	}
	return cleaned
}

func withoutNullBytePatterns(node any) any {
	switch value := node.(type) {
	case map[string]any:
		if pattern, ok := value["pattern"].(string); ok && nullBytePattern(pattern) {
			delete(value, "pattern")
		}
		for key, child := range value {
			value[key] = withoutNullBytePatterns(child)
		}
		return value
	case []any:
		for i, child := range value {
			value[i] = withoutNullBytePatterns(child)
		}
		return value
	default:
		return node
	}
}

func nullBytePattern(pattern string) bool {
	return strings.Contains(pattern, `\0`) ||
		strings.Contains(pattern, `\u0000`) ||
		strings.Contains(pattern, `\x00`) ||
		strings.ContainsRune(pattern, '\x00')
}
