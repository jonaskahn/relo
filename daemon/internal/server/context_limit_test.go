package server

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/routing"
)

func TestContextLimitCountsPromptAndRequestedOutput(t *testing.T) {
	server := New(Options{})
	request := &inference.Request{
		Messages: []inference.Message{{
			Role: inference.RoleUser,
			Content: []inference.ContentPart{{
				Type: inference.ContentTypeText, Text: strings.Repeat("token ", 3_000),
			}},
		}},
		MaxTokens: 1_000,
	}
	if !server.contextLimitExceeded(routing.Plan{ContextLimit: 1_000}, request) {
		t.Fatal("contextLimitExceeded() = false, want the request refused")
	}
	if server.contextLimitExceeded(routing.Plan{}, request) {
		t.Fatal("contextLimitExceeded() = true without a default-entry limit")
	}
}
