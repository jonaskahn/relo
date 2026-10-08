// Client config targets: resolving each integration's files.
package codingclients

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// FileOpenCode is the configuration file Relo merges an OpenCode provider into.
	FileOpenCode = "opencode-config"
	// FileHermes is the configuration file Relo merges a Hermes provider into.
	FileHermes = "hermes-config"
	// FileOpenClaw is the configuration file Relo merges an OpenClaw provider into.
	FileOpenClaw = "openclaw-config"
	// FilePi is Pi's model catalog.
	FilePi = "pi-models"
	// FilePrime is Prime's model catalog.
	FilePrime = "prime-models"
	// FileAside is the Aside account catalog Relo merges into.
	FileAside = "aside-models"
	// FileOMO is OMO's model catalog.
	FileOMO = "omo-models"
	// FileZCode is ZCode's configuration file.
	FileZCode = "zcode-config"
	// FileClineProviders is Cline's provider settings file.
	FileClineProviders = "cline-providers"
	// FileClineModels is Cline's model catalog, beside the provider settings.
	FileClineModels = "cline-models"
	// FileOMP is Oh My Pi's model catalog.
	FileOMP = "omp-models"
	// FileGajae is Gajae's model catalog.
	FileGajae = "gajae-models"
	// FileDSH is the DeepSeek Harness settings file.
	FileDSH = "dsh-settings"
	// FileMCode is MiniMax Code's configuration file.
	FileMCode = "mcode-config"
	// FileRaycast is Raycast AI's provider list.
	FileRaycast = "raycast-providers"
	// FileKimi is Kimi Code's configuration file.
	FileKimi = "kimi-config"
)

// ErrAsideAccount reports that Aside has no single account directory under
// ~/.aside/u, so Relo will not invent an account name.
var ErrAsideAccount = errors.New("aside has no single account directory")

// ConfigTarget is the file one agent reads and the directory that has to
// exist before Relo will create or edit that file.
type ConfigTarget struct {
	Kind string
	Path string
	Dir  string
}

// ConfigTarget resolves the configuration file for an agent Relo writes, honouring
// that agent's own environment. A relative override is refused: the daemon and
// the agent would otherwise disagree about which file a working directory names.
func (p Paths) ConfigTarget(id string) (ConfigTarget, error) {
	switch id {
	case ClientOpenCode:
		return p.configTarget(FileOpenCode, p.OpenCodeConfig)
	case ClientHermes:
		return p.configTarget(FileHermes, p.HermesConfig)
	case ClientOpenClaw:
		return p.configTarget(FileOpenClaw, p.OpenClawConfig)
	default:
		return ConfigTarget{}, fmt.Errorf("%s: %w", id, ErrUnrecognisedFile)
	}
}

func (p Paths) configTarget(kind string, resolve func() (string, error)) (ConfigTarget, error) {
	path, err := resolve()
	target := ConfigTarget{Kind: kind, Path: path}
	if err != nil {
		return target, err
	}
	target.Dir = filepath.Dir(path)
	return target, nil
}

// OpenCodeConfig returns OpenCode's global configuration file.
func (p Paths) OpenCodeConfig() (string, error) {
	root := filepath.Join(p.Home, ".config")
	if raw := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); raw != "" {
		resolved, err := absolutePath(raw, p.Home, "XDG_CONFIG_HOME")
		if err != nil {
			return "", err
		}
		root = resolved
	}
	return filepath.Join(root, "opencode", "opencode.json"), nil
}

// HermesConfig returns Hermes's configuration file.
func (p Paths) HermesConfig() (string, error) {
	root := filepath.Join(p.Home, ".hermes")
	if raw := strings.TrimSpace(os.Getenv("HERMES_HOME")); raw != "" {
		resolved, err := absolutePath(raw, p.Home, "HERMES_HOME")
		if err != nil {
			return "", err
		}
		root = resolved
	}
	return filepath.Join(root, "config.yaml"), nil
}

