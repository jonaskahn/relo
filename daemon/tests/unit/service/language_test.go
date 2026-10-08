package service_test

import (
	"context"
	"errors"
	"testing"

	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	"github.com/jonaskahn/relo/internal/i18n"
)

func TestLanguageSettings(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()

	t.Run("the language is auto before an operator chooses", func(t *testing.T) {
		tag, err := harness.settings.Language()
		if err != nil || tag != i18n.Auto {
			t.Fatalf("Language() = %q, %v, want auto", tag, err)
		}
		settings, err := harness.settings.Read(ctx)
		if err != nil {
			t.Fatalf("Settings() error = %v", err)
		}
		if settings.Language != i18n.Auto {
			t.Fatalf("language = %q, want auto until an operator chooses", settings.Language)
		}
	})

	t.Run("a language is stored and read back", func(t *testing.T) {
		if err := harness.settings.SaveLanguage("de"); err != nil {
			t.Fatalf("SaveLanguage() error = %v", err)
		}
		tag, err := harness.settings.Language()
		if err != nil || tag != "de" {
			t.Fatalf("Language() = %q, %v, want de stored", tag, err)
		}
		settings, err := harness.settings.Read(ctx)
		if err != nil {
			t.Fatalf("Settings() error = %v", err)
		}
		if settings.Language != "de" {
			t.Fatalf("language = %q, want the stored choice", settings.Language)
		}
	})

	t.Run("a settings write carries the language", func(t *testing.T) {
		if err := harness.settings.SaveLanguage("zh-Hans"); err != nil {
			t.Fatalf("SaveLanguage() error = %v", err)
		}
		if tag, err := harness.settings.Language(); err != nil || tag != "zh-Hans" {
			t.Fatalf("Language() = %q, %v, want the saved choice", tag, err)
		}
	})

	t.Run("an empty language is read as auto", func(t *testing.T) {
		if err := harness.settings.SaveLanguage(""); err != nil {
			t.Fatalf("SaveLanguage() error = %v", err)
		}
		if tag, err := harness.settings.Language(); err != nil || tag != i18n.Auto {
			t.Fatalf("Language() = %q, %v, want auto for an empty field", tag, err)
		}
	})

	t.Run("a language Relo cannot speak is refused", func(t *testing.T) {
		err := harness.settings.SaveLanguage("sw")
		if err == nil {
			t.Fatal("SaveLanguage() error = nil, want a refusal")
		}
		if !errors.Is(err, appsettings.ErrInvalidWrite) {
			t.Fatalf("error = %v, want ErrInvalidSettingsWrite", err)
		}
		if tag, readErr := harness.settings.Language(); readErr != nil || tag == "sw" {
			t.Fatalf("Language() = %q, %v, want the stored language after a refused write", tag, readErr)
		}
	})
}
