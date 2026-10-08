// Claude Desktop paths: install detection and config library.
package codingclients

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	// desktopProfileID is a UUID because Claude Desktop only reads an applied
	// profile whose id looks like one.
	desktopProfileID   = "5e1a0c3b-7d24-4f6a-9b8e-2c4d6f8a1b30"
	desktopProfileName = "relo"
	// legacyDesktopProfileID is the id earlier builds wrote. Desktop ignores
	// it, so Relo replaces that profile with one under desktopProfileID.
	legacyDesktopProfileID = "relo"
	desktopSupports1M      = int64(1_000_000)
)

// DesktopPaths is the machine a Claude Desktop config library is resolved on.
type DesktopPaths struct {
	Home     string
	Platform string
	Getenv   func(string) string
}

// DesktopConfigLibrary is the directory Claude Desktop reads gateway profiles
// from. CLAUDE_USER_DATA_DIR is used as given. Every other root is Desktop's
// Claude-3p user-data directory.
func DesktopConfigLibrary(paths DesktopPaths) string {
	return desktopJoin(paths.Platform, desktopUserData(paths), "configLibrary")
}

// DesktopLibrary is the config library on this machine.
func (p Paths) DesktopLibrary() string {
	return DesktopConfigLibrary(DesktopPaths{Home: p.Home, Platform: runtime.GOOS, Getenv: os.Getenv})
}

// DesktopInstalled is DesktopInstalledAt on this machine.
func (p Paths) DesktopInstalled() error {
	return DesktopInstalledAt(DesktopPaths{Home: p.Home, Platform: runtime.GOOS, Getenv: os.Getenv})
}

// DesktopInstalledAt reports whether Claude Desktop has run on the machine.
// The Claude-3p directory only appears once Desktop has used third-party
// mode, so Desktop's own user-data directory counts as well. Relo creates the
// config library itself when it writes the profile.
func DesktopInstalledAt(paths DesktopPaths) error {
	err := Installed(desktopUserData(paths))
	if err == nil || !errors.Is(err, ErrNotInstalled) {
		return err
	}
	return Installed(desktopElectronUserData(paths))
}

// DesktopProfileContent is the gateway profile Claude Desktop reads. The key
// stays in that file, which is the credential Desktop sends to Relo.
func DesktopProfileContent(baseURL, apiKey string, models []ModelRef) ([]byte, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("claude desktop: %w (no address or key)", ErrUnrecognisedFile)
	}
	return desktopProfile(baseURL, apiKey, models)
}

// SelectDesktopProfile makes Relo's profile the one Desktop uses, and remembers
// the profile that was selected so it can be restored.
func SelectDesktopProfile(library string) error {
	document, err := loadDesktopMeta(library)
	if err != nil {
		return err
	}
	entries := desktopEntries(document)
	previous := desktopPrevious(entries, desktopString(document["appliedId"]))
	document["entries"] = upsertDesktopEntry(entries, previous)
	document["appliedId"] = desktopProfileID
	if err := writeDesktopMeta(library, document); err != nil {
		return err
	}
	return removeLegacyDesktopProfile(library)
}

// DesktopProfilePath is the file Relo's gateway profile lives in.
func DesktopProfilePath(library string) string {
	return desktopJoin(desktopPlatform(library), library, desktopProfileID+".json")
}

func removeLegacyDesktopProfile(library string) error {
	path := desktopJoin(desktopPlatform(library), library, legacyDesktopProfileID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

// WriteDesktopProfile writes the profile and selects it. A caller that
// snapshots the profile file writes the bytes itself and selects afterwards.
func WriteDesktopProfile(library, baseURL, apiKey string, models []ModelRef) (string, []byte, error) {
	content, err := DesktopProfileContent(baseURL, apiKey, models)
	if err != nil {
		return "", nil, err
	}
	profilePath := DesktopProfilePath(library)
	if err := WriteFileAtomic(profilePath, content, 0o600); err != nil {
		return "", nil, err
	}
	if err := SelectDesktopProfile(library); err != nil {
		return "", nil, err
	}
	return profilePath, content, nil
}

// UpdateDesktopModels rewrites the model list of a profile Relo already
// wrote. A library with no Relo profile is left alone.
func UpdateDesktopModels(library, baseURL string, models []ModelRef) ([]byte, error) {
	profilePath := DesktopProfilePath(library)
	existing, err := os.ReadFile(profilePath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", profilePath, err)
	}
	profile, err := decodeDesktopProfile(profilePath, existing)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseURL) != "" {
		profile["inferenceGatewayBaseUrl"] = strings.TrimSuffix(baseURL, "/")
	}
	encoded, err := json.Marshal(desktopModels(models))
	if err != nil {
		return nil, err
	}
	var modelsValue any
	if err := json.Unmarshal(encoded, &modelsValue); err != nil {
		return nil, err
	}
	profile["inferenceModels"] = modelsValue
	profile["modelDiscoveryEnabled"] = false
	return writeDesktopProfile(profilePath, profile)
}

func decodeDesktopProfile(profilePath string, existing []byte) (map[string]any, error) {
	var profile map[string]any
	if err := json.Unmarshal(existing, &profile); err != nil {
		return nil, fmt.Errorf("claude desktop: %w (%v)", ErrUnrecognisedFile, err)
	}
	return profile, nil
}

func writeDesktopProfile(profilePath string, profile map[string]any) ([]byte, error) {
	content, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}
	content = append(content, '\n')
	if err := WriteFileAtomic(profilePath, content, 0o600); err != nil {
		return nil, err
	}
	return content, nil
}

