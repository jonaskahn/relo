package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

// chatFixture points the mock upstream at one complete body, which is the
// shape a tester turn asks for.
func chatFixture(h *harness, fixture string, status int) {
	h.upstream.mu.Lock()
	defer h.upstream.mu.Unlock()
	h.upstream.fixture = fixture
	h.upstream.sse = false
	h.upstream.status = status
}

// chatStreamFixture points the mock upstream at one SSE body, which is the
// shape an upstream that only answers streams returns.
func chatStreamFixture(h *harness, fixture string, status int) {
	h.upstream.mu.Lock()
	defer h.upstream.mu.Unlock()
	h.upstream.fixture = fixture
	h.upstream.sse = true
	h.upstream.status = status
}

// codexSecretRef names the stream-only connection's secret in the harness's
// store, kept apart from the one the mock upstream's own account uses.
const codexSecretRef = "apikey/openai-codex/one"

// codexCredential is the account the stream-only connection runs on: the
// pool is what makes a connection configured.
func codexCredential() account.PoolEntry {
	return account.PoolEntry{
		ID: "codex-one", ProviderID: "openai-codex", Kind: "api_key",
		Label: "codex", SecretRef: codexSecretRef, Status: account.StatusActive,
	}
}

// chatTurn sends one tester request and reads the answer the daemon writes.
// The status comes back too, because a malformed request is answered with a
// refusal rather than the answer shape.
func chatTurn(t *testing.T, h *harness, payload map[string]any) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode the request: %v", err)
	}
	response := h.management(http.MethodPost, "/api/v1/integrations/chat", adminToken, bytes.NewReader(raw))
	var answer map[string]any
	if len(response.Body.Bytes()) > 0 {
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatalf("decode the answer: %v (%s)", err, response.Body.String())
		}
	}
	return response.Code, answer
}

// directTurn names one connection's model, which is what a direct test sends.
func directTurn(providerID, modelID string) map[string]any {
	return map[string]any{
		"mode": "direct", "provider_id": providerID, "model_id": modelID,
		"messages": []map[string]any{{"role": "user", "content": "hello"}},
	}
}

// formatTurn names a client shape and a three-turn conversation, which is what
// a format test sends; extra fields name the target the caller chose.
func formatTurn(format string, extra map[string]any) map[string]any {
	payload := map[string]any{
		"mode": "format", "format": format, "provider_id": "openai", "model_id": "gpt-4o",
		"messages": []map[string]any{
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "hi"},
			{"role": "user", "content": "again"},
		},
	}
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

// chatFrame is one frame of a streamed tester answer as a test reads it: a
// text delta, or the answer that closed the turn.
type chatFrame struct {
	Type   string         `json:"type"`
	Text   string         `json:"text"`
	Answer map[string]any `json:"answer"`
}

// chatStreamTurn sends one tester request that asks to stream and reads the
// frames that came back.
func chatStreamTurn(t *testing.T, h *harness, payload map[string]any) (int, string, []chatFrame) {
	t.Helper()
	payload["stream"] = true
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode the request: %v", err)
	}
	response := h.management(http.MethodPost, "/api/v1/integrations/chat", adminToken, bytes.NewReader(raw))
	return response.Code, response.Header().Get("Content-Type"), chatFrames(t, response.Body.String())
}

// chatFrames reads the frames of a streamed tester answer, refusing anything
// that is not one data frame per line.
func chatFrames(t *testing.T, body string) []chatFrame {
	t.Helper()
	frames := make([]chatFrame, 0, 4)
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			t.Fatalf("stream line = %q, want a data frame", line)
		}
		var frame chatFrame
		if err := json.Unmarshal([]byte(data), &frame); err != nil {
			t.Fatalf("decode a stream frame: %v (%s)", err, line)
		}
		frames = append(frames, frame)
	}
	return frames
}

// usageOrigin reads the origin one recorded request was stored with.
func usageOrigin(t *testing.T, h *harness, requestID string) string {
	t.Helper()
	var origin string
	if err := h.db.SQL().QueryRow("SELECT origin FROM usage_events WHERE request_id = ?", requestID).Scan(&origin); err != nil {
		t.Fatalf("read the recorded origin: %v", err)
	}
	return origin
}

