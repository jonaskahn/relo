package codingclients_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/jonaskahn/relo/internal/adapters/codingclients"
)

const (
	baseURL    = "http://127.0.0.1:10201"
	helperFile = "/tmp/relo-codex-helper"
)

// TestMergeCodexConfigKeepsTheOperatorsFile is the promise a merge makes:
// Relo's provider appears, the operator's own rows survive, and the file still
// parses as TOML.
func TestMergeCodexConfigKeepsTheOperatorsFile(t *testing.T) {
	existing := strings.Join([]string{
		"# my codex config",
		"model_provider = \"openai\"",
		"approval_policy = \"never\"",
		"",
		"[model_providers.azure]",
		"name = \"Azure\"",
		"base_url = \"https://example.test/v1\"",
		"",
	}, "\n")
	merged, err := codingclients.MergeCodexConfig(existing, baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	document := map[string]any{}
	if err := toml.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("the merged file does not parse: %v\n%s", err, merged)
	}
	if document["model_provider"] != "relo" {
		t.Fatalf("model_provider = %v, want relo", document["model_provider"])
	}
	if document["approval_policy"] != "never" {
		t.Fatalf("approval_policy = %v, want the operator's own value kept", document["approval_policy"])
	}
	providers, ok := document["model_providers"].(map[string]any)
	if !ok {
		t.Fatalf("model_providers = %T, want a table", document["model_providers"])
	}
	if _, found := providers["azure"]; !found {
		t.Fatal("the operator's azure provider is gone")
	}
	relo, found := providers["relo"].(map[string]any)
	if !found {
		t.Fatalf("model_providers.relo is missing from %s", merged)
	}
	if _, found := relo["env_key"]; found {
		t.Fatalf("env_key = %v, want the key read from a command", relo["env_key"])
	}
	auth, ok := relo["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth = %T, want the command that prints the key", relo["auth"])
	}
	if !strings.Contains(fmt.Sprint(auth["command"], auth["args"]), helperFile) {
		t.Fatalf("auth = %v, want the key helper %s", auth, helperFile)
	}
	if relo["base_url"] != baseURL+"/v1" {
		t.Fatalf("base_url = %v, want %s/v1", relo["base_url"], baseURL)
	}
	if strings.Count(merged, "model_provider =") != 1 {
		t.Fatalf("the merged file carries %d model_provider assignments, want one\n%s",
			strings.Count(merged, "model_provider ="), merged)
	}
}

