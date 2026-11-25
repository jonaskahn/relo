package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/server"
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

// TestContinuedThreadTakesASystemPromptTheClientSentForItself covers the newer
// instruction: a client that continues a thread and names its own prompt means
// it, so the thread's opening prompt does not replace it.
func TestContinuedThreadTakesASystemPromptTheClientSentForItself(t *testing.T) {
	h := newHarness(t)
	first := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(threadRoot))
	id := threadMessageID(t, first.Body.String())
	next := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,
"thread":{"type":"continue","previous_message_id":"` + id + `"},
"system":[{"type":"text","text":"answer in French."}],
"messages":[{"role":"user","content":"go on"}]}`
	second := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(next))
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", second.Code, second.Body.String())
	}
	body := h.upstream.lastBody()
	if !strings.Contains(body, "answer in French.") {
		t.Fatalf("upstream body = %s, want the client's own prompt", body)
	}
	if strings.Contains(body, "You are a careful assistant.") {
		t.Fatalf("upstream body = %s, want the thread's opening prompt replaced", body)
	}
}

// TestContinuedThreadTakesToolsTheContinuationNames covers the newer tool
// list: a continuation that sends tools means those, not the ones the thread
// opened with.
func TestContinuedThreadTakesToolsTheContinuationNames(t *testing.T) {
	h := newHarness(t)
	first := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(threadRoot))
	id := threadMessageID(t, first.Body.String())
	next := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,
"thread":{"type":"continue","previous_message_id":"` + id + `"},
"tools":[{"name":"Write","description":"write","input_schema":{"type":"object"}}],
"messages":[{"role":"user","content":"go on"}]}`
	second := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(next))
	if second.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", second.Code, second.Body.String())
	}
	body := h.upstream.lastBody()
	if !strings.Contains(body, `"Write"`) || strings.Contains(body, `"Read"`) {
		t.Fatalf("upstream body = %s, want the continuation's own tools", body)
	}
}

// failingHistory is a thread store that cannot do its work, so the read and
// the write take their failure paths.
type failingHistory struct{}

func (failingHistory) Resolve(context.Context, string, string) (threads.Resolved, bool, error) {
	return threads.Resolved{}, false, errors.New("store down")
}

func (failingHistory) Record(context.Context, threads.Turn) error {
	return errors.New("store down")
}

// TestContinuedThreadReportsAStoreFailure covers the read that fails: the
// client is told the conversation is unavailable rather than sent upstream
// without the history it left behind.
func TestContinuedThreadReportsAStoreFailure(t *testing.T) {
	h := newHarness(t)
	h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
		options.Threads = failingHistory{}
	})
	before := h.upstream.requestCount()
	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,
"thread":{"type":"continue","previous_message_id":"msg_0000"},
"messages":[{"role":"user","content":"hi"}]}`
	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400", response.Code, response.Body.String())
	}
	if h.upstream.requestCount() != before {
		t.Fatal("a failed thread read still reached the upstream")
	}
}

// TestFinishedTurnThatCannotBeKeptStillAnswers covers the write that fails:
// the reply the client is reading is already written, so the loss costs only
// the next turn, and the operator reads the warning.
func TestFinishedTurnThatCannotBeKeptStillAnswers(t *testing.T) {
	h := newHarness(t)
	h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
		options.Threads = failingHistory{}
	})
	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(threadRoot))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(h.logBuffer.String(), "keep the finished thread turn") {
		t.Fatalf("log = %q, want the store failure recorded", h.logBuffer.String())
	}
}

// TestThreadWindowSaveSweepsTheStoredWindow covers the write the card makes: a
// stored window is applied at once, so the card's promise holds without a
// restart.
func TestThreadWindowSaveSweepsTheStoredWindow(t *testing.T) {
	h := newHarness(t)
	turn := threads.Turn{
		ClientKey: "test-agent", ID: "msg_old",
		Delta: []inference.Message{{Role: inference.RoleUser, Content: []inference.ContentPart{{
			Type: inference.ContentTypeText, Text: "old",
		}}}},
		UpdatedAtMs: time.Unix(1_600_000_000, 0).UnixMilli(),
	}
	if err := sqlite.NewThreadTurns(h.db).Put(context.Background(), turn); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	saved := h.management(http.MethodPut, "/api/v1/settings/threads", adminToken, strings.NewReader(`{"days":7}`))
	if saved.Code != http.StatusOK {
		t.Fatalf("save status = %d, body = %s", saved.Code, saved.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, found, _ := sqlite.NewThreadTurns(h.db).Get(context.Background(), "test-agent", "msg_old"); !found {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the stored window was never applied")
}

// TestThreadWindowSaveIsRefusedWithoutASettingsService covers a build that
// serves no management settings, which answers the card rather than crashing.
func TestThreadWindowSaveIsRefusedWithoutASettingsService(t *testing.T) {
	h := newHarness(t)
	response := h.managementOn(serverWithoutService(h), http.MethodPut, "/api/v1/settings/threads", adminToken,
		strings.NewReader(`{"days":7}`))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s, want 503", response.Code, response.Body.String())
	}
}

// TestThreadWindowSaveRejectsABodyItCannotRead covers the malformed body the
// card may send: it is refused before anything is stored.
func TestThreadWindowSaveRejectsABodyItCannotRead(t *testing.T) {
	h := newHarness(t)
	response := h.management(http.MethodPut, "/api/v1/settings/threads", adminToken,
		strings.NewReader(`{"days":"seven"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s, want 400", response.Code, response.Body.String())
	}
}

// TestDataPlaneWithoutCodecsAnswersUnimplemented covers a build that carries a
// relay but no wire codecs: the surface says so instead of decoding nothing.
func TestDataPlaneWithoutCodecsAnswersUnimplemented(t *testing.T) {
	h := newHarness(t)
	h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
		options.Formats = nil
	})
	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(threadRoot))
	if response.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, body = %s, want 501", response.Code, response.Body.String())
	}
}

// failingRetention is a window that cannot be applied, so the save's sweep
// takes its failure path.
type failingRetention struct{}

func (failingRetention) Days(context.Context) (int, bool, error) {
	return 0, false, errors.New("store down")
}
func (failingRetention) SaveDays(context.Context, int) error  { return errors.New("store down") }
func (failingRetention) Sweep(context.Context) (int64, error) { return 0, errors.New("store down") }
func (failingRetention) Preview(context.Context, int) (int64, error) {
	return 0, errors.New("store down")
}

// TestThreadWindowSaveReportsASweepThatFails covers the window applied behind
// the response: a store that cannot sweep is logged, and the save still holds.
func TestThreadWindowSaveReportsASweepThatFails(t *testing.T) {
	h := newHarness(t)
	h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
		options.ThreadRetention = failingRetention{}
	})
	response := h.management(http.MethodPut, "/api/v1/settings/threads", adminToken, strings.NewReader(`{"days":7}`))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(h.logBuffer.String(), "apply the stored thread window") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("log = %q, want the sweep failure recorded", h.logBuffer.String())
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