// OpenClawConfig returns the configuration file OpenClaw reads.
func (p Paths) OpenClawConfig() (string, error) {
	if raw := strings.TrimSpace(os.Getenv("OPENCLAW_CONFIG_PATH")); raw != "" {
		return absolutePath(raw, p.Home, "OPENCLAW_CONFIG_PATH")
	}
	return filepath.Join(p.Home, ".openclaw", "openclaw.json"), nil
}

func absolutePath(raw, home, name string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	switch {
	case trimmed == "~":
		return home, nil
	case strings.HasPrefix(trimmed, "~/"):
		return filepath.Join(home, trimmed[2:]), nil
	case filepath.IsAbs(trimmed):
		return trimmed, nil
	default:
		return "", fmt.Errorf("%s: %w", name, ErrRelativePath)
	}
}

// ConfigTargets lists every client file Relo merges a provider block into.
// Aside resolves the single account directory under ~/.aside/u and refuses
// to guess when that directory is missing or there is more than one.
func (p Paths) ConfigTargets(id string) ([]ConfigTarget, error) {
	switch id {
	case ClientPi:
		return fixedTarget(FilePi, filepath.Join(p.Home, ".pi", "agent", "models.json")), nil
	case ClientPrime:
		return fixedTarget(FilePrime, filepath.Join(p.Home, ".prime", "agent", "models.json")), nil
	case ClientOMO:
		return fixedTarget(FileOMO, filepath.Join(p.Home, ".omo", "agent", "models.json")), nil
	case ClientZCode:
		return fixedTarget(FileZCode, filepath.Join(p.Home, ".zcode", "v2", "config.json")), nil
	case ClientOMP:
		return fixedTarget(FileOMP, filepath.Join(p.Home, ".omp", "agent", "models.yml")), nil
	case ClientGajae:
		return fixedTarget(FileGajae, filepath.Join(p.Home, ".gjc", "agent", "models.yml")), nil
	case ClientDSH:
		return fixedTarget(FileDSH, filepath.Join(p.Home, ".dsh", "settings.yaml")), nil
	case ClientMCode:
		return fixedTarget(FileMCode, filepath.Join(p.Home, ".minimax", "config.yaml")), nil
	case ClientRaycast:
		return fixedTarget(FileRaycast, filepath.Join(p.Home, ".config", "raycast", "ai", "providers.yaml")), nil
	case ClientKimi:
		return fixedTarget(FileKimi, filepath.Join(p.Home, ".kimi-code", "config.toml")), nil
	default:
		return p.specialConfigTargets(id)
	}
}

func (p Paths) specialConfigTargets(id string) ([]ConfigTarget, error) {
	switch id {
	case ClientCline:
		root := filepath.Join(p.Home, ".cline", "data", "settings")
		return []ConfigTarget{
			{Kind: FileClineProviders, Path: filepath.Join(root, "providers.json"), Dir: root},
			{Kind: FileClineModels, Path: filepath.Join(root, "models.json"), Dir: root},
		}, nil
	case ClientAside:
		path, err := p.asideModels()
		if err != nil {
			return nil, err
		}
		return []ConfigTarget{{Kind: FileAside, Path: path, Dir: filepath.Dir(path)}}, nil
	default:
		return nil, fmt.Errorf("%s: %w", id, ErrUnrecognisedFile)
	}
}

func fixedTarget(kind, path string) []ConfigTarget {
	return []ConfigTarget{{Kind: kind, Path: path, Dir: filepath.Dir(path)}}
}

func (p Paths) asideModels() (string, error) {
	root := filepath.Join(p.Home, ".aside", "u")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("aside has no account directory: %w", ErrAsideAccount)
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", root, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	switch len(names) {
	case 0:
		return "", fmt.Errorf("aside has no account directory: %w", ErrAsideAccount)
	case 1:
		return filepath.Join(root, names[0], "models.json"), nil
	default:
		return "", fmt.Errorf("aside has more than one account directory: %w", ErrAsideAccount)
	}
}
