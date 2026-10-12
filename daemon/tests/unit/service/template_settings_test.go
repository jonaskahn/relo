package service_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	apptemplates "github.com/jonaskahn/relo/internal/application/templatesettings"
	"github.com/jonaskahn/relo/tests/testkit"
)

func TestTemplateSettingsService(t *testing.T) {
	db := testkit.OpenTestDB(t)
	store := sqlite.NewTemplateSettings(db)
	service := apptemplates.New(apptemplates.Options{
		Store: choiceStore{inner: store}, Connections: staticTemplates{
			"claude": "claude", "codex": "openai-codex", "openai": "openai",
		},
		Clock: testkit.NewFakeClock(time.Unix(1_700_000_000, 0)),
	})
	ctx := context.Background()

	stored, err := service.Read(ctx, "claude")
	if err != nil || !stored.AutoRefresh {
		t.Fatalf("Read(claude) = %+v, %v, want refresh on", stored, err)
	}
	off := false
	saved, err := service.Update(ctx, "claude", apptemplates.Patch{AutoRefresh: &off})
	if err != nil || saved.AutoRefresh {
		t.Fatalf("Update() = %+v, %v, want refresh off", saved, err)
	}
	if _, err := service.Read(ctx, "missing"); !errors.Is(err, apptemplates.ErrNotFound) {
		t.Fatalf("Read(missing) error = %v, want not found", err)
	}
	if _, err := service.Update(ctx, "openai", apptemplates.Patch{AutoRefresh: &off}); !errors.Is(err, apptemplates.ErrUnsupported) {
		t.Fatalf("Update(openai) error = %v, want unsupported", err)
	}
}

// TestTemplateSettingsLeaveTheLongContextColumnAlone pins that the stored
// long-context choice is carried through a write untouched. Nothing reads it any
// more, so rewriting it would only discard whatever an earlier build recorded.
func TestTemplateSettingsLeaveTheLongContextColumnAlone(t *testing.T) {
	db := testkit.OpenTestDB(t)
	store := sqlite.NewTemplateSettings(db)
	on := true
	if err := store.Save(context.Background(), "claude", &on, nil, 1_700_000_000); err != nil {
		t.Fatalf("Save(long context on) error = %v", err)
	}
	service := apptemplates.New(apptemplates.Options{
		Store: choiceStore{inner: store}, Connections: staticTemplates{"claude": "claude"},
		Clock: testkit.NewFakeClock(time.Unix(1_700_000_000, 0)),
	})
	ctx := context.Background()

	off := false
	if _, err := service.Update(ctx, "claude", apptemplates.Patch{AutoRefresh: &off}); err != nil {
		t.Fatalf("Update(auto refresh) error = %v", err)
	}
	choice, found, err := store.Get(ctx, "claude")
	if err != nil || !found {
		t.Fatalf("Get() = %+v, %t, %v, want the stored row back", choice, found, err)
	}
	if !choice.LongContext {
		t.Fatal("Update() rewrote the long-context column, which nothing reads")
	}
}

type staticTemplates map[string]string

func (s staticTemplates) TemplateID(providerID string) (string, bool) {
	templateID, found := s[providerID]
	return templateID, found
}

// TestTemplateSettingsWithoutClockServesDefaults pins the nil-safe
// constructor: a service built without a clock still reads the template
// defaults.
func TestTemplateSettingsWithoutClockServesDefaults(t *testing.T) {
	db := testkit.OpenTestDB(t)
	service := apptemplates.New(apptemplates.Options{
		Store:       choiceStore{inner: sqlite.NewTemplateSettings(db)},
		Connections: staticTemplates{"claude": "claude"},
	})
	stored, err := service.Read(context.Background(), "claude")
	if err != nil || !stored.AutoRefresh {
		t.Fatalf("Read(claude) = %+v, %v, want refresh on", stored, err)
	}
}

// TestTemplateSettingsModelPrefixes covers the model families one template's
// roster keeps: they are normalized on the way in, the template default
// applies until a list is stored, and a list the roster cannot use is refused.
func TestTemplateSettingsModelPrefixes(t *testing.T) {
	newService := func(t *testing.T) *apptemplates.Service {
		t.Helper()
		return apptemplates.New(apptemplates.Options{
			Store: choiceStore{inner: sqlite.NewTemplateSettings(testkit.OpenTestDB(t))},
			Connections: staticTemplates{
				"claude": "claude", "antigravity": "google-antigravity",
			},
			Clock: testkit.NewFakeClock(time.Unix(1_700_000_000, 0)),
		})
	}
	ctx := context.Background()

	t.Run("a stored list is normalized and read back", func(t *testing.T) {
		service := newService(t)
		prefixes := []string{"Claude", " opus ", "claude", ""}
		saved, err := service.Update(ctx, "claude", apptemplates.Patch{ModelPrefixes: &prefixes})
		if err != nil {
			t.Fatalf("Update(prefixes) error = %v", err)
		}
		want := []string{"claude", "opus"}
		if !slices.Equal(saved.ModelPrefixes, want) {
			t.Fatalf("Update() prefixes = %+v, want %+v", saved.ModelPrefixes, want)
		}
		stored, err := service.Read(ctx, "claude")
		if err != nil || !slices.Equal(stored.ModelPrefixes, want) {
			t.Fatalf("Read(claude) = %+v, %v, want %+v", stored, err, want)
		}
	})

	t.Run("the template default applies until a list is stored", func(t *testing.T) {
		stored, err := newService(t).Read(ctx, "antigravity")
		if err != nil {
			t.Fatalf("Read(antigravity) error = %v", err)
		}
		if want := []string{"claude", "gemini"}; !slices.Equal(stored.ModelPrefixes, want) {
			t.Fatalf("Read(antigravity) prefixes = %+v, want the template default %+v", stored.ModelPrefixes, want)
		}
	})

	t.Run("a prefix the roster cannot use is refused", func(t *testing.T) {
		prefixes := []string{"claude code"}
		if _, err := newService(t).Update(ctx, "claude", apptemplates.Patch{ModelPrefixes: &prefixes}); !errors.Is(err, apptemplates.ErrInvalidPrefixes) {
			t.Fatalf("Update(prefixes) error = %v, want invalid prefixes", err)
		}
	})

	t.Run("an update that names no field reads the stored choice", func(t *testing.T) {
		stored, err := newService(t).Update(ctx, "claude", apptemplates.Patch{})
		if err != nil || !stored.AutoRefresh {
			t.Fatalf("Update(empty) = %+v, %v, want the stored choice", stored, err)
		}
	})
}

type choiceStore struct {
	inner *sqlite.TemplateSettings
}

func (s choiceStore) Get(ctx context.Context, templateID string) (bool, bool, bool, error) {
	choice, found, err := s.inner.Get(ctx, templateID)
	if err != nil || !found {
		return false, false, found, err
	}
	return choice.LongContext, choice.AutoRefresh, true, nil
}

func (s choiceStore) Save(ctx context.Context, templateID string, longContext, autoRefresh *bool, nowMs int64) error {
	return s.inner.Save(ctx, templateID, longContext, autoRefresh, nowMs)
}

func (s choiceStore) Prefixes(ctx context.Context, templateID string) ([]string, bool, error) {
	return s.inner.Prefixes(ctx, templateID)
}

func (s choiceStore) SavePrefixes(ctx context.Context, templateID string, prefixes []string, nowMs int64) error {
	return s.inner.SavePrefixes(ctx, templateID, prefixes, nowMs)
}
