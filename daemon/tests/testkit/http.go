package testkit

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// MockUpstream starts a test server replaying fixture files, keyed by path.
// Fixture values are paths relative to tests/fixtures/.
func MockUpstream(t *testing.T, routes map[string]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	for path, fixture := range routes {
		body := readFixture(t, fixture)
		mux.HandleFunc(path, replay(body, fixture))
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func readFixture(t *testing.T, fixture string) []byte {
	t.Helper()
	body, err := os.ReadFile(FixturePath(fixture))
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture, err)
	}
	return body
}

func replay(body []byte, fixture string) http.HandlerFunc {
	contentType := "application/json"
	if strings.HasSuffix(fixture, ".txt") {
		contentType = "text/event-stream"
	}
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}
}
