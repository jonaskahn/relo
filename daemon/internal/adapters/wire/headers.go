// Outbound header merging for upstream requests.
package wire

import (
	"net/http"
	"strings"
)

var protectedHeaders = map[string]bool{
	"authorization":       true,
	"api-key":             true,
	"x-api-key":           true,
	"host":                true,
	"content-length":      true,
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
}

// MergeHeaders layers the caller's headers over the provider's static
// headers. Matching is case-insensitive, the caller wins, and the caller
// cannot set a header the dispatcher or the transport owns. The result is
// a fresh map; neither input is changed.
func MergeHeaders(static, caller map[string]string) map[string]string {
	merged := make(map[string]string, len(static)+len(caller))
	for name, value := range static {
		merged[name] = value
	}
	for name, value := range caller {
		if protectedHeaders[strings.ToLower(name)] {
			continue
		}
		for existing := range merged {
			if strings.EqualFold(existing, name) {
				delete(merged, existing)
			}
		}
		merged[name] = value
	}
	return merged
}

// ApplyHeaders writes merged headers onto an outbound request.
func ApplyHeaders(request *http.Request, headers map[string]string) {
	for name, value := range headers {
		request.Header.Set(name, value)
	}
}