// TestMergeCodexConfigIsIdempotent is what makes a repair safe: merging a file
// Relo already wrote changes nothing.
func TestMergeCodexConfigIsIdempotent(t *testing.T) {
	first, err := codingclients.MergeCodexConfig("", baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	second, err := codingclients.MergeCodexConfig(first, baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if first != second {
		t.Fatalf("a second merge changed the file:\n%s\n---\n%s", first, second)
	}
}

// TestMergeCodexConfigRefusesUnparseableInput reports the honest refusal: a
// file Relo cannot reason about is left alone.
func TestMergeCodexConfigRefusesUnparseableInput(t *testing.T) {
	if _, err := codingclients.MergeCodexConfig("[broken\nkey = \"unterminated", baseURL, "", helperFile, true); err == nil {
		t.Fatal("MergeCodexConfig() error = nil, want a refusal")
	}
	if _, err := codingclients.MergeCodexConfig("", "", "", helperFile, true); err == nil {
		t.Fatal("MergeCodexConfig() with no address error = nil, want a refusal")
	}
	if _, err := codingclients.MergeCodexConfig("", baseURL, "", "", true); err == nil {
		t.Fatal("MergeCodexConfig() with no key helper error = nil, want a refusal")
	}
}

// TestMergeCodexConfigAddsWindowDefaultsOnce writes the context window and
// compact limit when the file has none, and leaves a value the operator
// already chose. A second merge does not add another copy.
func TestMergeCodexConfigAddsWindowDefaultsOnce(t *testing.T) {
	merged, err := codingclients.MergeCodexConfig("approval_policy = \"never\"\n", baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	document := map[string]any{}
	if err := toml.Unmarshal([]byte(merged), &document); err != nil {
		t.Fatalf("the merged file does not parse: %v\n%s", err, merged)
	}
	if document["model_context_window"] != int64(1_000_000) {
		t.Fatalf("model_context_window = %v, want 1000000", document["model_context_window"])
	}
	if document["model_auto_compact_token_limit"] != int64(900_000) {
		t.Fatalf("model_auto_compact_token_limit = %v, want 900000", document["model_auto_compact_token_limit"])
	}
	if strings.Count(merged, "model_context_window") != 1 || strings.Count(merged, "model_auto_compact_token_limit") != 1 {
		t.Fatalf("window defaults were duplicated:\n%s", merged)
	}
	again, err := codingclients.MergeCodexConfig(merged, baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if again != merged {
		t.Fatalf("a second merge duplicated the window defaults:\n%s", again)
	}

	custom := "model_context_window = 128000\nmodel_auto_compact_token_limit = 64000\n"
	kept, err := codingclients.MergeCodexConfig(custom, baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if strings.Count(kept, "model_context_window") != 1 || strings.Count(kept, "model_auto_compact_token_limit") != 1 {
		t.Fatalf("an existing window was duplicated:\n%s", kept)
	}
	keptDocument := map[string]any{}
	if err := toml.Unmarshal([]byte(kept), &keptDocument); err != nil {
		t.Fatalf("the merged file does not parse: %v\n%s", err, kept)
	}
	if keptDocument["model_context_window"] != int64(128_000) || keptDocument["model_auto_compact_token_limit"] != int64(64_000) {
		t.Fatalf("window = %v / %v, want the operator's values",
			keptDocument["model_context_window"], keptDocument["model_auto_compact_token_limit"])
	}

	partial, err := codingclients.MergeCodexConfig("model_context_window = 128000\n", baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if strings.Count(partial, "model_context_window") != 1 || !strings.Contains(partial, "model_auto_compact_token_limit = 900000") {
		t.Fatalf("a missing compact limit was not filled in once:\n%s", partial)
	}
	commented, err := codingclients.MergeCodexConfig("# model_context_window = 128000\n", baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if !strings.Contains(commented, "\nmodel_context_window = 1000000\n") {
		t.Fatalf("a comment was treated as a set window:\n%s", commented)
	}

	narrow, err := codingclients.MergeCodexConfig(kept, baseURL, "", helperFile, false)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if strings.Contains(narrow, "model_context_window = 1000000") || strings.Contains(narrow, "model_auto_compact_token_limit = 900000") {
		t.Fatalf("a disabled window still wrote the defaults:\n%s", narrow)
	}
	if !strings.Contains(narrow, "model_context_window = 128000") || !strings.Contains(narrow, "model_auto_compact_token_limit = 64000") {
		t.Fatalf("disabling the window removed an operator value:\n%s", narrow)
	}
	againNarrow, err := codingclients.MergeCodexConfig(narrow, baseURL, "", helperFile, false)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if againNarrow != narrow {
		t.Fatalf("a second merge with the window off changed the file:\n%s", againNarrow)
	}
}

// TestStripCodexConfigDropsWindowDefaultsReloWrote removes the defaults a
// merge inserted, and keeps a different value plus the same number when it
// belongs to a table rather than the root.
func TestStripCodexConfigDropsWindowDefaultsReloWrote(t *testing.T) {
	merged, err := codingclients.MergeCodexConfig("approval_policy = \"never\"\n", baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	stripped := codingclients.StripCodexConfig(merged)
	if strings.Contains(stripped, "model_context_window") || strings.Contains(stripped, "model_auto_compact_token_limit") {
		t.Fatalf("stripped file still carries Relo's window defaults:\n%s", stripped)
	}
	if !strings.Contains(stripped, "approval_policy = \"never\"") {
		t.Fatalf("stripped file lost the operator's own row:\n%s", stripped)
	}

	custom := strings.Join([]string{
		"model_context_window = 128000",
		"model_auto_compact_token_limit = 64000",
		"",
		"[profiles.dev]",
		"model_context_window = 1000000",
		"",
	}, "\n")
	stripped = codingclients.StripCodexConfig(custom)
	if !strings.Contains(stripped, "model_context_window = 128000") ||
		!strings.Contains(stripped, "model_auto_compact_token_limit = 64000") ||
		!strings.Contains(stripped, "model_context_window = 1000000") {
		t.Fatalf("stripped file dropped a window Relo did not write:\n%s", stripped)
	}
}

// TestStripCodexConfigTakesReloOut keeps the operator's own rows while the
// block and the assignment Relo wrote go.
func TestStripCodexConfigTakesReloOut(t *testing.T) {
	existing := strings.Join([]string{
		"approval_policy = \"never\"",
		"",
		"[model_providers.azure]",
		"name = \"Azure\"",
		"",
	}, "\n")
	merged, err := codingclients.MergeCodexConfig(existing, baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	if _, found := codingclients.CodexBlockOf(merged); !found {
		t.Fatal("CodexBlockOf() found no block in a merged file")
	}
	stripped := codingclients.StripCodexConfig(merged)
	if strings.Contains(stripped, "relo") {
		t.Fatalf("stripped file still names Relo:\n%s", stripped)
	}
	if !strings.Contains(stripped, "model_providers.azure") {
		t.Fatalf("stripped file lost the operator's provider:\n%s", stripped)
	}
}

// TestCodexConfigPointsAtIgnoresTheRestOfTheFile is why an operator's own edit
// is not drift: the managed block and model_provider are Relo's, and a row
// beside them is not.
func TestCodexConfigPointsAtIgnoresTheRestOfTheFile(t *testing.T) {
	merged, err := codingclients.MergeCodexConfig("approval_policy = \"never\"\n", baseURL, "", helperFile, true)
	if err != nil {
		t.Fatalf("MergeCodexConfig() error = %v", err)
	}
	edited := strings.Replace(merged, "approval_policy = \"never\"", "approval_policy = \"on-request\"", 1)
	if !codingclients.CodexConfigPointsAt(edited, baseURL, helperFile) {
		t.Fatalf("CodexConfigPointsAt() = false after an edit outside the block:\n%s", edited)
	}
	if codingclients.CodexConfigPointsAt(codingclients.StripCodexConfig(merged), baseURL, helperFile) {
		t.Fatal("CodexConfigPointsAt() = true after the block was removed")
	}
	rewritten := strings.Replace(merged, "model_provider = \"relo\"", "model_provider = \"openai\"", 1)
	if codingclients.CodexConfigPointsAt(rewritten, baseURL, helperFile) {
		t.Fatal("CodexConfigPointsAt() = true after model_provider left Relo")
	}
}

// TestMergeClaudeSettingsKeepsOtherKeys is the same promise for the Claude Code
// settings file: two keys are Relo's, everything else is untouched.
func TestMergeClaudeSettingsKeepsOtherKeys(t *testing.T) {
	existing := "{\"model\":\"opus\",\"env\":{\"FOO\":\"bar\"},\"permissions\":{\"allow\":[\"Bash\"]}}"
	helper := "/tmp/relo/key-helper"
	merged, err := codingclients.MergeClaudeSettings(existing, "http://127.0.0.1:10202", helper, true)
	if err != nil {
		t.Fatalf("MergeClaudeSettings() error = %v", err)
	}
	for _, want := range []string{"\"model\": \"opus\"", "\"FOO\": \"bar\"", "\"permissions\"",
		"\"ANTHROPIC_BASE_URL\": \"http://127.0.0.1:10202\"", "\"apiKeyHelper\": \"/tmp/relo/key-helper\"",
		"\"" + codingclients.ClaudeGatewayDiscoveryEnv + "\": \"1\""} {
		if !strings.Contains(merged, want) {
			t.Fatalf("merged settings are missing %s:\n%s", want, merged)
		}
	}
	if !codingclients.ClaudeSettingsPointAt(merged, "http://127.0.0.1:10202", helper, true) {
		t.Fatal("ClaudeSettingsPointAt() = false for a merged file")
	}
	stripped, err := codingclients.StripClaudeSettings(merged, "http://127.0.0.1:10202", helper)
	if err != nil {
		t.Fatalf("StripClaudeSettings() error = %v", err)
	}
	if strings.Contains(stripped, "ANTHROPIC_BASE_URL") || strings.Contains(stripped, "apiKeyHelper") ||
		strings.Contains(stripped, codingclients.ClaudeGatewayDiscoveryEnv) {
		t.Fatalf("stripped settings still carry Relo's keys:\n%s", stripped)
	}
	if !strings.Contains(stripped, "\"FOO\": \"bar\"") || !strings.Contains(stripped, "\"model\": \"opus\"") {
		t.Fatalf("stripped settings lost the operator's own keys:\n%s", stripped)
	}
}

// TestMergeClaudeSettingsRefusesAForeignEnv reports the refusal a file Relo
// cannot merge safely: an env key that is not an object.
func TestMergeClaudeSettingsRefusesAForeignEnv(t *testing.T) {
	if _, err := codingclients.MergeClaudeSettings("{\"env\":\"nope\"}", "http://127.0.0.1:10202", "/tmp/helper", true); err == nil {
		t.Fatal("MergeClaudeSettings() error = nil, want a refusal")
	}
	if _, err := codingclients.MergeClaudeSettings("{not json", "http://127.0.0.1:10202", "/tmp/helper", false); err == nil {
		t.Fatal("MergeClaudeSettings() error = nil for a broken file, want a refusal")
	}
}

// TestMergeClaudeSettingsLeavesALoginAlone is the subscription path: the
// surface and the gateway switch are written, and the key helper is not,
// because that helper is the auth source Claude Code prefers over a login.
func TestMergeClaudeSettingsLeavesALoginAlone(t *testing.T) {
	helper := "/tmp/relo/key-helper"
	existing := "{\"apiKeyHelper\":\"" + helper + "\",\"env\":{\"ANTHROPIC_API_KEY\":\"user\"}}"
	merged, err := codingclients.MergeClaudeSettings(existing, "http://127.0.0.1:10202/", helper, false)
	if err != nil {
		t.Fatalf("MergeClaudeSettings() error = %v", err)
	}
	if strings.Contains(merged, "apiKeyHelper") {
		t.Fatalf("merged settings still install the helper:\n%s", merged)
	}
	if !strings.Contains(merged, "\"ANTHROPIC_API_KEY\": \"user\"") {
		t.Fatalf("merged settings dropped the operator's own key:\n%s", merged)
	}
	if !codingclients.ClaudeSettingsPointAt(merged, "http://127.0.0.1:10202", helper, false) {
		t.Fatal("ClaudeSettingsPointAt() = false for a login merge")
	}
	if codingclients.ClaudeSettingsPointAt(merged, "http://127.0.0.1:10202", helper, true) {
		t.Fatal("ClaudeSettingsPointAt() = true for a helper the login merge removed")
	}
}

// TestWriteFileAtomicOverwritesWhenASiblingCannotBeCreated is the ~/.codex
// case: the directory forbids a new name, and Relo still has to replace the
// catalog file it already wrote.
func TestWriteFileAtomicOverwritesWhenASiblingCannotBeCreated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "relo-model-catalog.json")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("seed the file: %v", err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("lock the directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := codingclients.WriteFileAtomic(path, []byte("new\n"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v, want an in-place write", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the file: %v", err)
	}
	if string(content) != "new\n" {
		t.Fatalf("content = %q, want the replacement", content)
	}
}
