// Codex config merge: the managed block Relo owns.
package codingclients

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/BurntSushi/toml"
)

const (
	codexBlockStart = "# >>> relo (managed) >>>"
	codexBlockEnd   = "# <<< relo (managed) <<<"
	reloProviderID  = "relo"
	// Codex treats a missing window as a small default and compacts early.
	// These are the values Relo writes when the operator has not chosen any.
	codexContextWindowKey = "model_context_window"
	codexContextWindow    = "1000000"
	codexCompactLimitKey  = "model_auto_compact_token_limit"
	codexCompactLimit     = "900000"
)

// ErrUnrecognisedFile reports a client file Relo will not rewrite, which is
// always better than a merge that drops something the operator wrote.
var ErrUnrecognisedFile = errors.New("the file is not in a shape Relo can edit")

const codexAuthTimeoutMs = 35000

// CodexBlock renders the block Relo owns in a Codex configuration file.
// The key is read by a helper Codex itself runs, which also starts the daemon.
// A shell variable never reaches the desktop app or the IDE, which is where
// env_key fails.
func CodexBlock(baseURL, helperFile string) string {
	command, args := codexAuthCommand(helperFile)
	return strings.Join([]string{
		codexBlockStart,
		"[model_providers." + reloProviderID + "]",
		"name = \"Relo\"",
		"base_url = \"" + strings.TrimSuffix(baseURL, "/") + "/v1\"",
		"requires_openai_auth = false",
		"",
		"[model_providers." + reloProviderID + ".auth]",
		"command = " + tomlString(command),
		"args = " + tomlArray(args),
		"timeout_ms = " + strconv.Itoa(codexAuthTimeoutMs),
		"refresh_interval_ms = 300000",
		codexBlockEnd,
	}, "\n") + "\n"
}

func codexAuthCommand(helperFile string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/d", "/c", windowsCmdQuoted(helperFile)}
	}
	return helperFile, []string{}
}

func windowsCmdQuoted(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func tomlArray(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = tomlString(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// MergeCodexConfig returns a Codex configuration with Relo's provider in it
// and the top-level model_provider pointing at that provider. A model_provider
// the operator set is rewritten rather than duplicated, because two top-level
// assignments would make the whole file unreadable. wideContext writes the
// 1M window when those root keys are absent, and takes Relo's defaults back
// out when it is off. A value the operator chose is left either way.
func MergeCodexConfig(existing, baseURL, catalogPath, helperFile string, wideContext bool) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("codex: %w (no address is served for the OpenAI surface)", ErrUnrecognisedFile)
	}
	if strings.TrimSpace(helperFile) == "" {
		return "", fmt.Errorf("codex: %w (no key helper)", ErrUnrecognisedFile)
	}
	eol := "\n"
	if strings.Contains(existing, "\r\n") {
		eol = "\r\n"
	}
	lines := splitLines(existing)
	lines = removeBlock(lines)
	lines = setModelProvider(lines)
	lines = setModelCatalog(lines, catalogPath)
	lines = dropStaleModel(lines, catalogPath)
	if wideContext {
		lines = ensureCodexWindowDefaults(lines)
	} else {
		lines = dropCodexWindowDefaults(lines)
	}
	lines = appendBlock(lines, CodexBlock(baseURL, helperFile))
	return finalizeCodexConfig(lines, eol)
}

func finalizeCodexConfig(lines []string, eol string) (string, error) {
	merged := strings.Join(lines, eol)
	if !strings.HasSuffix(merged, eol) {
		merged += eol
	}
	if _, err := decodeTOML(merged); err != nil {
		return "", fmt.Errorf("codex: %w (%v)", ErrUnrecognisedFile, err)
	}
	return merged, nil
}

// StripCodexConfig removes Relo's block, the model_provider assignment Relo
// wrote, and a context-window default Relo added. A window the operator set
// to some other value stays, so removing the integration gives their file back.
func StripCodexConfig(existing string) string {
	eol := "\n"
	if strings.Contains(existing, "\r\n") {
		eol = "\r\n"
	}
	lines := dropCodexWindowDefaults(removeBlock(splitLines(existing)))
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if isReloModelProvider(line) || isReloModelCatalog(line) {
			continue
		}
		kept = append(kept, line)
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, eol) + eol
}

