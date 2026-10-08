// Environment key blocks: the fenced credentials agents export.
package codingclients

import (
	"strings"
)

// EnvKeyName is the variable one integration expands for its Relo key.
func EnvKeyName(id string) string {
	return "RELO_" + strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(id), "-", "_")) + "_API_KEY"
}

func envFenceStart(id string) string {
	return "# >>> relo " + id + " >>>"
}

func envFenceEnd(id string) string {
	return "# <<< relo " + id + " <<<"
}

// EnvBlock is the login-shell fragment that exports one integration's key
// from Relo's key file.
func EnvBlock(id, keyFile string) string {
	return strings.Join([]string{
		envFenceStart(id),
		"export " + EnvKeyName(id) + "=\"$(tr -d '\\n' < " + shellSingleQuote(keyFile) + ")\"",
		envFenceEnd(id),
	}, "\n") + "\n"
}

// EnvBlockFish is the fish fragment that exports one integration's key
// from Relo's key file.
func EnvBlockFish(id, keyFile string) string {
	return strings.Join([]string{
		envFenceStart(id),
		"set -gx " + EnvKeyName(id) + " (tr -d '\\n' < " + shellSingleQuote(keyFile) + ")",
		envFenceEnd(id),
	}, "\n") + "\n"
}

// MergeEnvBlock puts one integration's fenced block into a file, replacing
// a block Relo already wrote for the same client.
func MergeEnvBlock(existing, id, block string) string {
	eol := lineEnding(existing)
	stripped := StripEnvFile(existing, id)
	block = strings.ReplaceAll(block, "\n", eol)
	if strings.TrimSpace(stripped) == "" {
		return block
	}
	return strings.TrimSuffix(stripped, eol) + eol + eol + block
}

// MergeEnvFile puts one integration's export into a login-shell file, replacing
// a block Relo already wrote for the same client.
func MergeEnvFile(existing, id, keyFile string) string {
	return MergeEnvBlock(existing, id, EnvBlock(id, keyFile))
}

// StripEnvFile removes one integration's export and leaves every other line.
func StripEnvFile(existing, id string) string {
	if strings.TrimSpace(existing) == "" {
		return ""
	}
	eol := lineEnding(existing)
	start, end := envFenceStart(id), envFenceEnd(id)
	kept := make([]string, 0, 8)
	inside := false
	for _, line := range splitLines(existing) {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == start:
			inside = true
		case trimmed == end && inside:
			inside = false
		case !inside:
			kept = append(kept, line)
		}
	}
	for len(kept) > 0 && strings.TrimSpace(kept[len(kept)-1]) == "" {
		kept = kept[:len(kept)-1]
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, eol) + eol
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

// PublishEnv writes the login-shell block and the live environment for one
// integration that reads a Relo-named variable.
func PublishEnv(paths Paths, agent Agent) error {
	if strings.TrimSpace(agent.EnvKey) == "" {
		return nil
	}
	if err := writeShellEnv(paths, agent); err != nil {
		return err
	}
	return applyLiveEnv(paths, agent)
}

// EnvPublished reports whether the login environment still carries the
// variable Relo published for one integration: the fenced block in the shell
// files and, where the platform keeps one, the live entry. An integration
// with no variable to publish is published.
func EnvPublished(paths Paths, agent Agent) bool {
	if strings.TrimSpace(agent.EnvKey) == "" {
		return true
	}
	return envPublished(paths, agent)
}

// UnpublishEnv takes one integration's variable back out of the login shell
// and the live environment.
func UnpublishEnv(paths Paths, agent Agent) error {
	if strings.TrimSpace(agent.EnvKey) == "" {
		return nil
	}
	if err := stripShellEnv(paths, agent); err != nil {
		return err
	}
	return clearLiveEnv(paths, agent)
}
