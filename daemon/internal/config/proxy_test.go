package config

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestProxyURLRejectsANonHTTPAddress(t *testing.T) {
	if err := ValidateProxyURL(""); err != nil {
		t.Fatalf("empty URL error = %v", err)
	}
	if err := ValidateProxyURL("http://127.0.0.1:7890"); err != nil {
		t.Fatalf("http URL error = %v", err)
	}
	if err := ValidateProxyURL("socks5://127.0.0.1:1080"); err == nil {
		t.Fatal("a socks URL was accepted")
	}
}

func TestUpdateProxyURLRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("autostart = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := UpdateProxyURL(path, "http://127.0.0.1:7890"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadProxyURL(path)
	if err != nil || got != "http://127.0.0.1:7890" {
		t.Fatalf("ReadProxyURL() = %q, %v", got, err)
	}
	if err := UpdateProxyURL(path, ""); err != nil {
		t.Fatal(err)
	}
	got, err = ReadProxyURL(path)
	if err != nil || got != "" {
		t.Fatalf("ReadProxyURL() = %q, %v", got, err)
	}
}

func TestOutboundTransportUsesTheProxyOnTheRequest(t *testing.T) {
	var throughProxy, direct bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		throughProxy = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		direct = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	client := &http.Client{Transport: OutboundTransport()}
	marked, err := http.NewRequest(http.MethodGet, upstream.URL+"/marked", nil)
	if err != nil {
		t.Fatal(err)
	}
	marked = marked.WithContext(WithOutboundProxy(marked.Context(), proxy.URL))
	response, err := client.Do(marked)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if !throughProxy {
		t.Fatal("the marked request did not reach the proxy")
	}

	direct = false
	throughProxy = false
	plain, err := http.NewRequest(http.MethodGet, upstream.URL+"/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = client.Do(plain)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if !direct || throughProxy {
		t.Fatalf("direct = %v, proxy = %v", direct, throughProxy)
	}
}
