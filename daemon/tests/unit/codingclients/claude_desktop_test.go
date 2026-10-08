package codingclients_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

func TestDesktopConfigLibraryFollowsTheApp(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "LOCALAPPDATA":
			return `C:\Users\ada\AppData\Local`
		case "APPDATA":
			return `C:\Users\ada\AppData\Roaming`
		case "XDG_CONFIG_HOME":
			return "/home/ada/.config"
		default:
			return ""
		}
	}
	cases := []struct {
		platform string
		home     string
		want     string
	}{
		{"darwin", "/Users/ada", "/Users/ada/Library/Application Support/Claude-3p/configLibrary"},
		{"linux", "/home/ada", "/home/ada/.config/Claude-3p/configLibrary"},
		{"windows", `C:\Users\ada`, `C:\Users\ada\AppData\Local\Claude-3p\configLibrary`},
	}
	for _, tc := range cases {
		got := codingclients.DesktopConfigLibrary(codingclients.DesktopPaths{
			Home: tc.home, Platform: tc.platform, Getenv: getenv,
		})
		if got != tc.want {
			t.Fatalf("%s library = %s, want %s", tc.platform, got, tc.want)
		}
	}
	explicit := codingclients.DesktopConfigLibrary(codingclients.DesktopPaths{
		Home: "/Users/ada", Platform: "darwin",
		Getenv: func(key string) string {
			if key == "CLAUDE_USER_DATA_DIR" {
				return "/tmp/claude"
			}
			return ""
		},
	})
	if explicit != "/tmp/claude/configLibrary" {
		t.Fatalf("explicit library = %s", explicit)
	}
}

