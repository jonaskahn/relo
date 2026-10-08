//go:build unix

package i18n

import (
	"os"
	"strings"
)

// System returns the locale the shell asked for as a language tag, or
// nothing when the environment names no language.
func System() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if tag := localeTag(os.Getenv(name)); tag != "" {
			return tag
		}
	}
	return ""
}

func localeTag(value string) string {
	if cut := strings.IndexAny(value, ".@"); cut >= 0 {
		value = value[:cut]
	}
	switch value {
	case "", "C", "POSIX":
		return ""
	}
	return strings.ReplaceAll(value, "_", "-")
}
