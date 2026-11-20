package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/threads"
)

// A Claude Desktop thread continues from the reply's id and sends only what is
// new, so the upstream has to be given the conversation Relo kept.
const threadRoot = `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,
"thread":{"type":"create"},
"system":[{"type":"text","text":"You are a careful assistant."}],
"tools":[{"name":"Read","description":"read","input_schema":{"type":"object"}}],
"messages":[{"role":"user","content":"break the file apart"}]}`

func sentMessages(t *testing.T, body string) (roles []string, tools int) {
	t.Helper()
	var sent struct {
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
		Tools []json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("upstream body is not JSON: %v", err)
	}
	for _, message := range sent.Messages {
		roles = append(roles, message.Role)
	}
	return roles, len(sent.Tools)
}

func threadMessageID(t *testing.T, stream string) string {
	t.Helper()
	const marker = `"id":"msg_`
	at := strings.Index(stream, marker)
	if at < 0 {
		t.Fatalf("no message id in %s", stream)
	}
	rest := stream[at+len(`"id":"`):]
	return rest[:strings.Index(rest, `"`)]
}

func TestContinuedThreadReachesTheUpstreamWithItsHistory(t *testing.T) {
	h := newHarness(t)
	first := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(threadRoot))
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body = %s", first.Code, first.Body.String())
	}
	id := threadMessageID(t, first.Body.String())

	next := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,
"thread":{"type":"continue","previous_message_id":"` + id + `"},
"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=1;"}],
"messages":[{"role":"user","content":"now refactor it"}]}`
	second := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(next))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, body = %s", second.Code, second.Body.String())
	}

	roles, tools := sentMessages(t, h.upstream.lastBody())
	if got := strings.Join(roles, ","); got != "system,user,assistant,user" {
		t.Fatalf("upstream roles = %s, want the first turn's history before the new message", got)
	}
	if tools != 1 {
		t.Fatalf("upstream tools = %d, want the thread's tool list", tools)
	}
	if strings.Contains(h.upstream.lastBody(), "billing-header") {
		t.Fatal("the billing line of a continued request reached the upstream")
	}
}

func TestContinuedThreadKeepsToolsWhenTheContinuationNamesNone(t *testing.T) {
	h := newHarness(t)
	first := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(threadRoot))
	id := threadMessageID(t, first.Body.String())
	next := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,
"thread":{"type":"continue","previous_message_id":"` + id + `"},
"messages":[{"role":"user","content":"go on"}]}`
	second := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(next))
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", second.Code, second.Body.String())
	}
	_, tools := sentMessages(t, h.upstream.lastBody())
	if tools != 1 {
		t.Fatalf("upstream tools = %d, want the thread's own list", tools)
	}
}

func TestContinuedThreadWithAnUnknownIDIsRefused(t *testing.T) {
	h := newHarness(t)
	before := h.upstream.requestCount()
	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,
"thread":{"type":"continue","previous_message_id":"msg_0000"},
"messages":[{"role":"user","content":"hi"}]}`
	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "invalid_request_error") {
		t.Fatalf("body = %s, want an invalid_request_error", response.Body.String())
	}
	if h.upstream.requestCount() != before {
		t.Fatal("a refused continuation still reached the upstream")
	}
}

func TestThreadHistoryBelongsToTheKeyThatOpenedIt(t *testing.T) {
	h := newHarness(t)
	first := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(threadRoot))
	id := threadMessageID(t, first.Body.String())
	history := threads.NewHistory(sqlite.NewThreadTurns(h.db), h.clock.Now)
	if _, found, err := history.Resolve(context.Background(), "another-key", id); err != nil || found {
		t.Fatalf("Resolve(other key) = found %v, error %v, want not found", found, err)
	}
}

func TestThreadWindowDefaultsToThirtyDaysAndRefusesNegative(t *testing.T) {
	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	want := time.Date(2026, time.October, 22, 0, 0, 0, 0, time.UTC).UnixMilli()
	if got := threads.CutoffMs(threads.DefaultRetentionDays, now); got != want {
		t.Fatalf("CutoffMs(30) = %d, want %d", got, want)
	}
	if threads.DefaultRetentionDays != 30 {
		t.Fatalf("DefaultRetentionDays = %d, want 30", threads.DefaultRetentionDays)
	}
	h := newHarness(t)
	response := h.management(http.MethodPut, "/api/v1/settings/threads", adminToken,
		strings.NewReader(`{"days":-1}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("negative window status = %d, body = %s, want 400", response.Code, response.Body.String())
	}
}

func TestThreadWindowIsStoredAndReadBack(t *testing.T) {
	h := newHarness(t)
	saved := h.management(http.MethodPut, "/api/v1/settings/threads", adminToken,
		strings.NewReader(`{"days":7}`))
	if saved.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saved.Code, saved.Body.String())
	}
	var settings struct {
		Threads struct {
			Days int `json:"days"`
		} `json:"threads"`
	}
	read := h.management(http.MethodGet, "/api/v1/settings", adminToken, nil)
	if read.Code != http.StatusOK {
		t.Fatalf("read status = %d, body = %s", read.Code, read.Body.String())
	}
	if err := json.Unmarshal(read.Body.Bytes(), &settings); err != nil {
		t.Fatalf("settings body is not JSON: %v", err)
	}
	if settings.Threads.Days != 7 {
		t.Fatalf("threads.days = %d, want the stored 7", settings.Threads.Days)
	}
}
