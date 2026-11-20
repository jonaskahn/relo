package storage_test

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/inference"
	"github.com/jonaskahn/relo/internal/threads"
	"github.com/jonaskahn/relo/tests/testkit"
)

func storedTurn(id, parent string) threads.Turn {
	return threads.Turn{
		ClientKey: "key-1", ID: id, ParentID: parent,
		Delta: []inference.Message{{
			Role:    inference.RoleUser,
			Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "hello " + id}},
		}},
		System: &inference.Message{
			Role:    inference.RoleSystem,
			Content: []inference.ContentPart{{Type: inference.ContentTypeText, Text: "be careful"}},
		},
		Tools:       []inference.Tool{{Name: "Read", Description: "read"}},
		UpdatedAtMs: 1_700_000_000_000,
	}
}

// TestThreadTurnsRoundTrip drives the store the way the data plane does: a
// turn is written, read back with every field, and the history above it
// rebuilds the conversation in order.
func TestThreadTurnsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	store := sqlite.NewThreadTurns(db)
	first := storedTurn("msg_1", "")
	second := storedTurn("msg_2", "msg_1")
	second.System, second.Tools = nil, nil
	for _, turn := range []threads.Turn{first, second} {
		if err := store.Put(ctx, turn); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}

	read, found, err := store.Get(ctx, "key-1", "msg_1")
	if err != nil || !found {
		t.Fatalf("Get() = found %v, error %v", found, err)
	}
	if read.System == nil || read.System.Content[0].Text != "be careful" {
		t.Fatalf("system = %+v, want the stored prompt", read.System)
	}
	if len(read.Tools) != 1 || read.Tools[0].Name != "Read" {
		t.Fatalf("tools = %+v, want the stored tool", read.Tools)
	}
	if len(read.Delta) != 1 || read.Delta[0].Content[0].Text != "hello msg_1" {
		t.Fatalf("delta = %+v, want the stored message", read.Delta)
	}

	history := threads.NewHistory(store, nil)
	resolved, found, err := history.Resolve(ctx, "key-1", "msg_2")
	if err != nil || !found {
		t.Fatalf("Resolve() = found %v, error %v", found, err)
	}
	if len(resolved.Messages) != 2 || resolved.System == nil || len(resolved.Tools) != 1 {
		t.Fatalf("resolved = %+v, want the chain with its prompt and tools", resolved)
	}
}

func TestThreadTurnsAreScopedToTheClientKey(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewThreadTurns(testkit.OpenTestDB(t))
	if err := store.Put(ctx, storedTurn("msg_1", "")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if _, found, err := store.Get(ctx, "another-key", "msg_1"); err != nil || found {
		t.Fatalf("Get(other key) = found %v, error %v, want not found", found, err)
	}
}

func TestThreadTurnsDeleteWhatTheWindowOutlived(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewThreadTurns(testkit.OpenTestDB(t))
	old := storedTurn("msg_old", "")
	old.UpdatedAtMs = 1_000
	fresh := storedTurn("msg_fresh", "")
	fresh.UpdatedAtMs = 2_000
	for _, turn := range []threads.Turn{old, fresh} {
		if err := store.Put(ctx, turn); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}
	if count, err := store.CountBefore(ctx, 1_500); err != nil || count != 1 {
		t.Fatalf("CountBefore() = %d, %v, want the one old turn", count, err)
	}
	deleted, err := store.DeleteBefore(ctx, 1_500)
	if err != nil || deleted != 1 {
		t.Fatalf("DeleteBefore() = %d, %v, want to delete the old turn", deleted, err)
	}
	if _, found, _ := store.Get(ctx, "key-1", "msg_fresh"); !found {
		t.Fatal("DeleteBefore() removed a turn inside the window")
	}
}

func TestThreadRetentionStoresTheWindow(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewThreadRetention(testkit.OpenTestDB(t), nil)
	days, stored, err := store.Days(ctx)
	if err != nil || stored {
		t.Fatalf("Days() = %d, stored %v, error %v, want the default", days, stored, err)
	}
	if days != threads.DefaultRetentionDays {
		t.Fatalf("default days = %d, want %d", days, threads.DefaultRetentionDays)
	}
	if err := store.SaveDays(ctx, 7); err != nil {
		t.Fatalf("SaveDays() error = %v", err)
	}
	if days, stored, err := store.Days(ctx); err != nil || !stored || days != 7 {
		t.Fatalf("Days() after save = %d, stored %v, error %v", days, stored, err)
	}
	if err := store.SaveDays(ctx, -1); err == nil {
		t.Fatal("SaveDays(-1) = nil, want a refusal")
	}
}
