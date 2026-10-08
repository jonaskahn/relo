// Tray config watching: reloading on operator changes.
package desktop

import (
	"os"

	"github.com/jonaskahn/relo/internal/config"
)

func (a *app) watchConfig() {
	path := config.ConfigPath(a.opts.Home)
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	a.configMu.Lock()
	if !info.ModTime().After(a.configTime) {
		a.configMu.Unlock()
		return
	}
	a.configTime = info.ModTime()
	a.configMu.Unlock()
	loaded, err := config.LoadConfig(path, nil)
	if err != nil {
		a.logger.Warn("could not reload the tray configuration", "error", err)
		return
	}
	a.applyLanguage(loaded.UI.Language)
}
