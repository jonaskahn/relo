package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/server"
)

// journalTraffic is a traffic log a test drives: it keeps every half it was
// handed, and reports each one as it arrives so a test can watch the file fill
// while a request is still running.
type journalTraffic struct {
	mu      sync.Mutex
	halves  []server.TrafficHalf
	arrived chan server.TrafficHalf
}

func (j *journalTraffic) AppendHalf(half server.TrafficHalf) error {
	j.mu.Lock()
	j.halves = append(j.halves, half)
	j.mu.Unlock()
	if j.arrived != nil {
		j.arrived <- half
	}
	return nil
}

func (j *journalTraffic) Halves(ctx context.Context, requestID string) ([]server.TrafficHalf, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	found := make([]server.TrafficHalf, 0, len(j.halves))
	for _, half := range j.halves {
		if half.RequestID == requestID {
			found = append(found, half)
		}
	}
	return found, nil
}

// kinds names the halves written so far, in the order they arrived.
func (j *journalTraffic) kinds() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	kinds := make([]string, 0, len(j.halves))
	for _, half := range j.halves {
		kinds = append(kinds, half.Kind)
	}
	return kinds
}

// traceJournal builds a server whose traffic log is the journal, with the
// switch the operator throws turned on.
func traceJournal(t *testing.T, h *harness, journal *journalTraffic, entries ...account.PoolEntry) {
	t.Helper()
	if len(entries) == 0 {
		entries = []account.PoolEntry{defaultEntry()}
	}
	h.server = h.newServerWithOptions(h.cfg, entries, func(options *server.Options) {
		options.Traffic = journal
	})
	enableTrafficLog(t, h)
}

// enableTrafficLog saves the global switch the traffic log reads per request.
func enableTrafficLog(t *testing.T, h *harness) {
	t.Helper()
	patch := `{"autostart":true,"log_level":"info","log_traffic":true,` +
		`"updates_url":"https://jonaskahn.github.io/relo/appcast.xml",` +
		`"updates_download":"https://github.com/jonaskahn/relo/releases/latest"}`
	response := h.management(http.MethodPatch, "/api/v1/settings/system", adminToken, strings.NewReader(patch))
	if response.Code != http.StatusOK {
		t.Fatalf("turn the traffic log on: %d %s", response.Code, response.Body.String())
	}
}

// TestTrafficLogKeepsTheRequestBeforeTheAnswerExists covers what the tracing is
// for: the agent's own request reaches the file while the upstream is still
// busy, so a request that hangs or dies is still there to read. The four halves
// of the exchange follow in the order they happen.
func TestTrafficLogKeepsTheRequestBeforeTheAnswerExists(t *testing.T) {
	h := newHarness(t)
	journal := &journalTraffic{arrived: make(chan server.TrafficHalf, 8)}
	traceJournal(t, h, journal)
	h.upstream.delayBy(300 * time.Millisecond)
	defer h.upstream.delayBy(0)

	answered := make(chan struct{})
	go func() {
		defer close(answered)
		h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody()))
	}()

	select {
	case half := <-journal.arrived:
		if half.Kind != activity.CaptureAgentRequest {
			t.Fatalf("first half = %q, want the agent's own request", half.Kind)
		}
		if half.Surface != "chat-completions" || half.Origin != inference.OriginExternal {
			t.Fatalf("half = %+v, want the surface and origin recorded", half)
		}
	case <-answered:
		t.Fatal("the answer finished before the request reached the log")
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the log")
	}
	<-answered
	h.settle()

	want := []string{
		activity.CaptureAgentRequest, activity.CaptureAgentResponse,
		activity.CaptureProviderRequest, activity.CaptureProviderResponse,
	}
	written := journal.kinds()
	if len(written) != len(want) {
		t.Fatalf("halves = %v, want the four of one exchange", written)
	}
	seen := map[string]bool{}
	for _, kind := range written {
		seen[kind] = true
	}
	for _, kind := range want {
		if !seen[kind] {
			t.Fatalf("halves = %v, want %q traced", written, kind)
		}
	}
	for _, half := range journal.halves {
		if half.RequestID == "" {
			t.Fatalf("half = %+v, want the request id on every half", half)
		}
		if half.Timestamp == "" {
			t.Fatalf("half = %+v, want the moment it was written", half)
		}
	}
}

