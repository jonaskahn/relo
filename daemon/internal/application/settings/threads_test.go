package settings

import (
	"context"
	"testing"

	"github.com/jonaskahn/relo/internal/threads"
)

// TestThreadWindowWithoutAStore keeps a build that carries no thread store
// readable: the card shows the default window, and a write is refused rather
// than silently dropped.
func TestThreadWindowWithoutAStore(t *testing.T) {
	service := New(Options{ConfigPath: t.TempDir() + "/config.toml"})
	settings := Settings{}
	if err := service.readThreads(context.Background(), &settings); err != nil {
		t.Fatalf("readThreads() error = %v", err)
	}
	if settings.Threads.Days != threads.DefaultRetentionDays {
		t.Fatalf("threads.days = %d, want the default", settings.Threads.Days)
	}
	if _, err := service.SaveThreads(context.Background(), ThreadRetention{Days: 7}); err == nil {
		t.Fatal("SaveThreads() = nil, want a refusal without a store")
	}
}
