package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorBody is the shape every management failure answers with.
type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// get sends one management read with the language the caller asked for.
func get(t *testing.T, h *harness, path, accept string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Header.Set("Authorization", "Bearer "+adminToken)
	if accept != "" {
		request.Header.Set("Accept-Language", accept)
	}
	h.server.Handler().ServeHTTP(recorder, request)
	return recorder
}

func decodeError(t *testing.T, recorder *httptest.ResponseRecorder) errorBody {
	t.Helper()
	var body errorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return body
}

func TestManagementProseFollowsTheLanguage(t *testing.T) {
	harness := newHarness(t)

	t.Run("english until an operator chooses otherwise", func(t *testing.T) {
		body := decodeError(t, get(t, harness, "/api/v1/nothing", ""))
		if body.Error.Code != "not_found" || body.Error.Message != "no matching route" {
			t.Fatalf("body = %+v, want the English refusal", body)
		}
	})

	t.Run("a request that names a language gets it", func(t *testing.T) {
		body := decodeError(t, get(t, harness, "/api/v1/nothing", "de-DE,de;q=0.9"))
		if body.Error.Code != "not_found" {
			t.Fatalf("body = %+v, want the error code to stay stable", body)
		}
		if body.Error.Message != "keine passende Route" {
			t.Fatalf("message = %q, want the German refusal", body.Error.Message)
		}
	})

	t.Run("a language Relo cannot speak falls back to the operator setting", func(t *testing.T) {
		body := decodeError(t, get(t, harness, "/api/v1/nothing", "sw-KE,sw;q=0.9"))
		if body.Error.Message != "no matching route" {
			t.Fatalf("message = %q, want the operator language", body.Error.Message)
		}
	})

	t.Run("the stored language is what a client that asks for nothing gets", func(t *testing.T) {
		if err := harness.settings.SaveLanguage("de"); err != nil {
			t.Fatalf("SaveLanguage() error = %v", err)
		}
		body := decodeError(t, get(t, harness, "/api/v1/nothing", ""))
		if body.Error.Message != "keine passende Route" {
			t.Fatalf("message = %q, want the stored language", body.Error.Message)
		}
	})

	t.Run("the request still wins over the stored language", func(t *testing.T) {
		body := decodeError(t, get(t, harness, "/api/v1/nothing", "zh-Hans"))
		if body.Error.Message != "没有匹配的路由" {
			t.Fatalf("message = %q, want the language the request asked for", body.Error.Message)
		}
	})
}

func TestStatusReportsTheOperatorLanguage(t *testing.T) {
	harness := newHarness(t)

	var report struct {
		Language string `json:"language"`
	}
	recorder := harness.management(http.MethodGet, "/api/v1/status", adminToken, nil)
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if report.Language != "en" {
		t.Fatalf("language = %q, want the language the process started in", report.Language)
	}

	if err := harness.settings.SaveLanguage("zh-Hans"); err != nil {
		t.Fatalf("SaveLanguage() error = %v", err)
	}
	recorder = harness.management(http.MethodGet, "/api/v1/status", adminToken, nil)
	if err := json.Unmarshal(recorder.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if report.Language != "zh-Hans" {
		t.Fatalf("language = %q, want the stored language", report.Language)
	}
}

func TestSettingsCarryTheLanguage(t *testing.T) {
	harness := newHarness(t)

	t.Run("a settings read reports the language", func(t *testing.T) {
		recorder := harness.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
		var settings struct {
			Language string `json:"language"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &settings); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		if settings.Language != "auto" {
			t.Fatalf("language = %q, want auto before an operator chooses", settings.Language)
		}
	})

	t.Run("a settings write stores the language", func(t *testing.T) {
		body := strings.NewReader(`{"language":"de"}`)
		recorder := harness.management(http.MethodPatch, "/api/v1/settings/language", adminToken, body)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
		}
		read := harness.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
		var settings struct {
			Language string `json:"language"`
		}
		if err := json.Unmarshal(read.Body.Bytes(), &settings); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		if settings.Language != "de" {
			t.Fatalf("language = %q, want the stored choice", settings.Language)
		}
	})

	t.Run("a language Relo cannot speak is refused", func(t *testing.T) {
		body := strings.NewReader(`{"language":"sw"}`)
		recorder := harness.management(http.MethodPatch, "/api/v1/settings/language", adminToken, body)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body %s)", recorder.Code, recorder.Body.String())
		}
		if decoded := decodeError(t, recorder); decoded.Error.Code != "bad_request" {
			t.Fatalf("error = %q, want bad_request", decoded.Error.Code)
		}
	})
}