// TestChatTesterCallsTheChosenProvider is the direct mode: one connection and
// model answer, the turn is stored as internal traffic, and the answer carries
// the identifiers the log shows.
func TestChatTesterCallsTheChosenProvider(t *testing.T) {
	h := newHarness(t)
	chatFixture(h, "openai/chat_nonstreaming.json", http.StatusOK)

	status, answer := chatTurn(t, h, directTurn("openai", "gpt-4o"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", status, answer)
	}
	if answer["ok"] != true || answer["text"] != "Hi there" {
		t.Fatalf("answer = %v, want the provider's text", answer)
	}
	if answer["provider"] != "openai" || answer["model"] != "gpt-4o" {
		t.Fatalf("answer = %v, want the chosen provider and model", answer)
	}
	requestID, _ := answer["request_id"].(string)
	if requestID == "" {
		t.Fatalf("answer = %v, want a request identifier", answer)
	}
	if answer["route_reason"] != string(routing.KindQualified) {
		t.Fatalf("route_reason = %v, want %q", answer["route_reason"], routing.KindQualified)
	}
	if origin := usageOrigin(t, h, requestID); origin != inference.OriginInternal {
		t.Fatalf("origin = %q, want %q", origin, inference.OriginInternal)
	}
}

// TestChatTesterReachesAStreamOnlyUpstream is the connection the operator
// adds through the Codex sign-in: the ChatGPT backend refuses a one-shot
// request, so the relay asks for the stream on the tester's behalf and
// reassembles the answer. The requirement is read from the connection's
// template, which the row keeps.
func TestChatTesterReachesAStreamOnlyUpstream(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	repo := sqlite.NewCatalogRepo(h.db)
	if err := repo.SaveProvider(ctx, sqlite.ProviderRow{
		ID: "openai-codex", TemplateID: "openai-codex", Origin: string(catalog.OriginSignIn),
		Label: "ChatGPT (Codex sign-in)", Auth: string(catalog.AuthAPIKey),
		APIFormat: string(catalog.FormatOpenAIResp), BaseURL: h.upstream.URL(),
		ModelsFormat: string(catalog.ModelsCodex),
		Enabled:      true, Rank: 100, PoolStrategy: "least-loaded",
	}); err != nil {
		t.Fatalf("save the stream-only provider: %v", err)
	}
	if err := repo.SaveModel(ctx, sqlite.ModelRow{
		ProviderID: "openai-codex", ModelID: "gpt-5-codex", Source: "manual",
		APIFormat: string(catalog.FormatOpenAIResp), Enabled: true,
	}); err != nil {
		t.Fatalf("save the stream-only model: %v", err)
	}
	name, category, status := "GPT-5 Codex", string(catalog.CategoryChat), "active"
	if err := repo.SaveModelFacts(ctx, sqlite.ModelFactsRow{
		ProviderID: "openai-codex", ModelID: "gpt-5-codex", Layer: "override",
		Name: &name, Category: &category, Status: &status,
	}); err != nil {
		t.Fatalf("save the stream-only model facts: %v", err)
	}
	// The account arrives with the rebuilt pools, because the pool is what
	// makes a connection configured.
	h.secrets[codexSecretRef] = "sk-codex"
	h.server = h.newServerWithCredentials(h.cfg.Server.Port, defaultEntry(), codexCredential())

	chatStreamFixture(h, "openai/responses_streaming.txt", http.StatusOK)
	turnStatus, answer := chatTurn(t, h, directTurn("openai-codex", "gpt-5-codex"))
	if turnStatus != http.StatusOK || answer["ok"] != true {
		t.Fatalf("status = %d, answer = %v, want the reassembled answer", turnStatus, answer)
	}
	if answer["text"] != "Hello world" {
		t.Fatalf("text = %v, want the streamed text reassembled", answer["text"])
	}
	if answer["output_tokens"] != float64(2) {
		t.Fatalf("answer = %v, want the usage the stream reported", answer)
	}
	if body := h.upstream.lastBody(); !strings.Contains(body, `"stream":true`) {
		t.Fatalf("upstream body = %q, want the streamed request the backend requires", body)
	}
	if body := h.upstream.lastBody(); strings.Contains(body, "max_output_tokens") {
		t.Fatalf("upstream body = %q, want no ceiling the backend refuses", body)
	}
}

// TestChatTesterSendsEveryClientFormat is the format mode: each shape Relo
// accepts decodes the same conversation and reaches the same provider.
func TestChatTesterSendsEveryClientFormat(t *testing.T) {
	h := newHarness(t)
	chatFixture(h, "openai/chat_nonstreaming.json", http.StatusOK)

	for _, format := range []string{"openai-chat", "openai-responses", "anthropic"} {
		t.Run(format, func(t *testing.T) {
			status, answer := chatTurn(t, h, formatTurn(format, nil))
			if status != http.StatusOK || answer["ok"] != true || answer["text"] != "Hi there" {
				t.Fatalf("answer = %v, want the provider's text through %s", answer, format)
			}
		})
	}
}

// TestChatTesterFollowsARoute is the other format-mode target: a published
// route, which the tester reaches the way a client would.
func TestChatTesterFollowsARoute(t *testing.T) {
	h := newHarness(t)
	chatFixture(h, "openai/chat_nonstreaming.json", http.StatusOK)
	if err := h.routes.Save(context.Background(), "fast", approuting.Write{
		Label: "Fast", Strategy: "priority", Enabled: true, Listed: true,
		Members: []approuting.MemberWrite{
			{ProviderID: "openai", ModelID: "gpt-4o", Kind: "model", Enabled: true, Weight: 1},
		},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}

	status, answer := chatTurn(t, h, formatTurn("anthropic", map[string]any{"route_id": "fast"}))
	if status != http.StatusOK || answer["ok"] != true || answer["text"] != "Hi there" {
		t.Fatalf("answer = %v, want the route to serve the text", answer)
	}
	if answer["route_reason"] != string(routing.KindGroup) {
		t.Fatalf("route_reason = %v, want %q", answer["route_reason"], routing.KindGroup)
	}
}

// TestChatTesterRefusesAnUnreachableTarget reports the failure rather than a
// blank answer, and a malformed request is a bad request.
func TestChatTesterRefusesAnUnreachableTarget(t *testing.T) {
	h := newHarness(t)
	chatFixture(h, "openai/chat_nonstreaming.json", http.StatusOK)

	t.Run("a target no provider serves", func(t *testing.T) {
		status, answer := chatTurn(t, h, directTurn("nope", "nope"))
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%v)", status, answer)
		}
		if answer["ok"] != false || answer["error"] == "" {
			t.Fatalf("answer = %v, want a refusal with a reason", answer)
		}
	})

	t.Run("a direct test without a model", func(t *testing.T) {
		status, _ := chatTurn(t, h, map[string]any{
			"mode": "direct", "provider_id": "openai",
			"messages": []map[string]any{{"role": "user", "content": "hello"}},
		})
		if status != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", status)
		}
	})
}