// CodexConfigPointsAt reports whether a Codex file still carries the provider
// Relo wrote. The managed block and model_provider are Relo's; the rest of
// the file is the operator's and does not count.
func CodexConfigPointsAt(content, baseURL, helperFile string) bool {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(helperFile) == "" {
		return false
	}
	block, found := CodexBlockOf(content)
	if !found || block != CodexBlock(baseURL, helperFile) {
		return false
	}
	document, err := decodeTOML(content)
	if err != nil {
		return false
	}
	provider, _ := document["model_provider"].(string)
	return provider == reloProviderID
}

// CodexBlockOf returns the block a Codex configuration carries, and whether it
// has one at all.
func CodexBlockOf(content string) (string, bool) {
	lines := splitLines(content)
	start, end := -1, -1
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == codexBlockStart && start < 0 {
			start = index
			continue
		}
		if trimmed == codexBlockEnd && start >= 0 {
			end = index
			break
		}
	}
	if start < 0 || end < start {
		return "", false
	}
	return strings.Join(lines[start:end+1], "\n") + "\n", true
}

func splitLines(content string) []string {
	normalised := strings.ReplaceAll(content, "\r\n", "\n")
	if normalised == "" {
		return []string{}
	}
	return strings.Split(strings.TrimSuffix(normalised, "\n"), "\n")
}

func removeBlock(lines []string) []string {
	kept := make([]string, 0, len(lines))
	inside := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == codexBlockStart:
			inside = true
		case trimmed == codexBlockEnd:
			inside = false
		case !inside:
			kept = append(kept, line)
		}
	}
	return kept
}

func setModelProvider(lines []string) []string {
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		if isModelProvider(trimmed) {
			lines[index] = "model_provider = \"" + reloProviderID + "\""
			return lines
		}
	}
	insert := leadingCommentEnd(lines)
	assignment := "model_provider = \"" + reloProviderID + "\""
	updated := make([]string, 0, len(lines)+2)
	updated = append(updated, lines[:insert]...)
	updated = append(updated, assignment)
	updated = append(updated, lines[insert:]...)
	return updated
}

func leadingCommentEnd(lines []string) int {
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return index
	}
	return len(lines)
}

func appendBlock(lines []string, block string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, strings.Split(strings.TrimSuffix(block, "\n"), "\n")...)
}

func isModelProvider(line string) bool {
	return strings.HasPrefix(line, "model_provider") && assignmentValue(line) != ""
}

func isReloModelProvider(line string) bool {
	return isModelProvider(strings.TrimSpace(line)) && strings.Trim(assignmentValue(line), "\"") == reloProviderID
}

func setModelCatalog(lines []string, catalogPath string) []string {
	if strings.TrimSpace(catalogPath) == "" {
		return lines
	}
	assignment := "model_catalog_json = " + tomlString(catalogPath)
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		if isModelCatalog(trimmed) {
			lines[index] = assignment
			return lines
		}
	}
	insert := leadingCommentEnd(lines)
	updated := make([]string, 0, len(lines)+1)
	updated = append(updated, lines[:insert]...)
	updated = append(updated, assignment)
	return append(updated, lines[insert:]...)
}

func dropCodexWindowDefaults(lines []string) []string {
	kept := make([]string, 0, len(lines))
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			inTable = true
		}
		if !inTable && isCodexWindowDefault(line) {
			continue
		}
		kept = append(kept, line)
	}
	return kept
}

