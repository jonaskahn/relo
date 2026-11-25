package storage_test

import (
	"context"
	"testing"
	"time"

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

// TestThreadRetentionPreviewsAndSweepsTheWindow drives the pass the daemon
// runs on its maintenance tick: the preview counts what the window would
// delete, the sweep deletes it, and both measure from the same day.
func TestThreadRetentionPreviewsAndSweepsTheWindow(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, time.November, 20, 12, 0, 0, 0, time.UTC)
	db := testkit.OpenTestDB(t)
	turns := sqlite.NewThreadTurns(db)
	expired := storedTurn("msg_expired", "")
	expired.UpdatedAtMs = time.Date(2026, time.October, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	kept := storedTurn("msg_kept", "")
	kept.UpdatedAtMs = time.Date(2026, time.November, 19, 0, 0, 0, 0, time.UTC).UnixMilli()
	for _, turn := range []threads.Turn{expired, kept} {
		if err := turns.Put(ctx, turn); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}
	store := sqlite.NewThreadRetention(db, testkit.NewFakeClock(now))
	if err := store.SaveDays(ctx, 7); err != nil {
		t.Fatalf("SaveDays() error = %v", err)
	}
	preview, err := store.Preview(ctx, 7)
	if err != nil || preview != 1 {
		t.Fatalf("Preview() = %d, %v, want the one expired turn", preview, err)
	}
	if _, found, _ := turns.Get(ctx, "key-1", "msg_expired"); !found {
		t.Fatal("Preview() deleted a turn, want a count only")
	}
	deleted, err := store.Sweep(ctx)
	if err != nil || deleted != 1 {
		t.Fatalf("Sweep() = %d, %v, want the one expired turn", deleted, err)
	}
	if _, found, _ := turns.Get(ctx, "key-1", "msg_kept"); !found {
		t.Fatal("Sweep() removed a turn inside the window")
	}
	if !store.Now().Equal(now) {
		t.Fatalf("Now() = %v, want the store clock", store.Now())
	}
}

// TestThreadWindowOfZeroKeepsEveryTurn covers the unlimited choice: neither
// the preview nor the sweep deletes anything when the stored window is zero.
func TestThreadWindowOfZeroKeepsEveryTurn(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	turn := storedTurn("msg_old", "")
	turn.UpdatedAtMs = 1
	if err := sqlite.NewThreadTurns(db).Put(ctx, turn); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	store := sqlite.NewThreadRetention(db, nil)
	if err := store.SaveDays(ctx, 0); err != nil {
		t.Fatalf("SaveDays() error = %v", err)
	}
	if preview, err := store.Preview(ctx, 0); err != nil || preview != 0 {
		t.Fatalf("Preview(0) = %d, %v, want nothing", preview, err)
	}
	if deleted, err := store.Sweep(ctx); err != nil || deleted != 0 {
		t.Fatalf("Sweep() = %d, %v, want the unlimited window", deleted, err)
	}
}

// TestThreadStoreReportsAStoreFailure covers the wraps an operator reads when
// the database is gone: every read and write names the operation it failed.
func TestThreadStoreReportsAStoreFailure(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	turns := sqlite.NewThreadTurns(db)
	retention := sqlite.NewThreadRetention(db, nil)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, _, err := retention.Days(ctx); err == nil {
		t.Fatal("Days() = nil, want the read failure")
	}
	if err := retention.SaveDays(ctx, 7); err == nil {
		t.Fatal("SaveDays() = nil, want the write failure")
	}
	if _, err := retention.Sweep(ctx); err == nil {
		t.Fatal("Sweep() = nil, want the window read failure")
	}
	if err := turns.Put(ctx, storedTurn("msg_1", "")); err == nil {
		t.Fatal("Put() = nil, want the write failure")
	}
	if _, _, err := turns.Get(ctx, "key-1", "msg_1"); err == nil {
		t.Fatal("Get() = nil, want the read failure")
	}
	if _, err := turns.DeleteBefore(ctx, 1); err == nil {
		t.Fatal("DeleteBefore() = nil, want the delete failure")
	}
	if _, err := turns.CountBefore(ctx, 1); err == nil {
		t.Fatal("CountBefore() = nil, want the count failure")
	}
}

// TestThreadTurnWithNoSystemOrToolsStoresEmpty keeps a continued turn small:
// a turn that never opened the thread stores no prompt and no tool list.
func TestThreadTurnWithNoSystemOrToolsStoresEmpty(t *testing.T) {
	ctx := context.Background()
	store := sqlite.NewThreadTurns(testkit.OpenTestDB(t))
	turn := threads.Turn{ClientKey: "key-1", ID: "msg_2", ParentID: "msg_1", UpdatedAtMs: 1}
	if err := store.Put(ctx, turn); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	read, found, err := store.Get(ctx, "key-1", "msg_2")
	if err != nil || !found {
		t.Fatalf("Get() = found %v, error %v", found, err)
	}
	if read.System != nil || read.Tools != nil || read.Delta != nil {
		t.Fatalf("turn = %+v, want the empty fields kept empty", read)
	}
}

// TestThreadStoreRefusesATurnItCannotEncode covers the write the store owes the
// caller: a tool schema that is not JSON is refused rather than stored empty.
func TestThreadStoreRefusesATurnItCannotEncode(t *testing.T) {
	store := sqlite.NewThreadTurns(testkit.OpenTestDB(t))
	turn := storedTurn("msg_1", "")
	turn.Tools = []inference.Tool{{Name: "Broken", Parameters: []byte("{")}}
	if err := store.Put(context.Background(), turn); err == nil {
		t.Fatal("Put() = nil, want the encode refusal")
	}
}

// TestThreadStoreReportsATurnItCannotDecode covers a row written by an older
// build: the read refuses it rather than answering an empty turn.
func TestThreadStoreReportsATurnItCannotDecode(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	store := sqlite.NewThreadTurns(db)
	if _, err := db.SQL().ExecContext(ctx,
		`INSERT INTO thread_turns (client_key, id, delta, updated_at) VALUES ('key-1', 'msg_1', 'not json', 1)`); err != nil {
		t.Fatalf("seed the unreadable turn: %v", err)
	}
	if _, _, err := store.Get(ctx, "key-1", "msg_1"); err == nil {
		t.Fatal("Get() = nil, want the decode failure")
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