// TestChatTesterReportsAnUpstreamRefusal keeps the provider's own status and
// message, which is what tells an operator the credential was wrong.
func TestChatTesterReportsAnUpstreamRefusal(t *testing.T) {
	h := newHarness(t)
	chatFixture(h, "openai/chat_error.json", http.StatusBadRequest)

	status, answer := chatTurn(t, h, directTurn("openai", "gpt-4o"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", status, answer)
	}
	if answer["ok"] != false {
		t.Fatalf("answer = %v, want the upstream refusal", answer)
	}
	if answer["status"] != float64(http.StatusBadRequest) {
		t.Fatalf("answer = %v, want the upstream status", answer)
	}
	message, _ := answer["error"].(string)
	if !strings.Contains(message, "Incorrect API key") || answer["error_code"] == "" {
		t.Fatalf("answer = %v, want the upstream message and a code", answer)
	}
}

// TestChatTesterAnswersWithoutText keeps a successful turn whose answer carries
// no text a success, so an operator is not told a working call failed.
func TestChatTesterAnswersWithoutText(t *testing.T) {
	h := newHarness(t)
	chatFixture(h, "openai/chat_empty.json", http.StatusOK)

	_, answer := chatTurn(t, h, directTurn("openai", "gpt-4o"))
	if answer["ok"] != true || answer["text"] != "" {
		t.Fatalf("answer = %v, want a successful turn with no text", answer)
	}
}

// TestChatTesterStreamsTheReplyWhenAsked covers the turn that asks to stream:
// the daemon answers frame by frame, each text delta on its own, and closes
// with the answer that carries the identifiers and the usage the log shows.
func TestChatTesterStreamsTheReplyWhenAsked(t *testing.T) {
	h := newHarness(t)
	chatStreamFixture(h, "openai/chat_streaming.txt", http.StatusOK)

	status, contentType, frames := chatStreamTurn(t, h, directTurn("openai", "gpt-4o"))
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.HasPrefix(contentType, "text/event-stream") {
		t.Fatalf("content type = %q, want the frames the console reads", contentType)
	}
	if len(frames) < 2 {
		t.Fatalf("frames = %+v, want the deltas and the answer that closes them", frames)
	}
	streamed := ""
	for _, frame := range frames[:len(frames)-1] {
		if frame.Type != "delta" {
			t.Fatalf("frame = %+v, want a text delta before the answer", frame)
		}
		streamed += frame.Text
	}
	answer := frames[len(frames)-1]
	if answer.Type != "answer" || answer.Answer == nil {
		t.Fatalf("last frame = %+v, want the answer that closes the turn", answer)
	}
	if answer.Answer["ok"] != true || answer.Answer["text"] != "Hello world" {
		t.Fatalf("answer = %v, want the streamed text", answer.Answer)
	}
	if streamed != "Hello world" {
		t.Fatalf("deltas = %q, want the text the answer reports", streamed)
	}
	if answer.Answer["input_tokens"] != float64(9) || answer.Answer["output_tokens"] != float64(2) {
		t.Fatalf("answer = %v, want the usage the stream reported", answer.Answer)
	}
	if id, _ := answer.Answer["request_id"].(string); id == "" {
		t.Fatalf("answer = %v, want the identifier the log shows", answer.Answer)
	}
}

// TestChatTesterClosesAStreamedRefusal covers the turns that fail after the
// stream opened: the console is told in the frame that closes it, so a
// failure is read the same way whatever the relay reported.
func TestChatTesterClosesAStreamedRefusal(t *testing.T) {
	t.Run("an upstream refusal", func(t *testing.T) {
		h := newHarness(t)
		chatFixture(h, "openai/chat_error.json", http.StatusBadRequest)

		status, _, frames := chatStreamTurn(t, h, directTurn("openai", "gpt-4o"))
		if status != http.StatusOK || len(frames) == 0 {
			t.Fatalf("status = %d, frames = %+v, want a closed stream", status, frames)
		}
		answer := frames[len(frames)-1]
		if answer.Type != "answer" || answer.Answer["ok"] != false {
			t.Fatalf("last frame = %+v, want a refusal", answer)
		}
		if answer.Answer["status"] != float64(http.StatusBadRequest) {
			t.Fatalf("answer = %v, want the upstream status", answer.Answer)
		}
		message, _ := answer.Answer["error"].(string)
		if !strings.Contains(message, "Incorrect API key") || answer.Answer["error_code"] == "" {
			t.Fatalf("answer = %v, want the upstream message and a code", answer.Answer)
		}
	})

	t.Run("a target no provider serves", func(t *testing.T) {
		h := newHarness(t)
		chatFixture(h, "openai/chat_nonstreaming.json", http.StatusOK)

		status, _, frames := chatStreamTurn(t, h, directTurn("nope", "nope"))
		if status != http.StatusOK || len(frames) == 0 {
			t.Fatalf("status = %d, frames = %+v, want a closed stream", status, frames)
		}
		answer := frames[len(frames)-1]
		if answer.Type != "answer" || answer.Answer["ok"] != false || answer.Answer["error"] == "" {
			t.Fatalf("last frame = %+v, want a refusal with a reason", answer)
		}
	})
}

// TestRequestLogFiltersByOrigin covers the filter the logs page gained: the
// two origins read apart, and the filter holds across a page boundary.
func TestRequestLogFiltersByOrigin(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	events := []sqlite.UsageEvent{
		{RequestID: "external-1", Timestamp: 3_000, Provider: "openai", Model: "gpt-4o", Status: 200, Origin: inference.OriginExternal},
		{RequestID: "internal-1", Timestamp: 2_000, Provider: "openai", Model: "gpt-4o", Status: 200, Origin: inference.OriginInternal},
		{RequestID: "internal-2", Timestamp: 1_000, Provider: "openai", Model: "gpt-4o", Status: 200, Origin: inference.OriginInternal},
	}
	for _, event := range events {
		if _, err := h.usage.AppendEvent(ctx, event); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", event.RequestID, err)
		}
	}

	page := func(t *testing.T, query string) ([]string, string) {
		t.Helper()
		response := h.management(http.MethodGet, "/api/v1/activity/requests?"+query, adminToken, nil)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", response.Code, response.Body.String())
		}
		var body struct {
			Items []struct {
				RequestID string
				Origin    string
			}
			NextCursor string `json:"next_cursor"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode the response: %v", err)
		}
		ids := make([]string, 0, len(body.Items))
		for _, item := range body.Items {
			if item.Origin == "" {
				t.Fatalf("row %s carries no origin", item.RequestID)
			}
			ids = append(ids, item.RequestID)
		}
		return ids, body.NextCursor
	}

	t.Run("internal traffic reads apart from external", func(t *testing.T) {
		if ids, _ := page(t, "origin=internal"); len(ids) != 2 {
			t.Fatalf("internal rows = %v, want the two chat tester turns", ids)
		}
		ids, _ := page(t, "origin=external")
		if len(ids) != 1 || ids[0] != "external-1" {
			t.Fatalf("external rows = %v, want the one data plane request", ids)
		}
	})

	t.Run("the filter holds across a page boundary", func(t *testing.T) {
		first, cursor := page(t, "origin=internal&limit=1")
		if len(first) != 1 || cursor == "" {
			t.Fatalf("first page = %v, cursor = %q, want one row and a cursor", first, cursor)
		}
		second, _ := page(t, "origin=internal&limit=1&cursor="+url.QueryEscape(cursor))
		if len(second) != 1 || second[0] == first[0] {
			t.Fatalf("second page = %v, want the other internal row", second)
		}
	})
}