// TestTrafficLogKeepsEveryAttemptOfARefusedRequest covers the walk: each try at
// each account lands in the file with its own ordinal, and the answer the agent
// was given closes the exchange.
func TestTrafficLogKeepsEveryAttemptOfARefusedRequest(t *testing.T) {
	h := newHarness(t)
	h.secrets[spareCredentialRef] = "sk-second"
	h.upstream.status = http.StatusServiceUnavailable
	journal := &journalTraffic{}
	traceJournal(t, h, journal, defaultEntry(), spareEntry())
	setRetryAttempts(t, h, "openai", 1)

	body := `{"model":"claude-relo-openai--gpt-4o","max_tokens":16,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	response := h.dataPlaneOn(inference.ProtocolAnthropic, http.MethodPost, "/v1/messages", dataPlaneToken,
		strings.NewReader(body))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	h.settle()

	ordinals := 0
	requests, replies := 0, 0
	for _, half := range journal.halves {
		switch half.Kind {
		case activity.CaptureProviderRequest:
			requests++
			ordinals = checkOrdinal(t, half, ordinals)
		case activity.CaptureProviderResponse:
			replies++
			ordinals = checkOrdinal(t, half, ordinals)
		}
	}
	if requests != 4 || replies != 4 {
		t.Fatalf("%d provider requests and %d replies, want one pair per try at each account", requests, replies)
	}
	kinds := journal.kinds()
	if kinds[0] != activity.CaptureAgentRequest || kinds[len(kinds)-1] != activity.CaptureAgentResponse {
		t.Fatalf("halves = %v, want the request first and the answer last", kinds)
	}
}

// checkOrdinal keeps the compacted ordinals a request's halves are addressed
// by, so a test reads which try each half belongs to.
func checkOrdinal(t *testing.T, half server.TrafficHalf, seen int) int {
	t.Helper()
	if half.Ordinal == nil {
		t.Fatalf("half = %+v, want the attempt it belongs to", half)
	}
	if *half.Ordinal == seen {
		return seen
	}
	if *half.Ordinal != seen+1 {
		t.Fatalf("ordinal = %d, want the tries in order", *half.Ordinal)
	}
	return seen + 1
}

// TestCaptureReadsFallBackToTheTrafficLog covers the detail page's whole-body
// path: a state row that was cut short is answered from the half the traffic
// log kept, and the manifest says the request is traced rather than claiming a
// body it does not hold.
func TestCaptureReadsFallBackToTheTrafficLog(t *testing.T) {
	h := newHarness(t)
	journal := &journalTraffic{}
	traceJournal(t, h, journal)
	eventID, ids := seedCapturedRequest(t, h)
	event := strconv.FormatInt(eventID, 10)
	whole := strings.Repeat("the whole reply ", 4000)
	ordinal := 0
	if err := journal.AppendHalf(server.TrafficHalf{
		RequestID: "captured-1", Timestamp: "2026-10-11T09:00:00Z",
		Kind: activity.CaptureProviderResponse, Ordinal: &ordinal, Status: http.StatusOK,
		Message: &server.TrafficMessage{Status: http.StatusOK, Body: whole},
	}); err != nil {
		t.Fatalf("AppendHalf() error = %v", err)
	}

	listed := h.management(http.MethodGet, "/api/v1/activity/requests/"+event+"/captures", adminToken, nil)
	var manifest struct {
		Items []struct {
			Kind      string `json:"kind"`
			BodyBytes int64  `json:"body_bytes"`
			Truncated bool   `json:"truncated"`
		} `json:"items"`
		Traced bool `json:"traced"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode the manifest: %v", err)
	}
	if !manifest.Traced {
		t.Fatal("traced = false, want the log's request reported")
	}
	reply := manifest.Items[len(manifest.Items)-1]
	if reply.Truncated || reply.BodyBytes != int64(len(whole)) {
		t.Fatalf("reply = %+v, want the whole body the log kept (%d bytes)", reply, len(whole))
	}

	path := "/api/v1/activity/requests/" + event + "/captures/" + strconv.FormatInt(ids[2], 10) + "/body"
	body := h.management(http.MethodGet, path, adminToken, nil)
	if body.Code != http.StatusOK {
		t.Fatalf("body status = %d, body = %s", body.Code, body.Body.String())
	}
	if got := body.Body.String(); got != whole {
		t.Fatalf("body = %d bytes, want the whole %d the log kept", len(got), len(whole))
	}
	if got := body.Header().Get("X-Relo-Truncated"); got != "false" {
		t.Fatalf("X-Relo-Truncated = %q, want the whole body reported", got)
	}
}

// TestCaptureReadsStayOnTheStateRowsWithoutATrace covers the other side: a
// request the log never wrote keeps its state-sized body and its own verdict.
func TestCaptureReadsStayOnTheStateRowsWithoutATrace(t *testing.T) {
	h := newHarness(t)
	journal := &journalTraffic{}
	traceJournal(t, h, journal)
	eventID, ids := seedCapturedRequest(t, h)
	event := strconv.FormatInt(eventID, 10)

	body := h.management(http.MethodGet,
		"/api/v1/activity/requests/"+event+"/captures/"+strconv.FormatInt(ids[2], 10)+"/body", adminToken, nil)
	if body.Code != http.StatusOK {
		t.Fatalf("body status = %d, body = %s", body.Code, body.Body.String())
	}
	if got := body.Body.String(); got != replyBody {
		t.Fatalf("body = %q, want the stored reply", got)
	}
	if got := body.Header().Get("X-Relo-Truncated"); got != "true" {
		t.Fatalf("X-Relo-Truncated = %q, want the stored verdict", got)
	}
}

// TestTrafficLogStaysOffUntilTheOperatorAsks covers the default: with the switch
// off the file is never written, whatever an operator does through the console.
func TestTrafficLogStaysOffUntilTheOperatorAsks(t *testing.T) {
	h := newHarness(t)
	journal := &journalTraffic{}
	h.server = h.newServerWithOptions(h.cfg, []account.PoolEntry{defaultEntry()}, func(options *server.Options) {
		options.Traffic = journal
	})

	response := h.dataPlane(http.MethodPost, "/v1/chat/completions", dataPlaneToken, strings.NewReader(streamingBody()))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	h.settle()
	if written := journal.kinds(); len(written) != 0 {
		t.Fatalf("halves = %v, want nothing written while the switch is off", written)
	}
}