// RemoveDesktopProfile deletes Relo's profile and selects the profile that
// was active before it. Another profile in the library is left in place.
func RemoveDesktopProfile(library string) error {
	document, err := loadDesktopMeta(library)
	if err != nil {
		return err
	}
	entries := desktopEntries(document)
	kept, previous, ownedIDs := splitDesktopEntries(entries)
	if ownedIDs[desktopString(document["appliedId"])] {
		if next := desktopRestoredID(kept, previous); next != "" {
			document["appliedId"] = next
		} else {
			delete(document, "appliedId")
		}
	}
	document["entries"] = kept
	profilePath := DesktopProfilePath(library)
	if err := os.Remove(profilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", profilePath, err)
	}
	if err := removeLegacyDesktopProfile(library); err != nil {
		return err
	}
	return writeDesktopMetaAfterRemove(library, document, kept)
}

func writeDesktopMetaAfterRemove(library string, document map[string]any, kept []map[string]any) error {
	if len(kept) == 0 {
		delete(document, "entries")
		delete(document, "appliedId")
		if len(document) == 0 {
			if err := os.Remove(desktopMetaPath(library)); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", desktopMetaPath(library), err)
			}
			return nil
		}
	}
	return writeDesktopMeta(library, document)
}

func splitDesktopEntries(entries []map[string]any) ([]map[string]any, string, map[string]bool) {
	previous := ""
	ownedIDs := map[string]bool{desktopProfileID: true}
	kept := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if desktopOwned(entry) {
			previous = desktopString(entry["previousAppliedId"])
			ownedIDs[desktopString(entry["id"])] = true
			continue
		}
		kept = append(kept, entry)
	}
	return kept, previous, ownedIDs
}

func desktopProfile(baseURL, apiKey string, models []ModelRef) ([]byte, error) {
	profile := map[string]any{
		"inferenceProvider":       "gateway",
		"inferenceCredentialKind": "static",
		"inferenceGatewayBaseUrl": strings.TrimSuffix(baseURL, "/"),
		"inferenceGatewayApiKey":  apiKey,
		"modelDiscoveryEnabled":   false,
		"inferenceModels":         desktopModels(models),
	}
	encoded, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func desktopModels(models []ModelRef) []desktopModel {
	rows := make([]desktopModel, 0, len(models))
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		// Desktop drops a name that carries another vendor's word, so it is
		// given an opaque id the snapshot resolves back to the model. The
		// million-token window is the supports1m flag, not part of the name.
		name := catalog.DesktopModelAlias(gatewayPickerID(model))
		if seen[name] {
			continue
		}
		seen[name] = true
		row := desktopModel{
			Name: name, LabelOverride: codexDisplayName(model),
			AnthropicFamilyTier: "opus",
		}
		if model.ContextWindow != nil && *model.ContextWindow >= desktopSupports1M {
			row.Supports1m = true
			row.Prefer1m = true
		}
		rows = append(rows, row)
	}
	if len(rows) > 0 {
		rows[0].IsFamilyDefault = true
	}
	return rows
}

type desktopModel struct {
	Name                string `json:"name"`
	LabelOverride       string `json:"labelOverride"`
	AnthropicFamilyTier string `json:"anthropicFamilyTier"`
	IsFamilyDefault     bool   `json:"isFamilyDefault,omitempty"`
	Supports1m          bool   `json:"supports1m,omitempty"`
	Prefer1m            bool   `json:"prefer1m,omitempty"`
}

