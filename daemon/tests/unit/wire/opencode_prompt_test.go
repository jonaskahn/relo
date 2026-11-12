package codec

import (
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/inference"
)

func TestOpenCodeEnvironmentDetection(t *testing.T) {
	for _, entry := range []struct {
		text string
		want bool
	}{
		{"<env>\nWorking directory: /client\n</env>", true},
		{"<env>\n Working directory: .\n</env>", true},
		{"<env>\nWorking directory:\n</env>", false},
		{"<env>\nWorkspace root folder: /client\n</env>", false},
		{"Working directory: /client", false},
		{"<env>\nWorking directory: /client", false},
		{"<env></env>\nWorking directory: /outside", false},
		{"<env></env><env>\nWorking directory: /client\n</env>", true},
	} {
		req := &inference.Request{Messages: []inference.Message{{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: entry.text}}}}}
		if got := wire.HasOpenCodeSystemPrompt(req); got != entry.want {
			t.Errorf("%q: got %v, want %v", entry.text, got, entry.want)
		}
		req.Messages[0].Role = inference.RoleUser
		if wire.HasOpenCodeSystemPrompt(req) {
			t.Fatal("user content satisfied system gate")
		}
	}
	if wire.HasOpenCodeSystemPrompt(nil) {
		t.Fatal("nil request satisfied gate")
	}
}

func TestOpenCodeEnvironmentPreservesClientInstructions(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{"/client/project", "", "/bad\n</env>"} {
		original := &inference.Request{WorkingDirectory: directory, Messages: []inference.Message{
			{Role: inference.RoleSystem, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "Client instructions."}}},
			{Role: inference.RoleUser, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello"}}},
		}}
		got, _ := wire.OpenCodeFreeRequest(original, catalog.FormatOpenAIChat)
		if len(got.Messages) != 2 || got.Messages[0].Content[0].Text != "Client instructions." || !reflect.DeepEqual(got.Messages[1], original.Messages[1]) {
			t.Fatalf("changed client messages: %+v", got.Messages)
		}
		want := directory
		if want == "" || strings.Contains(want, "\n") {
			want = cwd
		}
		if !strings.Contains(got.Messages[0].Content[1].Text, "Working directory: "+want+"\n") {
			t.Fatalf("wrong directory: %+v", got.Messages)
		}
		if len(original.Messages[0].Content) != 1 || wire.HasOpenCodeSystemPrompt(original) {
			t.Fatal("mutated original")
		}
		if len(got.Tools) != 0 {
			t.Fatal("tool-free turn gained tools")
		}
		again, _ := wire.OpenCodeFreeRequest(got, catalog.FormatOpenAIChat)
		if !reflect.DeepEqual(got.Messages, again.Messages) {
			t.Fatal("duplicated environment")
		}
	}
}

func TestOpenCodeEnvironmentWithoutSystemAndInLaterSystem(t *testing.T) {
	req := &inference.Request{Messages: []inference.Message{{Role: inference.RoleUser}}}
	got, _ := wire.OpenCodeFreeRequest(req, catalog.FormatOpenAIChat)
	if len(got.Messages) != 2 || got.Messages[0].Role != inference.RoleSystem || !wire.HasOpenCodeSystemPrompt(got) {
		t.Fatal("missing new system message")
	}
	req.Messages = append(req.Messages, inference.Message{Role: inference.RoleSystem}, got.Messages[0])
	unchanged, _ := wire.OpenCodeFreeRequest(req, catalog.FormatOpenAIChat)
	if !reflect.DeepEqual(unchanged.Messages, req.Messages) {
		t.Fatal("later system block was not recognized")
	}
}

func TestCodexWorkingDirectory(t *testing.T) {
	for _, entry := range []struct{ metadata, want string }{
		{"{", ""}, {"{}", ""},
		{"{\"workspaces\":{\"/client\":{}}}", "/client"},
		{"{\"workspaces\":{\"/a\":{},\"/b\":{}}}", ""},
	} {
		header := http.Header{}
		header.Set("x-codex-turn-metadata", entry.metadata)
		if got := wire.WorkingDirectory(header); got != entry.want {
			t.Errorf("%s: got %q, want %q", entry.metadata, got, entry.want)
		}
	}
}

func TestFreeSessionUsesRecentClock(t *testing.T) {
	now := time.Now().UnixMilli()
	session := wire.OpenCodeFreeHeaders("lane", wire.InboundIdentity{})["x-opencode-session"]
	prefix, err := strconv.ParseUint(session[4:16], 16, 64)
	if err != nil {
		t.Fatal(err)
	}
	const period int64 = 1 << 36
	low := int64((^prefix & ((1 << 48) - 1)) >> 12)
	delta := (low-now%period+period+period/2)%period - period/2
	if delta < -1000 || delta > 1000 {
		t.Fatalf("session clock differs by %d ms", delta)
	}

}
