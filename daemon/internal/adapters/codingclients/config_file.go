// Client config files: reading, installing, and model references.
package codingclients

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/catalog"
)

var (
	// ErrNotInstalled reports an agent whose configuration directory is not
	// on this machine. Relo does not create that directory.
	ErrNotInstalled = errors.New("the agent is not installed")
	// ErrRelativePath reports an environment override that is not absolute,
	// so Relo will not guess which file it names.
	ErrRelativePath = errors.New("the configuration path is not absolute")
	// ErrForeignProvider reports a relo provider already in a file Relo does
	// not own. Replacing it takes an explicit overwrite.
	ErrForeignProvider = errors.New("the file already has a relo provider")
)

// ModelRef is one model a client configuration names. A limit or a capability
// is carried only when the catalog already knows it. A nil capability is
// unknown, and a client config omits the field rather than inventing it.
type ModelRef struct {
	ID            string
	Name          string
	ContextWindow *int64
	MaxOutput     *int64
	Tools         *bool
	Reasoning     *bool
	Vision        *bool
	// Connection is the label of the connection serving the model, empty for
	// a route. Upstream is the provider's own model identifier, and ChatGPT
	// marks a model served by the ChatGPT sign-in, which Codex knows natively.
	Connection string
	Upstream   string
	ChatGPT    bool
	// ReasoningEfforts are the effort values this model accepts. Nil means the
	// model stated no ladder, so a client offers the shared list. An empty
	// slice means the model stated that it takes none.
	ReasoningEfforts []string
}

// ReasoningEfforts are the picker levels a client offers when a model supports
// reasoning. They are not a default sent on every request. The copy they
// return keeps callers from mutating the shipped levels.
func ReasoningEfforts() []string {
	return []string{"low", "medium", "high", "xhigh", "max"}
}

func modelEfforts(model ModelRef) []string {
	if model.ReasoningEfforts != nil {
		return model.ReasoningEfforts
	}
	return ReasoningEfforts()
}

// Installed reports whether the agent's configuration directory exists.
func Installed(dir string) error {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotInstalled
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", dir, err)
	}
	if !info.IsDir() {
		return ErrNotInstalled
	}
	return nil
}

// ReadRegular returns a configuration file's bytes. A missing file is empty,
// which is a file Relo may create. A symlink or anything else is refused.
func ReadRegular(path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w", ErrUnrecognisedFile)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(content), nil
}

// Fragment renders the provider block Relo would write, without touching a file.
func Fragment(id, baseURL string, models []ModelRef) (string, error) {
	switch id {
	case ClientOpenCode:
		return openCodeFragment(baseURL, models)
	case ClientHermes:
		return hermesFragment(baseURL, models)
	case ClientOpenClaw:
		return openClawFragment(baseURL, models)
	default:
		return "", fmt.Errorf("%s: %w", id, ErrUnrecognisedFile)
	}
}

// MergeProvider returns the client's configuration with Relo's provider in it.
func MergeProvider(id, existing, baseURL string, models []ModelRef) (string, error) {
	switch id {
	case ClientOpenCode:
		return MergeOpenCode(existing, baseURL, models)
	case ClientHermes:
		return MergeHermes(existing, baseURL, models)
	case ClientOpenClaw:
		return MergeOpenClaw(existing, baseURL, models)
	default:
		return "", fmt.Errorf("%s: %w", id, ErrUnrecognisedFile)
	}
}

// ProviderPointsAt reports whether the relo provider in a client file is the
// one Relo would write. The rest of the document is the client's own.
func ProviderPointsAt(id, existing, baseURL string, models []ModelRef) bool {
	switch id {
	case ClientOpenCode:
		return openCodePointsAt(existing, baseURL, models)
	case ClientHermes:
		return hermesPointsAt(existing, baseURL, models)
	case ClientOpenClaw:
		return openClawPointsAt(existing, baseURL, models)
	default:
		return false
	}
}

// HasReloProvider reports whether the document already names a relo provider.
func HasReloProvider(id, existing string) (bool, error) {
	switch id {
	case ClientOpenCode:
		return openCodeHasRelo(existing)
	case ClientHermes:
		return hermesHasRelo(existing)
	case ClientOpenClaw:
		return openClawHasRelo(existing)
	default:
		return false, fmt.Errorf("%s: %w", id, ErrUnrecognisedFile)
	}
}

// ConfigDisplayName is the picker label Relo writes into a client file.
func ConfigDisplayName(model ModelRef) string {
	name := strings.TrimSpace(model.Name)
	if name == "" {
		name = model.ID
	}
	return catalog.ConfigModelName(name, model.Connection, model.ContextWindow)
}

func namedConfigLabel(model ModelRef) string {
	if strings.TrimSpace(model.Name) == "" {
		return ""
	}
	return ConfigDisplayName(model)
}

func orderedModels(models []ModelRef) []ModelRef {
	copied := append([]ModelRef(nil), models...)
	sort.Slice(copied, func(i, j int) bool { return copied[i].ID < copied[j].ID })
	kept := copied[:0]
	for _, model := range copied {
		if strings.TrimSpace(model.ID) == "" {
			continue
		}
		kept = append(kept, model)
	}
	return kept
}

func requireBase(baseURL string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", fmt.Errorf("%w (no address is served)", ErrUnrecognisedFile)
	}
	return strings.TrimSuffix(trimmed, "/") + "/v1", nil
}

func ensureNewline(content string) string {
	if content == "" || strings.HasSuffix(content, "\n") {
		return content
	}
	return content + "\n"
}

func emptyDocument(content string) bool {
	trimmed := strings.TrimSpace(content)
	return trimmed == "" || emptyObject(trimmed)
}

// StripProvider removes the provider Relo wrote and leaves the rest of the file.
func StripProvider(id, existing string) (string, error) {
	switch id {
	case ClientOpenCode:
		return StripOpenCode(existing)
	case ClientHermes:
		return StripHermes(existing)
	case ClientOpenClaw:
		return StripOpenClaw(existing)
	default:
		return "", fmt.Errorf("%s: %w", id, ErrUnrecognisedFile)
	}
}