func desktopUserData(paths DesktopPaths) string {
	if explicit := strings.TrimSpace(paths.env("CLAUDE_USER_DATA_DIR")); explicit != "" {
		return strings.TrimRight(explicit, `/\`)
	}
	if paths.Platform == "windows" {
		if local := strings.TrimSpace(paths.env("LOCALAPPDATA")); local != "" {
			return desktopJoin(paths.Platform, strings.TrimRight(local, `/\`), "Claude-3p")
		}
	}
	root := desktopElectronUserData(paths)
	if strings.HasSuffix(root, "-3p") {
		return root
	}
	return root + "-3p"
}

func desktopElectronUserData(paths DesktopPaths) string {
	home := strings.TrimRight(paths.Home, `/\`)
	switch paths.Platform {
	case "windows":
		if appData := strings.TrimSpace(paths.env("APPDATA")); appData != "" {
			return desktopJoin(paths.Platform, strings.TrimRight(appData, `/\`), "Claude")
		}
		return desktopJoin(paths.Platform, home, "AppData", "Roaming", "Claude")
	case "darwin":
		return desktopJoin(paths.Platform, home, "Library", "Application Support", "Claude")
	default:
		if xdg := strings.TrimSpace(paths.env("XDG_CONFIG_HOME")); xdg != "" {
			return desktopJoin(paths.Platform, strings.TrimRight(xdg, `/\`), "Claude")
		}
		return desktopJoin(paths.Platform, home, ".config", "Claude")
	}
}

func (paths DesktopPaths) env(key string) string {
	if paths.Getenv == nil {
		return ""
	}
	return paths.Getenv(key)
}

func desktopJoin(platform string, parts ...string) string {
	sep := "/"
	if platform == "windows" {
		sep = `\`
	}
	return strings.Join(parts, sep)
}

func desktopPlatform(path string) string {
	if strings.Contains(path, `\`) && !strings.Contains(path, "/") {
		return "windows"
	}
	return "darwin"
}

func desktopMetaPath(library string) string {
	return desktopJoin(desktopPlatform(library), library, "_meta.json")
}

func loadDesktopMeta(library string) (map[string]any, error) {
	content, err := os.ReadFile(desktopMetaPath(library))
	if os.IsNotExist(err) {
		return map[string]any{"entries": []any{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", desktopMetaPath(library), err)
	}
	var document map[string]any
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, fmt.Errorf("claude desktop: %w (%v)", ErrUnrecognisedFile, err)
	}
	if document == nil {
		document = map[string]any{}
	}
	return document, nil
}

func writeDesktopMeta(library string, document map[string]any) error {
	encoded, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(desktopMetaPath(library), append(encoded, '\n'), 0o600)
}

func desktopEntries(document map[string]any) []map[string]any {
	raw, _ := document["entries"].([]any)
	entries := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if ok {
			entries = append(entries, entry)
		}
	}
	return entries
}

func desktopPrevious(entries []map[string]any, applied string) string {
	appliedOurs := applied == desktopProfileID
	previous := ""
	for _, entry := range entries {
		if desktopOwned(entry) {
			if desktopString(entry["id"]) == applied {
				appliedOurs = true
			}
			if previous == "" {
				previous = desktopString(entry["previousAppliedId"])
			}
		}
	}
	if applied != "" && !appliedOurs {
		return applied
	}
	return previous
}

func upsertDesktopEntry(entries []map[string]any, previous string) []map[string]any {
	owned := map[string]any{"id": desktopProfileID, "name": desktopProfileName}
	if previous != "" {
		owned["previousAppliedId"] = previous
	}
	for index, entry := range entries {
		if desktopOwned(entry) {
			entries[index] = owned
			return entries
		}
	}
	return append(entries, owned)
}

func desktopOwned(entry map[string]any) bool {
	id := desktopString(entry["id"])
	return id == desktopProfileID || id == legacyDesktopProfileID || desktopString(entry["name"]) == desktopProfileName
}

func desktopRestoredID(entries []map[string]any, previous string) string {
	if desktopKnownID(entries, previous) {
		return previous
	}
	for _, entry := range entries {
		if id := desktopString(entry["id"]); safeDesktopID(id) {
			return id
		}
	}
	return ""
}

func desktopKnownID(entries []map[string]any, id string) bool {
	if !safeDesktopID(id) {
		return false
	}
	for _, entry := range entries {
		if desktopString(entry["id"]) == id {
			return true
		}
	}
	return false
}

func safeDesktopID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for index, char := range id {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
		case index > 0 && (char == '_' || char == '-'):
		default:
			return false
		}
	}
	return true
}

func desktopString(value any) string {
	text, _ := value.(string)
	return text
}
