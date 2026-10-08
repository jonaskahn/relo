// Operator language resolution from home and request.
package platform

import (
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/i18n"
)

// ResolveLanguage resolves the language in force for a state directory:
// the flag, the config file, and the system locale, highest first. Every
// read here is best-effort, so a surface that reports a broken state
// directory still speaks. An empty home reads only the system locale, which
// is what a helper with no state directory to consult wants.
func ResolveLanguage(home, language string) (i18n.Resolution, error) {
	layers := i18n.Options{
		Flag: language, System: i18n.System(),
	}
	if home != "" {
		if tag, ok, err := config.ConfiguredLanguage(config.ConfigPath(home)); err == nil && ok {
			layers.Config = tag
		}
	}
	return i18n.Resolve(layers)
}
