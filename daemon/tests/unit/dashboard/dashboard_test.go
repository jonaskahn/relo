package dashboard_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jonaskahn/relo/internal/dashboard"
)

// build returns a file tree shaped like a compiled console.
func build() fstest.MapFS {
	return fstest.MapFS{
		"index.html":            {Data: []byte("<!doctype html><title>Relo console</title>")},
		"_app/immutable/app.js": {Data: []byte("console.log('relo')")},
		"logo.svg":              {Data: []byte("<svg/>")},
	}
}

func serve(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

func TestConsoleServesTheBuild(t *testing.T) {
	handler := dashboard.New(build(), dashboard.Options{})
	cases := []struct {
		name     string
		target   string
		contains string
		cache    string
	}{
		{"the shell answers the root", "/", "<title>Relo console</title>", "no-cache"},
		{"a client-side route falls back to the shell", "/providers", "<title>Relo console</title>", "no-cache"},
		{"a deep route falls back to the shell", "/logs/deep/link", "<title>Relo console</title>", "no-cache"},
		{"an immutable asset is served with a long cache", "/_app/immutable/app.js", "console.log", "public, max-age=31536000, immutable"},
		{"a static file is served revalidating", "/logo.svg", "<svg/>", "no-cache"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			response := serve(handler, http.MethodGet, testCase.target)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}
			if body := response.Body.String(); !strings.Contains(body, testCase.contains) {
				t.Fatalf("body = %q, want %q", body, testCase.contains)
			}
			if cache := response.Header().Get("Cache-Control"); cache != testCase.cache {
				t.Fatalf("Cache-Control = %q, want %q", cache, testCase.cache)
			}
		})
	}
}

func TestConsoleRefusesWhatItDoesNotHold(t *testing.T) {
	handler := dashboard.New(build(), dashboard.Options{})

	t.Run("a missing immutable asset is a 404", func(t *testing.T) {
		response := serve(handler, http.MethodGet, "/_app/immutable/gone.js")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})

	t.Run("a missing file with an extension is a 404", func(t *testing.T) {
		response := serve(handler, http.MethodGet, "/nope.png")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})

	t.Run("a traversal cannot leave the tree", func(t *testing.T) {
		response := serve(handler, http.MethodGet, "/../secret.txt")
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})

	t.Run("a write is refused", func(t *testing.T) {
		response := serve(handler, http.MethodPost, "/")
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", response.Code)
		}
		if allow := response.Header().Get("Allow"); allow != "GET, HEAD" {
			t.Fatalf("Allow = %q, want GET, HEAD", allow)
		}
	})

	t.Run("a head is answered without a body", func(t *testing.T) {
		response := serve(handler, http.MethodHead, "/")
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		if response.Body.Len() != 0 {
			t.Fatalf("body = %q, want no body for a head", response.Body.String())
		}
	})
}

func TestConsoleWithoutAShell(t *testing.T) {
	handler := dashboard.New(fstest.MapFS{}, dashboard.Options{})
	response := serve(handler, http.MethodGet, "/")
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", response.Code)
	}
	if body := response.Body.String(); !strings.Contains(body, "no dashboard") {
		t.Fatalf("body = %q, want the unbuilt report", body)
	}
}

// TestHandlerMatchesTheBuild covers both shapes of the embedded console: a
// binary with a synced console serves it, and one without reports 501.
func TestHandlerMatchesTheBuild(t *testing.T) {
	handler := dashboard.Handler(dashboard.Options{})
	if handler == nil {
		t.Fatal("Handler() = nil")
	}
	response := serve(handler, http.MethodGet, "/")
	if dashboard.Built() {
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 with an embedded build", response.Code)
		}
		return
	}
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 without an embedded build", response.Code)
	}
}

// TestConsoleCarriesTheLanguage covers the language the console renders
// in: the handler hands the page the tag, and a handler without one serves
// the shell exactly as the build produced it.
func TestConsoleCarriesTheLanguage(t *testing.T) {
	shell := fstest.MapFS{
		"index.html": {Data: []byte("<html><head><title>Relo</title></head><body></body></html>")},
	}

	t.Run("the language is injected before the head closes", func(t *testing.T) {
		handler := dashboard.New(shell, dashboard.Options{Language: func() string { return "de" }})
		body := serve(handler, http.MethodGet, "/").Body.String()
		if !strings.Contains(body, "window.__reloLanguage=\"de\"") {
			t.Fatalf("body = %q, want the language handed to the page", body)
		}
		if !strings.Contains(body, "document.documentElement.lang=\"de\"") {
			t.Fatalf("body = %q, want the document language set", body)
		}
		if strings.Index(body, "window.__reloLanguage") > strings.Index(body, "</head>") {
			t.Fatalf("body = %q, want the script before the head closes", body)
		}
	})

	t.Run("a change reaches the next page load", func(t *testing.T) {
		tag := "en"
		handler := dashboard.New(shell, dashboard.Options{Language: func() string { return tag }})
		if body := serve(handler, http.MethodGet, "/").Body.String(); !strings.Contains(body, "\"en\"") {
			t.Fatalf("body = %q, want the first language", body)
		}
		tag = "zh-Hans"
		if body := serve(handler, http.MethodGet, "/").Body.String(); !strings.Contains(body, "\"zh-Hans\"") {
			t.Fatalf("body = %q, want the stored language", body)
		}
	})

	t.Run("a handler without a language serves the shell untouched", func(t *testing.T) {
		handler := dashboard.New(shell, dashboard.Options{})
		body := serve(handler, http.MethodGet, "/").Body.String()
		if strings.Contains(body, "__reloLanguage") {
			t.Fatalf("body = %q, want the shell as built", body)
		}
	})
}

func TestConsoleAppearanceArrivesBeforePaint(t *testing.T) {
	shell := fstest.MapFS{"index.html": {Data: []byte("<html><head></head><body></body></html>")}}
	theme, accent := "dark", "blue"
	handler := dashboard.New(shell, dashboard.Options{Appearance: func() (string, string) { return theme, accent }})
	body := serve(handler, http.MethodGet, "/").Body.String()
	if !strings.Contains(body, `window.__reloAppearance={theme:"dark",accent:"blue"}`) ||
		strings.Index(body, "__reloAppearance") > strings.Index(body, "</head>") {
		t.Fatalf("appearance was not injected before paint: %s", body)
	}
	theme, accent = "light", "teal"
	body = serve(handler, http.MethodGet, "/").Body.String()
	if !strings.Contains(body, `theme:"light",accent:"teal"`) {
		t.Fatalf("new appearance was not served: %s", body)
	}
}
