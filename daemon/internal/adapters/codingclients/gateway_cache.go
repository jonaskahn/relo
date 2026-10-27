// Gateway model cache: picker identifiers per model.
package codingclients

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/catalog"
)

// ClaudeGatewayDiscoveryEnv is the switch Claude Code needs before it will
// list models from a gateway. Subscription launches carry no token, so the
// client will not refresh the cache itself and reads the file Relo writes.
const ClaudeGatewayDiscoveryEnv = "CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY"

type gatewayCache struct {
	BaseURL   string         `json:"baseUrl"`
	FetchedAt int64          `json:"fetchedAt"`
	Models    []gatewayModel `json:"models"`
}

type gatewayModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

// GatewayCachePath returns the model cache Claude Code reads for one config
// directory.
func GatewayCachePath(configDir string) string {
	return filepath.Join(configDir, "cache", "gateway-models.json")
}

// GatewayCacheLists reports whether the picker cache carries exactly the models
// Relo would publish, with the same base URL and picker names it writes.
func GatewayCacheLists(configDir, baseURL string, models []ModelRef) bool {
	content, err := os.ReadFile(GatewayCachePath(configDir))
	if err != nil {
		return false
	}
	var cached gatewayCache
	if err := json.Unmarshal(content, &cached); err != nil {
		return false
	}
	if strings.TrimSuffix(strings.TrimSpace(cached.BaseURL), "/") != strings.TrimSuffix(strings.TrimSpace(baseURL), "/") {
		return false
	}
	return samePickerNames(cached.Models, models)
}

// GatewayCacheDiff counts how the picker cache differs from the models Relo
// would publish. A missing cache, a cache for another gateway, or one that
// does not parse counts every published model as added.
func GatewayCacheDiff(configDir, baseURL string, models []ModelRef) ModelsDiff {
	content, err := os.ReadFile(GatewayCachePath(configDir))
	if err != nil {
		return missingDiff(models)
	}
	var cached gatewayCache
	if err := json.Unmarshal(content, &cached); err != nil {
		return missingDiff(models)
	}
	if strings.TrimSuffix(strings.TrimSpace(cached.BaseURL), "/") != strings.TrimSuffix(strings.TrimSpace(baseURL), "/") {
		return missingDiff(models)
	}
	return pickerNamesDiff(cached.Models, models)
}

// missingDiff counts every published model as added, for a cache that cannot
// be read or does not belong to this gateway.
func missingDiff(models []ModelRef) ModelsDiff {
	return ModelsDiff{Added: len(wantedPickerNames(models))}
}

func samePickerNames(listed []gatewayModel, models []ModelRef) bool {
	return pickerNamesDiff(listed, models).Empty()
}

func pickerNamesDiff(listed []gatewayModel, models []ModelRef) ModelsDiff {
	got := make(map[string]string, len(listed))
	for _, model := range listed {
		got[model.ID] = model.DisplayName
	}
	return diffNames(got, wantedPickerNames(models))
}

func gatewayPickerID(model ModelRef) string {
	id := strings.TrimSpace(model.ID)
	if strings.HasPrefix(id, catalog.ClaudePrefix) {
		return id
	}
	return catalog.AnthropicClientID(id, model.ContextWindow)
}

// WriteGatewayCache writes the picker cache Claude Code reads when it has no
// token of its own. baseURL has to equal ANTHROPIC_BASE_URL or the client
// ignores the file. The ids are the Anthropic spellings, which are the only
// ones the picker keeps.
func WriteGatewayCache(configDir, baseURL string, models []ModelRef) error {
	if GatewayCacheLists(configDir, baseURL, models) {
		return nil
	}
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	rows := make([]gatewayModel, 0, len(models))
	for _, model := range models {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		row := gatewayModel{ID: gatewayPickerID(model), DisplayName: ConfigDisplayName(model)}
		rows = append(rows, row)
	}
	payload := gatewayCache{BaseURL: baseURL, FetchedAt: time.Now().UnixMilli(), Models: rows}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return WriteFileAtomic(GatewayCachePath(configDir), append(encoded, '\n'), 0o600)
}

// RemoveGatewayCache deletes the picker cache when it points at Relo, and
// leaves a cache some other gateway wrote alone.
func RemoveGatewayCache(configDir, baseURL string) error {
	path := GatewayCachePath(configDir)
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var cached gatewayCache
	if err := json.Unmarshal(content, &cached); err != nil {
		return nil
	}
	if strings.TrimSuffix(cached.BaseURL, "/") != strings.TrimSuffix(strings.TrimSpace(baseURL), "/") {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