func ensureCodexWindowDefaults(lines []string) []string {
	missing := make([]string, 0, 2)
	if !rootHasAssignment(lines, codexContextWindowKey) {
		missing = append(missing, codexContextWindowKey+" = "+codexContextWindow)
	}
	if !rootHasAssignment(lines, codexCompactLimitKey) {
		missing = append(missing, codexCompactLimitKey+" = "+codexCompactLimit)
	}
	if len(missing) == 0 {
		return lines
	}
	insert := leadingCommentEnd(lines)
	updated := make([]string, 0, len(lines)+len(missing))
	updated = append(updated, lines[:insert]...)
	updated = append(updated, missing...)
	return append(updated, lines[insert:]...)
}

func rootHasAssignment(lines []string, key string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			return false
		}
		if rootAssignmentKey(trimmed) == key {
			return true
		}
	}
	return false
}

func rootAssignmentKey(line string) string {
	key, value, found := strings.Cut(line, "=")
	if !found || strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.TrimSpace(key)
}

func isCodexWindowDefault(line string) bool {
	key := rootAssignmentKey(strings.TrimSpace(line))
	value, _, _ := strings.Cut(assignmentValue(line), "#")
	value = strings.TrimSpace(value)
	switch key {
	case codexContextWindowKey:
		return value == codexContextWindow
	case codexCompactLimitKey:
		return value == codexCompactLimit
	default:
		return false
	}
}

func dropStaleModel(lines []string, catalogPath string) []string {
	published := PublishedCodexSlugs(catalogPath)
	if len(published) == 0 {
		return lines
	}
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			break
		}
		key, value, found := strings.Cut(trimmed, "=")
		if !found || strings.TrimSpace(key) != "model" {
			continue
		}
		value, _, _ = strings.Cut(value, "#")
		if !published[strings.Trim(strings.TrimSpace(value), "\"'")] {
			return append(lines[:index:index], lines[index+1:]...)
		}
		return lines
	}
	return lines
}

func isModelCatalog(line string) bool {
	return strings.HasPrefix(line, "model_catalog_json") && assignmentValue(line) != ""
}

func isReloModelCatalog(line string) bool {
	trimmed := strings.TrimSpace(line)
	return isModelCatalog(trimmed) && strings.Contains(assignmentValue(trimmed), codexCatalogFile)
}

func tomlString(value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)
	return `"` + escaped + `"`
}

func assignmentValue(line string) string {
	_, value, found := strings.Cut(line, "=")
	if !found {
		return ""
	}
	return strings.TrimSpace(value)
}

func decodeTOML(content string) (map[string]any, error) {
	document := map[string]any{}
	if err := toml.Unmarshal([]byte(content), &document); err != nil {
		return nil, err
	}
	return document, nil
}

// MergeClaudeSettings returns Claude Code's settings with Relo's surface in
// place, leaving every other key exactly as it was. installHelper is the
// proxy mode: the key helper is an auth source, and a claude.ai login must
// not receive one. A helper Relo previously installed is taken back out when
// this merge is for a login.
func MergeClaudeSettings(existing, baseURL, helperPath string, installHelper bool) (string, error) {
	if strings.TrimSpace(baseURL) == "" {
		return "", fmt.Errorf("claude-code: %w (no address is served for the Anthropic surface)", ErrUnrecognisedFile)
	}
	document, err := decodeSettings(existing)
	if err != nil {
		return "", err
	}
	env, err := settingsEnv(document)
	if err != nil {
		return "", err
	}
	env["ANTHROPIC_BASE_URL"] = strings.TrimSuffix(baseURL, "/")
	env[ClaudeGatewayDiscoveryEnv] = "1"
	document["env"] = env
	if installHelper {
		document["apiKeyHelper"] = helperPath
	} else if value, ok := document["apiKeyHelper"].(string); ok && value == helperPath {
		delete(document, "apiKeyHelper")
	}
	return encodeSettings(document)
}

