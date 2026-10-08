// Appearance settings: reading and updating the console theme.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

var appearanceMu sync.Mutex
var uiHeader = regexp.MustCompile(`^\s*\[ui\]\s*(?:#.*)?$`)
var tableHeader = regexp.MustCompile(`^\s*\[.*\]\s*(?:#.*)?$`)
var appearanceKey = regexp.MustCompile(`^(\s*)(language|theme|accent)(\s*=\s*)(?:"[^"]*"|'[^']*'|[^#\r\n]*)(\s*(?:#.*)?)(\r?\n?)$`)

// AppearancePatch changes only the named fields of the shared UI config.
type AppearancePatch struct {
	Language *string `json:"language,omitempty"`
	Theme    *string `json:"theme,omitempty"`
	Accent   *string `json:"accent,omitempty"`
}

// ReadAppearance reads the file on every request so config.toml is the source
// of truth even when another process edited it after the daemon started.
func ReadAppearance(path string) (UIConfig, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig().UI, nil
	}
	if err != nil {
		return UIConfig{}, err
	}
	return decodeAppearance(data)
}

func decodeAppearance(data []byte) (UIConfig, error) {
	ui := DefaultConfig().UI
	var file struct {
		UI UIConfig `toml:"ui"`
	}
	file.UI = ui
	if _, err := toml.Decode(string(data), &file); err != nil {
		return UIConfig{}, err
	}
	if err := ValidateAppearance(file.UI.Theme, file.UI.Accent); err != nil {
		return UIConfig{}, err
	}
	if err := validateLanguage(file.UI.Language); err != nil {
		return UIConfig{}, err
	}
	return file.UI, nil
}

// UpdateAppearance edits the two keys in place, keeping other keys, comments,
// and section order. A temp file and rename avoid exposing a partial config.
func UpdateAppearance(path string, patch AppearancePatch) (UIConfig, error) {
	appearanceMu.Lock()
	defer appearanceMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return UIConfig{}, err
	}
	current, err := decodeAppearance(data)
	if err != nil {
		return UIConfig{}, err
	}
	applyAppearancePatch(&current, patch)
	if err := ValidateAppearance(current.Theme, current.Accent); err != nil {
		return UIConfig{}, err
	}
	updated := replaceAppearance(string(data), current)
	if _, err := decodeAppearance([]byte(updated)); err != nil {
		return UIConfig{}, fmt.Errorf("update appearance: %w", err)
	}
	if err := writeFileAtomically(path, updated); err != nil {
		return UIConfig{}, err
	}
	return current, nil
}

func applyAppearancePatch(current *UIConfig, patch AppearancePatch) {
	if patch.Language != nil {
		current.Language = *patch.Language
	}
	if patch.Theme != nil {
		current.Theme = *patch.Theme
	}
	if patch.Accent != nil {
		current.Accent = *patch.Accent
	}
}

func replaceAppearance(source string, ui UIConfig) string {
	lines := strings.SplitAfter(source, "\n")
	paint := &appearancePaint{lines: lines, ui: ui, seen: map[string]bool{}}
	for i, line := range lines {
		paint.visitLine(i, line)
	}
	return paint.finish(source)
}

type appearancePaint struct {
	lines   []string
	ui      UIConfig
	seen    map[string]bool
	inside  bool
	foundUI bool
}

func (p *appearancePaint) visitLine(i int, line string) {
	trimmed := strings.TrimRight(line, "\r\n")
	if uiHeader.MatchString(trimmed) {
		p.inside, p.foundUI = true, true
		return
	}
	if p.inside && tableHeader.MatchString(trimmed) {
		p.lines[i] = missingAppearance(p.seen, p.ui) + line
		p.inside = false
	}
	if !p.inside {
		return
	}
	parts := appearanceKey.FindStringSubmatch(line)
	if parts == nil {
		return
	}
	key := parts[2]
	value := p.ui.Theme
	switch key {
	case "language":
		value = p.ui.Language
	case "accent":
		value = p.ui.Accent
	}
	p.lines[i] = parts[1] + key + parts[3] + `"` + value + `"` + parts[4] + parts[5]
	p.seen[key] = true
}

func (p *appearancePaint) finish(source string) string {
	result := strings.Join(p.lines, "")
	if p.foundUI {
		if p.inside {
			if !strings.HasSuffix(result, "\n") {
				result += "\n"
			}
			result += missingAppearance(p.seen, p.ui)
		}
		return result
	}
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result + "\n[ui]\n" + missingAppearance(p.seen, p.ui)
}

func missingAppearance(seen map[string]bool, ui UIConfig) string {
	result := ""
	if !seen["language"] {
		result += `language = "` + ui.Language + `"` + "\n"
	}
	if !seen["theme"] {
		result += `theme = "` + ui.Theme + `"` + "\n"
	}
	if !seen["accent"] {
		result += `accent = "` + ui.Accent + `"` + "\n"
	}
	return result
}
