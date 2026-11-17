package dispatch_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAProxyStatusIsRetryableThroughAnotherProxy pins how a status the proxy
// itself answered is classified: the vendor never saw the request, so the
// attempt is a proxy failure that another proxy may serve.
func TestAProxyStatusIsRetryableThroughAnotherProxy(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusProxyAuthRequired)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"proxy authentication required"}}`)
	}))
	t.Cleanup(origin.Close)

	recorder := httptest.NewRecorder()
	result, err := newExecutor(t).Execute(context.Background(), completeExchange(origin.URL), recorder)
	if err == nil {
		t.Fatal("Execute() error = nil, want the refusal")
	}
	if !result.ProxyFailure {
		t.Fatalf("result = %+v, want it reported as a proxy failure", result)
	}
	if !result.Retryable {
		t.Fatalf("result = %+v, want another proxy to be able to serve it", result)
	}
	if result.Status != http.StatusProxyAuthRequired {
		t.Fatalf("status = %d, want %d", result.Status, http.StatusProxyAuthRequired)
	}
}