func TestWriteDesktopProfileRestoresThePreviousSelection(t *testing.T) {
	library := t.TempDir()
	user := filepath.Join(library, "user.json")
	if err := os.WriteFile(user, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write the previous profile: %v", err)
	}
	meta := map[string]any{
		"appliedId": "user",
		"entries":   []any{map[string]any{"id": "user", "name": "Personal"}},
		"theme":     "dark",
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("encode metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(library, "_meta.json"), encoded, 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	window := int64(1_000_000)
	_, content, err := codingclients.WriteDesktopProfile(library, "http://127.0.0.1:10202", "rlo_ak_test", []codingclients.ModelRef{
		{ID: "claude-opus", Name: "Opus", ContextWindow: &window},
	})
	if err != nil {
		t.Fatalf("WriteDesktopProfile() error = %v", err)
	}
	var profile map[string]any
	if err := json.Unmarshal(content, &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if profile["inferenceGatewayBaseUrl"] != "http://127.0.0.1:10202" || profile["modelDiscoveryEnabled"] != false {
		t.Fatalf("profile = %#v", profile)
	}
	models, _ := profile["inferenceModels"].([]any)
	if len(models) != 1 {
		t.Fatalf("models = %#v", profile["inferenceModels"])
	}
	selected := readDesktopMeta(t, library)
	if selected["appliedId"] != desktopProfileID(library) || selected["theme"] != "dark" {
		t.Fatalf("metadata = %#v", selected)
	}
	if err := codingclients.RemoveDesktopProfile(library); err != nil {
		t.Fatalf("RemoveDesktopProfile() error = %v", err)
	}
	if _, err := os.Stat(codingclients.DesktopProfilePath(library)); !os.IsNotExist(err) {
		t.Fatalf("profile remove error = %v, want gone", err)
	}
	restored := readDesktopMeta(t, library)
	if restored["appliedId"] != "user" || restored["theme"] != "dark" {
		t.Fatalf("restored metadata = %#v", restored)
	}
	if _, err := os.Stat(user); err != nil {
		t.Fatalf("the previous profile is gone: %v", err)
	}
}

// desktopProfileID is the id Relo's profile file is named after.
func desktopProfileID(library string) string {
	return strings.TrimSuffix(filepath.Base(codingclients.DesktopProfilePath(library)), ".json")
}

func readDesktopMeta(t *testing.T, library string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(library, "_meta.json"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	return document
}

func TestDesktopInstalledRecognisesTheAppBeforeThirdPartyModeRan(t *testing.T) {
	home := t.TempDir()
	paths := func(env map[string]string) codingclients.DesktopPaths {
		return codingclients.DesktopPaths{
			Home: home, Platform: "darwin", Getenv: func(key string) string { return env[key] },
		}
	}
	support := filepath.Join(home, "Library", "Application Support")

	if err := codingclients.DesktopInstalledAt(paths(nil)); !errors.Is(err, codingclients.ErrNotInstalled) {
		t.Fatalf("no Desktop directory: error = %v, want %v", err, codingclients.ErrNotInstalled)
	}

	if err := os.MkdirAll(filepath.Join(support, "Claude"), 0o755); err != nil {
		t.Fatalf("create the Desktop directory: %v", err)
	}
	if err := codingclients.DesktopInstalledAt(paths(nil)); err != nil {
		t.Fatalf("Desktop directory only: error = %v, want installed", err)
	}
}

func TestDesktopInstalledAcceptsTheThirdPartyDirectoryAndAnExplicitOne(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "Library", "Application Support", "Claude-3p"), 0o755); err != nil {
		t.Fatalf("create the third-party directory: %v", err)
	}
	if err := codingclients.DesktopInstalledAt(codingclients.DesktopPaths{Home: home, Platform: "darwin"}); err != nil {
		t.Fatalf("third-party directory only: error = %v, want installed", err)
	}

	explicit := t.TempDir()
	err := codingclients.DesktopInstalledAt(codingclients.DesktopPaths{
		Home: t.TempDir(), Platform: "darwin",
		Getenv: func(key string) string {
			if key == "CLAUDE_USER_DATA_DIR" {
				return explicit
			}
			return ""
		},
	})
	if err != nil {
		t.Fatalf("explicit directory: error = %v, want installed", err)
	}
}

// desktopProfileIDPattern is what Claude Desktop accepts for an applied profile.
var desktopProfileIDPattern = regexp.MustCompile(`^[a-f0-9-]{36}$`)

// desktopVendorWords is the list Claude Desktop uses to refuse a gateway model
// name that belongs to another vendor.
var desktopVendorWords = regexp.MustCompile(`ark-code|astron|command-r|deepseek|doubao|gemini|gemma|glm|gpt|grok|hermes|hy3|kimi|lfm|\bling\b|llama|longcat|mimo|minimax|mistral|mixtral|moonshot|nemotron|openai|phi-|qianfan|qwen|tc-code|\bunic\b|yi-|stepfun|step-3|seed-|bytedance|hunyuan|granite|amazon\.nova|nova-|devstral|ministral|ernie|codex|arcee|trinity|abab|phi\d|\bk2\.|\bm2\.|jamba|arctic|solar|mercury|zamba|kat-coder|\bds-|dpsk`)

func TestDesktopProfileIDIsAUUIDDesktopReads(t *testing.T) {
	library := t.TempDir()
	if id := desktopProfileID(library); !desktopProfileIDPattern.MatchString(id) {
		t.Fatalf("profile id %q is not one Desktop reads", id)
	}
	if _, _, err := codingclients.WriteDesktopProfile(library, "http://127.0.0.1:10202", "rlo_ak_test", nil); err != nil {
		t.Fatalf("WriteDesktopProfile() error = %v", err)
	}
	if applied := readDesktopMeta(t, library)["appliedId"]; applied != desktopProfileID(library) {
		t.Fatalf("appliedId = %v, want the profile id", applied)
	}
}

func TestSelectDesktopProfileReplacesTheProfileAnEarlierBuildWrote(t *testing.T) {
	library := t.TempDir()
	legacy := filepath.Join(library, "relo.json")
	if err := os.WriteFile(legacy, []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write the legacy profile: %v", err)
	}
	meta := `{"appliedId":"relo","entries":[{"id":"user","name":"Personal"},{"id":"relo","name":"relo","previousAppliedId":"user"}]}`
	if err := os.WriteFile(filepath.Join(library, "_meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if _, _, err := codingclients.WriteDesktopProfile(library, "http://127.0.0.1:10202", "rlo_ak_test", nil); err != nil {
		t.Fatalf("WriteDesktopProfile() error = %v", err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatalf("legacy profile error = %v, want gone", err)
	}
	document := readDesktopMeta(t, library)
	id := desktopProfileID(library)
	entries, _ := document["entries"].([]any)
	if document["appliedId"] != id || len(entries) != 2 {
		t.Fatalf("metadata = %#v", document)
	}
	owned, _ := entries[1].(map[string]any)
	if owned["id"] != id || owned["previousAppliedId"] != "user" {
		t.Fatalf("owned entry = %#v, want the new id and the remembered selection", owned)
	}

	if err := codingclients.RemoveDesktopProfile(library); err != nil {
		t.Fatalf("RemoveDesktopProfile() error = %v", err)
	}
	if restored := readDesktopMeta(t, library); restored["appliedId"] != "user" {
		t.Fatalf("restored metadata = %#v, want the previous profile", restored)
	}
}

func TestRemoveDesktopProfileRestoresFromAnEarlierBuildsEntry(t *testing.T) {
	library := t.TempDir()
	meta := `{"appliedId":"relo","entries":[{"id":"user","name":"Personal"},{"id":"relo","name":"relo","previousAppliedId":"user"}]}`
	if err := os.WriteFile(filepath.Join(library, "_meta.json"), []byte(meta), 0o600); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(library, "relo.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write the legacy profile: %v", err)
	}
	if err := codingclients.RemoveDesktopProfile(library); err != nil {
		t.Fatalf("RemoveDesktopProfile() error = %v", err)
	}
	if restored := readDesktopMeta(t, library); restored["appliedId"] != "user" {
		t.Fatalf("restored metadata = %#v, want the previous profile", restored)
	}
	if _, err := os.Stat(filepath.Join(library, "relo.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy profile error = %v, want gone", err)
	}
}

func TestDesktopModelNamesAvoidTheVendorWordsDesktopRefuses(t *testing.T) {
	window := int64(1_000_000)
	ids := []string{
		"claude-relo-openai-codex--gpt-5.5",
		"claude-relo-grok--grok-4.7",
		"claude-relo-opencode-go--deepseek-flash[1m]",
		"claude-reloc-smart-combo",
		"claude-relo-anthropic--claude-opus-4-8",
	}
	models := make([]codingclients.ModelRef, 0, len(ids)+1)
	for _, id := range ids {
		models = append(models, codingclients.ModelRef{ID: id, Name: id, ContextWindow: &window})
	}
	models = append(models, codingclients.ModelRef{ID: ids[0], Name: "duplicate"})
	content, err := codingclients.DesktopProfileContent("http://127.0.0.1:10202", "rlo_ak_test", models)
	if err != nil {
		t.Fatalf("DesktopProfileContent() error = %v", err)
	}
	var profile struct {
		Models []map[string]any `json:"inferenceModels"`
	}
	if err := json.Unmarshal(content, &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if len(profile.Models) != len(ids) {
		t.Fatalf("models = %d, want %d without the duplicate", len(profile.Models), len(ids))
	}
	shape := regexp.MustCompile(`^claude-opus-4-8-r[0-9a-f]{12}$`)
	for _, row := range profile.Models {
		name, _ := row["name"].(string)
		if !shape.MatchString(name) || desktopVendorWords.MatchString(name) {
			t.Fatalf("name %q is not one Desktop accepts", name)
		}
		if row["supports1m"] != true || row["prefer1m"] != true {
			t.Fatalf("row = %#v, want the 1M flags", row)
		}
	}
}
