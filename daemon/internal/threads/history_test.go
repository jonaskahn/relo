package threads

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/inference"
)

type recordingStore struct {
	turns   map[string]Turn
	deleted int64
}

func (r *recordingStore) Put(_ context.Context, turn Turn) error {
	if r.turns == nil {
		r.turns = map[string]Turn{}
	}
	r.turns[turn.ClientKey+"/"+turn.ID] = turn
	return nil
}

func (r *recordingStore) Get(_ context.Context, clientKey, id string) (Turn, bool, error) {
	turn, found := r.turns[clientKey+"/"+id]
	return turn, found, nil
}

func (r *recordingStore) DeleteBefore(context.Context, int64) (int64, error) { return 0, nil }
func (r *recordingStore) CountBefore(context.Context, int64) (int64, error)  { return r.deleted, nil }

func messages(role, text string) inference.Message {
	return inference.Message{Role: role, Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: text}}}
}

func TestResolveRebuildsTheWholeChainInOrder(t *testing.T) {
	store := &recordingStore{}
	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	history := NewHistory(store, func() time.Time { return now })
	tools := []inference.Tool{{Name: "Read"}}
	system := messages(inference.RoleSystem, "be careful")
	first := Turn{ClientKey: "k", ID: "msg_1", Delta: []inference.Message{
		messages(inference.RoleUser, "one"), messages(inference.RoleAssistant, "two"),
	}, System: &system, Tools: tools}
	second := Turn{ClientKey: "k", ID: "msg_2", ParentID: "msg_1", Delta: []inference.Message{
		messages(inference.RoleUser, "three"), messages(inference.RoleAssistant, "four"),
	}}
	for _, turn := range []Turn{first, second} {
		if err := history.Record(context.Background(), turn); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}

	resolved, found, err := history.Resolve(context.Background(), "k", "msg_2")
	if err != nil || !found {
		t.Fatalf("Resolve() = found %v, error %v", found, err)
	}
	if len(resolved.Messages) != 4 {
		t.Fatalf("messages = %d, want the whole chain", len(resolved.Messages))
	}
	if resolved.Messages[0].Content[0].Text != "one" || resolved.Messages[3].Content[0].Text != "four" {
		t.Fatalf("messages = %+v, want the chain in order", resolved.Messages)
	}
	if resolved.System == nil || resolved.System.Content[0].Text != "be careful" {
		t.Fatalf("system = %+v, want the thread's own prompt", resolved.System)
	}
	if len(resolved.Tools) != 1 || resolved.Tools[0].Name != "Read" {
		t.Fatalf("tools = %+v, want the thread's tool list", resolved.Tools)
	}
	if resolved.NodeID != "msg_2" {
		t.Fatalf("node id = %q, want the reply that was continued", resolved.NodeID)
	}
}

func TestResolveReportsAMissingTurn(t *testing.T) {
	history := NewHistory(&recordingStore{}, nil)
	if _, found, err := history.Resolve(context.Background(), "k", "msg_missing"); err != nil || found {
		t.Fatalf("Resolve(missing) = found %v, error %v, want not found", found, err)
	}
}

func TestResolveStopsACycle(t *testing.T) {
	store := &recordingStore{turns: map[string]Turn{
		"k/msg_1": {ClientKey: "k", ID: "msg_1", ParentID: "msg_2"},
		"k/msg_2": {ClientKey: "k", ID: "msg_2", ParentID: "msg_1"},
	}}
	history := NewHistory(store, nil)
	if _, found, _ := history.Resolve(context.Background(), "k", "msg_1"); found {
		t.Fatal("a cyclic chain resolved, want the walk to stop")
	}
}

func TestResolveWalksAtMostTheChainLimit(t *testing.T) {
	store := &recordingStore{}
	history := NewHistory(store, nil)
	for index := 0; index < maxChain+1; index++ {
		turn := Turn{ClientKey: "k", ID: "msg_" + itoa(index), Delta: []inference.Message{messages(inference.RoleUser, "x")}}
		if index > 0 {
			turn.ParentID = "msg_" + itoa(index-1)
		}
		if err := history.Record(context.Background(), turn); err != nil {
			t.Fatalf("Record() error = %v", err)
		}
	}
	if _, found, _ := history.Resolve(context.Background(), "k", "msg_"+itoa(maxChain)); found {
		t.Fatal("a chain past the limit resolved, want the walk to stop")
	}
}

func TestRetentionWindowDefaultsAndRejectsNegative(t *testing.T) {
	if days, err := RetentionWindow(0); err != nil || days != DefaultRetentionDays {
		t.Fatalf("RetentionWindow(0) = %d, %v, want the default window", days, err)
	}
	if _, err := RetentionWindow(-1); err == nil {
		t.Fatal("RetentionWindow(-1) = nil, want a refusal")
	}
}

func TestCutoffIsMeasuredFromTheStartOfTheDay(t *testing.T) {
	now := time.Date(2026, time.November, 20, 23, 59, 0, 0, time.UTC)
	want := time.Date(2026, time.November, 14, 0, 0, 0, 0, time.UTC).UnixMilli()
	if got := CutoffMs(7, now); got != want {
		t.Fatalf("CutoffMs(7) = %d, want %d", got, want)
	}
	if CutoffMs(0, now) != 0 {
		t.Fatal("CutoffMs(0) = non-zero, want the unlimited window")
	}
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}
