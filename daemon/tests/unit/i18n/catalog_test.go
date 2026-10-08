package i18n_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/jonaskahn/relo/internal/i18n"
)

// catalogDir is where the catalogs live, relative to this package.
const catalogDir = "../../../internal/i18n/catalogs"

// message is one catalog entry as the files declare it.
type message struct {
	Description string
	One         string
	Other       string
}

// catalog reads one catalog file into its messages, keyed by dotted ID.
func catalog(t *testing.T, tag string) map[string]message {
	t.Helper()
	path := filepath.Join(catalogDir, "active."+tag+".toml")
	var decoded map[string]any
	if _, err := toml.DecodeFile(path, &decoded); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	messages := map[string]message{}
	flatten(decoded, "", messages)
	return messages
}

// flatten turns the nested tables of one catalog back into the dotted IDs
// the code localizes with.
func flatten(table map[string]any, prefix string, into map[string]message) {
	for key, value := range table {
		id := key
		if prefix != "" {
			id = prefix + "." + key
		}
		nested, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := nested["other"]; ok {
			into[id] = message{
				Description: asString(nested["description"]),
				One:         asString(nested["one"]),
				Other:       asString(nested["other"]),
			}
			continue
		}
		flatten(nested, id, into)
	}
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}

func TestCatalogsCoverEveryLanguage(t *testing.T) {
	loaded, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	supported := loaded.Supported()
	want := []string{
		"en", "de", "zh-Hans",
		"ar", "cs", "es", "fr", "hi", "id", "it", "ja", "ko",
		"nl", "pl", "pt-BR", "ro", "ru", "sv", "th", "tr", "uk", "vi",
		"zh-Hant",
	}
	if len(supported) != len(want) {
		t.Fatalf("Supported() = %v, want %d languages", supported, len(want))
	}
	for i, tag := range want {
		if supported[i] != tag {
			t.Fatalf("Supported() = %v, want %v", supported, want)
		}
	}
}

func TestCatalogsDeclareTheSameMessages(t *testing.T) {
	loaded, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	reference := catalog(t, "en")
	if len(reference) == 0 {
		t.Fatal("the English catalog is empty")
	}
	for _, tag := range loaded.Supported() {
		entries := catalog(t, tag)
		for id, translated := range entries {
			if strings.TrimSpace(translated.Other) == "" {
				t.Errorf("%s: %s has no other form", tag, id)
			}
			if tag == "en" {
				continue
			}
			if _, ok := reference[id]; !ok {
				t.Errorf("%s: %s is not in the English catalog", tag, id)
				continue
			}
			if tag == "de" && reference[id].One != "" && translated.One == "" {
				t.Errorf("de: %s has no one form although English does", id)
			}
		}
		for id := range reference {
			if _, ok := entries[id]; !ok {
				t.Errorf("%s: %s is missing from this catalog", tag, id)
			}
		}
	}
}

func TestTranslatorRendersEveryMessage(t *testing.T) {
	loaded, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	data := map[string]any{
		"Count": 2, "Active": 1, "Addr": "127.0.0.1:10101", "URL": "https://auth.test",
		"Code": "abcd", "Provider": "openai", "Label": "work", "ID": "abc123",
		"Status": "paused", "Priority": 5, "Detail": "no reason", "Prefix": "/agent/v1",
		"Path": "/tmp/config.toml", "Present": true, "Admin": "token", "Legacy": "gone",
		"List": "openai", "From": "openai", "To": "anthropic", "Reason": "spillover",
		"Duration": 12, "In": 1, "Out": 2, "CacheRead": 0, "CacheWrite": 0,
		"Client": "codex", "Default": "custom", "Events": 1, "Attempts": 1,
		"Days": 30, "Bytes": "1.0 MiB", "Size": "1.0 MiB", "Pages": 1, "PageSize": 4096,
		"FreePages": 0, "WAL": "0 B", "Deleted": 1, "Freed": "0 B", "Cursor": "c1",
		"Passed": 3, "Failed": 0, "Window": "5h", "Used": "10.0", "Source": "probe",
		"Languages": "en", "When": "now", "Name": "team", "Ordinal": 1,
		"Seconds": 5,
		"Value":   "1,234",
	}
	for _, tag := range loaded.Supported() {
		translator := loaded.Translate(tag)
		for id, entry := range catalog(t, "en") {
			rendered := translator.Text(id, data)
			if strings.TrimSpace(rendered) == "" {
				t.Errorf("%s: %s rendered empty", tag, id)
			}
			if strings.Contains(rendered, "{{") {
				t.Errorf("%s: %s rendered an unexpanded template: %q", tag, id, rendered)
			}
			if entry.One != "" && tag == "de" {
				singular := translator.Plural(id, 1, data)
				plural := translator.Plural(id, 3, data)
				if strings.Contains(singular, "{{") || strings.Contains(plural, "{{") {
					t.Errorf("%s: %s rendered an unexpanded plural: %q / %q", tag, id, singular, plural)
				}
			}
		}
	}
}

func TestTranslatorFallsBack(t *testing.T) {
	loaded, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	t.Run("a tag with no catalog renders English", func(t *testing.T) {
		if got := loaded.Translate("sw").Text("cli.daemon.status.running", nil); got != "Relo is running" {
			t.Fatalf("Text() = %q, want the English message", got)
		}
	})

	t.Run("an unknown message renders as its own ID", func(t *testing.T) {
		if got := loaded.Translate("de").Text("cli.nothing.here", nil); got != "cli.nothing.here" {
			t.Fatalf("Text() = %q, want the message ID", got)
		}
	})

	t.Run("catalogs that never loaded render the message ID", func(t *testing.T) {
		var unloaded *i18n.Catalogs
		if got := unloaded.Translate("de").Text("cli.daemon.status.running", nil); got != "cli.daemon.status.running" {
			t.Fatalf("Text() = %q, want the message ID", got)
		}
	})
}

func TestCatalogsMatchTheEmbeddedFiles(t *testing.T) {
	entries, err := os.ReadDir(catalogDir)
	if err != nil {
		t.Fatalf("read %s: %v", catalogDir, err)
	}
	loaded, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != len(loaded.Supported()) {
		t.Fatalf("catalog files = %d, want one per language (%d)", len(entries), len(loaded.Supported()))
	}
}