// StripClaudeSettings removes the settings Relo set, and leaves a key the
// operator set themselves alone.
func StripClaudeSettings(existing, baseURL, helperPath string) (string, error) {
	document, err := decodeSettings(existing)
	if err != nil {
		return "", err
	}
	if env, err := settingsEnv(document); err == nil {
		if value, ok := env["ANTHROPIC_BASE_URL"].(string); ok && value == strings.TrimSuffix(baseURL, "/") {
			delete(env, "ANTHROPIC_BASE_URL")
		}
		if value, ok := env[ClaudeGatewayDiscoveryEnv].(string); ok && value == "1" {
			delete(env, ClaudeGatewayDiscoveryEnv)
		}
		if len(env) == 0 {
			delete(document, "env")
		} else {
			document["env"] = env
		}
	}
	if value, ok := document["apiKeyHelper"].(string); ok && value == helperPath {
		delete(document, "apiKeyHelper")
	}
	return encodeSettings(document)
}

// ClaudeSettingsPointAt reports whether a settings document already points at
// Relo's surface the way this mode writes it, which is what tells an intact
// integration from one an editor or an update undid.
func ClaudeSettingsPointAt(content, baseURL, helperPath string, installHelper bool) bool {
	document, err := decodeSettings(content)
	if err != nil {
		return false
	}
	env, err := settingsEnv(document)
	if err != nil {
		return false
	}
	if value, ok := env["ANTHROPIC_BASE_URL"].(string); !ok || value != strings.TrimSuffix(baseURL, "/") {
		return false
	}
	if value, ok := env[ClaudeGatewayDiscoveryEnv].(string); !ok || value != "1" {
		return false
	}
	helper, _ := document["apiKeyHelper"].(string)
	if installHelper {
		return helper == helperPath
	}
	return helper != helperPath
}

func decodeSettings(content string) (map[string]any, error) {
	document := map[string]any{}
	if strings.TrimSpace(content) == "" {
		return document, nil
	}
	if err := json.Unmarshal([]byte(content), &document); err != nil {
		return nil, fmt.Errorf("claude-code: %w (%v)", ErrUnrecognisedFile, err)
	}
	return document, nil
}

func settingsEnv(document map[string]any) (map[string]any, error) {
	raw, found := document["env"]
	if !found {
		return map[string]any{}, nil
	}
	env, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("claude-code: %w (env is not an object)", ErrUnrecognisedFile)
	}
	return env, nil
}

func encodeSettings(document map[string]any) (string, error) {
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode the Claude Code settings: %w", err)
	}
	return string(encoded) + "\n", nil
}

// ReadFileOrEmpty returns a file's content, and an empty string when it does
// not exist yet.
func ReadFileOrEmpty(path string) (string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(content), nil
}

// WriteFileAtomic writes through a temporary neighbour and a rename, so a
// reader never sees half a configuration file. A directory that forbids
// creating a sibling (macOS privacy on ~/.codex, a read-only folder bit)
// still gets the bytes written in place.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := writeBeside(dir, path, data, mode); err == nil {
		return nil
	} else if !permissionDenied(err) {
		return err
	}
	if err := writeInPlace(path, data, mode); err != nil {
		return err
	}
	return nil
}

func writeBeside(dir, path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(dir, filepath.Base(path)+".relo-*")
	if err != nil {
		return fmt.Errorf("create a temporary file beside %s: %w", path, err)
	}
	name := temporary.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Chmod(name, mode); err != nil {
		return fmt.Errorf("set the mode of %s: %w", path, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func writeInPlace(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("set the mode of %s: %w", path, err)
	}
	return nil
}

func permissionDenied(err error) bool {
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES)
}

// Digest renders the fingerprint Relo stores for what it wrote.
func Digest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Snapshot copies a client file aside once, before Relo's first write to it.
// A snapshot that already exists is kept: it is the state the operator had
// before Relo touched anything.
func Snapshot(source, dir, name string) (string, error) {
	content, err := os.ReadFile(source)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", source, err)
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := WriteFileAtomic(path, content, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
