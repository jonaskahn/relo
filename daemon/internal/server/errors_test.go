package server

import (
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/jonaskahn/relo/internal/routing"
)

// TestChatTargetRefusalsAreInspectable covers the setup-time refusals with
// errors.Is, so a caller can tell a missing target from a missing model.
func TestChatTargetRefusalsAreInspectable(t *testing.T) {
	format := chatTesterRequest{Mode: chatModeFormat}
	if _, err := format.resolveTarget(); !errors.Is(err, ErrChatTargetRequired) {
		t.Fatalf("format target without route = %v, want ErrChatTargetRequired", err)
	}
	direct := chatTesterRequest{Mode: chatModeDirect}
	if _, err := direct.resolveTarget(); !errors.Is(err, ErrChatModelRequired) {
		t.Fatalf("direct target without model = %v, want ErrChatModelRequired", err)
	}
	empty := chatTesterRequest{Mode: chatModeDirect, ProviderID: "openai", ModelID: "gpt-4o"}
	if _, err := empty.canonical(t.Context(), chatCompletionsSurface(), nil, "openai/gpt-4o"); !errors.Is(err, ErrChatMessageRequired) {
		t.Fatalf("canonical without messages = %v, want ErrChatMessageRequired", err)
	}
}

// TestLogQueryRefusalsAreInspectable covers the log-filter refusals with
// errors.Is, so a caller can tell a bad timestamp from a bad limit.
func TestLogQueryRefusalsAreInspectable(t *testing.T) {
	request := httptest.NewRequest("GET", "/api/v1/activity/requests?since_ms=nope", nil)
	if _, err := logsQuery(request); !errors.Is(err, ErrLogSinceRequired) {
		t.Fatalf("logsQuery bad since = %v, want ErrLogSinceRequired", err)
	}
	request = httptest.NewRequest("GET", "/api/v1/activity/requests?limit=nope", nil)
	if _, err := logsQuery(request); !errors.Is(err, ErrLogLimitRequired) {
		t.Fatalf("logsQuery bad limit = %v, want ErrLogLimitRequired", err)
	}
}

// TestSkipCauseKeepsItsWords covers the routing-skip wrapper: the composed
// error stays under ErrNoRoute while the skip reason keeps its own words.
func TestSkipCauseKeepsItsWords(t *testing.T) {
	plan := routing.Plan{Skipped: []routing.Skipped{{Reason: "paused for maintenance"}}}
	cause := skipCause(plan)
	if cause == nil || cause.Error() != "paused for maintenance" {
		t.Fatalf("skipCause = %v, want the skip reason verbatim", cause)
	}
}
