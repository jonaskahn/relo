// Client config targets: resolving each integration's files.
package codingclients

import (
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
	// FileZCode is ZCode's configuration file.
	FileZCode = "zcode-config"
	// FileClineProviders is Cline's provider settings file.
	FileClineProviders = "cline-providers"
	// FileClineModels is Cline's model catalog, beside the provider settings.
	FileClineModels = "cline-models"
	// FileOMP is Oh My Pi's model catalog.
	FileOMP = "omp-models"
	// FileDSH is the DeepSeek Harness Cordis patch carrying the profile web.
	FileDSH = "dsh-settings"
	// FileDSHHeadless is the Cordis patch of the profile the command line runs.
	FileDSHHeadless = "dsh-headless"
	// FileDSHDesktop is the Cordis patch of the profile the desktop app boots.
	// The app owns that profile outright, so the patch is only written once
	// the app has created it.
	FileDSHDesktop = "dsh-desktop"
	// FileMCode is MiniMax Code's configuration file.
	FileMCode = "mcode-config"
	// FileRaycast is Raycast AI's provider list.
	FileRaycast = "raycast-providers"
	// FileKimi is Kimi Code's configuration file.
	FileKimi = "kimi-config"
)

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

// dshPatch returns the Cordis patch a shipped harness profile reads,
// honouring DSH_HOME when the operator keeps it somewhere else. Only a shipped
// name is safe to write: the harness fills the rest of that profile in on its
// first run, and refuses a profile folder that holds a patch and nothing else
// when no template names it.
func (p Paths) dshPatch(profile string) (string, error) {
	root := filepath.Join(p.Home, ".dsh")
	if raw := strings.TrimSpace(os.Getenv("DSH_HOME")); raw != "" {
		resolved, err := absolutePath(raw, p.Home, "DSH_HOME")
		if err != nil {
			return "", err
		}
		root = resolved
	}
	return filepath.Join(root, "profiles", profile, "cordis.patch.yml"), nil
}

// dshTargets are the three profiles a harness boot reads: the web profile,
// which serves the browser, the headless profile, which serves the command
// line, and the desktop app's own profile. Each has its own kind because a
// file record is keyed by integration and kind.
//
// The desktop profile is written only where the desktop app has already
// created it. That app owns the directory outright and refuses a profile
// folder holding a patch and nothing else, so Relo creating the profile would
// leave the app unable to start.
func (p Paths) dshTargets() ([]ConfigTarget, error) {
	targets := make([]ConfigTarget, 0, 3)
	for _, profile := range []struct {
		kind, name string
	}{
		{FileDSH, "web"},
		{FileDSHHeadless, "headless"},
		{FileDSHDesktop, "desktop"},
	} {
		patch, err := p.dshPatch(profile.name)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(patch)
		if profile.kind == FileDSHDesktop {
			if _, err := os.Stat(dir); err != nil {
				continue
			}
		}
		targets = append(targets, ConfigTarget{Kind: profile.kind, Path: patch, Dir: dir})
	}
	return targets, nil
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
func (p Paths) ConfigTargets(id string) ([]ConfigTarget, error) {
	switch id {
	case ClientPi:
		return fixedTarget(FilePi, filepath.Join(p.Home, ".pi", "agent", "models.json")), nil
	case ClientZCode:
		return fixedTarget(FileZCode, filepath.Join(p.Home, ".zcode", "v2", "config.json")), nil
	case ClientOMP:
		return fixedTarget(FileOMP, filepath.Join(p.Home, ".omp", "agent", "models.yml")), nil
	case ClientDSH:
		return p.dshTargets()
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
	default:
		return nil, fmt.Errorf("%s: %w", id, ErrUnrecognisedFile)
	}
}

func fixedTarget(kind, path string) []ConfigTarget {
	return []ConfigTarget{{Kind: kind, Path: path, Dir: filepath.Dir(path)}}
}
