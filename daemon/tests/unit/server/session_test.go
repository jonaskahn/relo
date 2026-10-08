package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/server"
	"github.com/jonaskahn/relo/tests/testkit"
)

// dashboardRequest builds a management request from the loopback interface,
// the way a browser on the same machine sends one.
func dashboardRequest(harness *harness, method, target string, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:52311"
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return request
}

// authRequest builds a JSON request to one of the console's sign-in routes,
// the way the compiled console sends one.
func authRequest(harness *harness, method, target, body string) *http.Request {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.RemoteAddr = "127.0.0.1:52311"
	request.Header.Set("Content-Type", "application/json")
	return request
}

// loginRequest posts the admin token the way the console signs in.
func loginRequest(harness *harness, token string) *http.Request {
	return authRequest(harness, http.MethodPost, "/api/v1/auth/login", `{"admin_token":"`+token+`"}`)
}

func cookieOf(response *httptest.ResponseRecorder, name string) string {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	return ""
}

// decodeAuthSession reads the sign-in state the session endpoint reports.
func decodeAuthSession(t *testing.T, response *httptest.ResponseRecorder) bool {
	t.Helper()
	var session struct {
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode session body: %v", err)
	}
	return session.Authenticated
}

func TestSignIn(t *testing.T) {
	harness := newHarness(t)

	t.Run("the sign-in page is served without a session", func(t *testing.T) {
		request := dashboardRequest(harness, http.MethodGet, "/login", "")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		if cookie := cookieOf(response, server.SessionCookieName); cookie != "" {
			t.Fatalf("the sign-in page minted the session %q", cookie)
		}
	})

	t.Run("a console asset is readable before signing in", func(t *testing.T) {
		request := dashboardRequest(harness, http.MethodGet, "/_app/immutable/app.js", "")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want the asset served without a session", response.Code)
		}
	})

	t.Run("a loopback browser trades the token for a session", func(t *testing.T) {
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, loginRequest(harness, adminToken))
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", response.Code, truncateBody(response.Body.String()))
		}
		if cache := response.Header().Get("Cache-Control"); cache != "no-store" {
			t.Fatalf("Cache-Control = %q, want no-store", cache)
		}
		var issued struct {
			CSRFToken string `json:"csrf_token"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil {
			t.Fatalf("decode sign-in body: %v", err)
		}
		if issued.CSRFToken == "" {
			t.Fatal("the sign-in answered no CSRF token")
		}
		if csrf := cookieOf(response, server.CSRFCookieName); csrf != issued.CSRFToken {
			t.Fatalf("CSRF cookie = %q, body = %q, want the same token", csrf, issued.CSRFToken)
		}
		if cookieOf(response, server.SessionCookieName) == "" {
			t.Fatal("signing in set no session cookie")
		}
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name == server.SessionCookieName && !cookie.HttpOnly {
				t.Fatal("the session cookie is readable from JavaScript")
			}
			if cookie.Name == server.CSRFCookieName && cookie.HttpOnly {
				t.Fatal("the CSRF cookie is unreadable to the console script")
			}
			if cookie.SameSite != http.SameSiteStrictMode {
				t.Fatalf("cookie %s SameSite = %v, want strict", cookie.Name, cookie.SameSite)
			}
		}
	})

	t.Run("the session signs a later request in", func(t *testing.T) {
		signIn := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(signIn, loginRequest(harness, adminToken))
		request := dashboardRequest(harness, http.MethodGet, "/api/v1/auth/session", "")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: cookieOf(signIn, server.SessionCookieName)})
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		if !decodeAuthSession(t, response) {
			t.Fatal("the session endpoint did not recognise the session")
		}
	})

	t.Run("an anonymous caller is not signed in", func(t *testing.T) {
		request := dashboardRequest(harness, http.MethodGet, "/api/v1/auth/session", "")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
		if decodeAuthSession(t, response) {
			t.Fatal("an anonymous request reported a session")
		}
	})

	t.Run("a wrong token is refused", func(t *testing.T) {
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, loginRequest(harness, "not-the-admin-token"))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
		if cookie := cookieOf(response, server.SessionCookieName); cookie != "" {
			t.Fatalf("a refused sign-in minted the session %q", cookie)
		}
	})

	t.Run("a missing token is refused", func(t *testing.T) {
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, loginRequest(harness, ""))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
	})

	t.Run("a non-loopback request may not sign in", func(t *testing.T) {
		request := loginRequest(harness, adminToken)
		request.RemoteAddr = "203.0.113.7:4444"
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
		if cookie := cookieOf(response, server.SessionCookieName); cookie != "" {
			t.Fatalf("a remote request minted the session %q", cookie)
		}
	})

	t.Run("a cross-site sign-in is refused", func(t *testing.T) {
		request := loginRequest(harness, adminToken)
		request.Header.Set("Origin", "https://evil.example")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})

	t.Run("a bearer token still works for scripts", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/api/v1/accounts", adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})

	t.Run("the old query-token bootstrap is gone", func(t *testing.T) {
		request := dashboardRequest(harness, http.MethodGet, "/?token="+adminToken, "")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the sign-in redirect", response.Code)
		}
		if location := response.Header().Get("Location"); !strings.HasPrefix(location, "/login") {
			t.Fatalf("Location = %q, want the sign-in page", location)
		}
		if cookie := cookieOf(response, server.SessionCookieName); cookie != "" {
			t.Fatalf("a URL token minted the session %q", cookie)
		}
	})
}

func TestSignOut(t *testing.T) {
	harness := newHarness(t)
	session, err := harness.sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}

	t.Run("signing out revokes the session and clears the cookies", func(t *testing.T) {
		request := authRequest(harness, http.MethodPost, "/api/v1/auth/logout", "")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
		request.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: session.CSRFToken})
		request.Header.Set(server.CSRFHeaderName, session.CSRFToken)
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204", response.Code)
		}
		for _, cookie := range response.Result().Cookies() {
			if cookie.Name != server.SessionCookieName && cookie.Name != server.CSRFCookieName {
				continue
			}
			if cookie.MaxAge >= 0 {
				t.Fatalf("cookie %s was not expired: MaxAge = %d", cookie.Name, cookie.MaxAge)
			}
		}
		if _, found := harness.sessions.Lookup(session.Token); found {
			t.Fatal("the session is still live after signing out")
		}
	})

	t.Run("signing out without CSRF is refused", func(t *testing.T) {
		fresh, err := harness.sessions.Mint()
		if err != nil {
			t.Fatalf("Mint() error = %v", err)
		}
		request := authRequest(harness, http.MethodPost, "/api/v1/auth/logout", "")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: fresh.Token})
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
		if _, found := harness.sessions.Lookup(fresh.Token); !found {
			t.Fatal("a refused sign-out revoked the session anyway")
		}
	})
}

// truncateBody keeps a failure message readable.
func truncateBody(body string) string {
	if len(body) <= 300 {
		return body
	}
	return body[:300] + "..."
}

func TestSessionGuard(t *testing.T) {
	harness := newHarness(t)
	session, err := harness.sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}

	t.Run("no session and no token sends a page to sign in", func(t *testing.T) {
		response := harness.management(http.MethodGet, "/", "", nil)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the sign-in redirect", response.Code)
		}
		if location := response.Header().Get("Location"); !strings.HasPrefix(location, "/login") {
			t.Fatalf("Location = %q, want the sign-in page", location)
		}
	})

	t.Run("a session reads a page", func(t *testing.T) {
		request := dashboardRequest(harness, http.MethodGet, "/", "")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", response.Code)
		}
	})

	t.Run("a write without CSRF is refused", func(t *testing.T) {
		request := authRequest(harness, http.MethodPut, "/api/v1/settings/retention", "{}")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
		request.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: session.CSRFToken})
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})

	t.Run("a write with CSRF succeeds", func(t *testing.T) {
		request := authRequest(harness, http.MethodPut, "/api/v1/settings/retention", "{}")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
		request.AddCookie(&http.Cookie{Name: server.CSRFCookieName, Value: session.CSRFToken})
		request.Header.Set(server.CSRFHeaderName, session.CSRFToken)
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", response.Code, truncateBody(response.Body.String()))
		}
	})

	t.Run("a cross-site write is refused", func(t *testing.T) {
		request := authRequest(harness, http.MethodPut, "/api/v1/settings/retention", "{}")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
		request.Header.Set(server.CSRFHeaderName, session.CSRFToken)
		request.Header.Set("Origin", "https://evil.example")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})

	t.Run("a wrong CSRF token is refused", func(t *testing.T) {
		request := authRequest(harness, http.MethodPut, "/api/v1/settings/retention", "{}")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
		request.Header.Set(server.CSRFHeaderName, "not-the-token")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})
}

func TestSessionsExpire(t *testing.T) {
	clk := testkit.NewFakeClock(time.Unix(1_700_000_000, 0))
	sessions := server.NewSessions(clk)
	session, err := sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	if _, found := sessions.Lookup(session.Token); !found {
		t.Fatal("a fresh session was not found")
	}
	clk.Add(13 * time.Hour)
	if _, found := sessions.Lookup(session.Token); found {
		t.Fatal("an expired session was still valid")
	}
	if sessions.Count() != 0 {
		t.Fatalf("Count() = %d, want expired sessions dropped", sessions.Count())
	}
}

func TestSessionsRevokeAndBound(t *testing.T) {
	clk := testkit.NewFakeClock(time.Unix(1_700_000_000, 0))
	sessions := server.NewSessions(clk)
	first, err := sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	sessions.Revoke(first.Token)
	if _, found := sessions.Lookup(first.Token); found {
		t.Fatal("a revoked session was still valid")
	}
	if _, found := sessions.Lookup(""); found {
		t.Fatal("an empty token was accepted")
	}
	for index := 0; index < server.MaxSessions+10; index++ {
		if _, err := sessions.Mint(); err != nil {
			t.Fatalf("Mint() error = %v", err)
		}
		clk.Add(time.Minute)
	}
	if count := sessions.Count(); count > server.MaxSessions {
		t.Fatalf("Count() = %d, want at most %d", count, server.MaxSessions)
	}
}

func TestSessionsExpiredRequestIsUnauthorized(t *testing.T) {
	harness := newHarness(t)
	session, err := harness.sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	harness.clock.Add(13 * time.Hour)
	request := dashboardRequest(harness, http.MethodGet, "/", "")
	request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
	response := httptest.NewRecorder()
	harness.server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want the sign-in redirect for an expired session", response.Code)
	}
	if location := response.Header().Get("Location"); !strings.HasPrefix(location, "/login") {
		t.Fatalf("Location = %q, want the sign-in page", location)
	}
}

// TestExpiredSessionKeepsTheAPIContract covers the two audiences the guard
// serves: a browser is sent to sign in, and a script keeps its JSON shape.
func TestExpiredSessionKeepsTheAPIContract(t *testing.T) {
	harness := newHarness(t)
	session, err := harness.sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	harness.clock.Add(13 * time.Hour)

	t.Run("an API path keeps the JSON 401", func(t *testing.T) {
		request := dashboardRequest(harness, http.MethodGet, "/api/v1/accounts", "")
		request.AddCookie(&http.Cookie{Name: server.SessionCookieName, Value: session.Token})
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", response.Code)
		}
		if contentType := response.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
			t.Fatalf("Content-Type = %q, want JSON", contentType)
		}
	})

	t.Run("a page request keeps the sign-in redirect", func(t *testing.T) {
		request := dashboardRequest(harness, http.MethodGet, "/logs", "")
		response := httptest.NewRecorder()
		harness.server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want the sign-in redirect", response.Code)
		}
		if location := response.Header().Get("Location"); !strings.HasPrefix(location, "/login") {
			t.Fatalf("Location = %q, want the sign-in page", location)
		}
	})
}

func TestSessionTokensAreOpaque(t *testing.T) {
	sessions := server.NewSessions(nil)
	first, err := sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	second, err := sessions.Mint()
	if err != nil {
		t.Fatalf("Mint() error = %v", err)
	}
	if first.Token == second.Token || first.CSRFToken == second.CSRFToken {
		t.Fatal("two sessions share a token")
	}
	if len(first.Token) != server.SessionTokenBytes*2 {
		t.Fatalf("token length = %d, want %d hex characters", len(first.Token), server.SessionTokenBytes*2)
	}
	if first.CSRFToken == first.Token {
		t.Fatal("the CSRF token repeats the session token")
	}
}
