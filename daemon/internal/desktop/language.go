// Tray localization: the operator's language for menus.
package desktop

import (
	"github.com/gogpu/systray"

	"github.com/jonaskahn/relo/internal/i18n"
)

func (a *app) text(id string, data map[string]any) string {
	return a.translator().Text(id, data)
}

func (a *app) translator() *i18n.Translator {
	return a.opts.Catalogs.Translate(a.languageNow())
}

func (a *app) languageNow() string {
	a.languageMu.Lock()
	defer a.languageMu.Unlock()
	return a.language
}

func (a *app) notify(id string, data map[string]any) {
	a.tray.ShowNotification("Relo", a.text(id, data))
}

func (a *app) buildLanguageMenu() *systray.Menu {
	menu := systray.NewMenu()
	a.languageItems = make(map[string]*systray.MenuItem, len(i18n.Supported())+1)
	for _, tag := range append([]string{i18n.Auto}, i18n.Supported()...) {
		choice := tag
		item := menu.AddCheckbox(a.languageLabel(choice), a.languageNow() == choice, func() {
			a.selectLanguage(choice)
		})
		a.languageItems[choice] = item
	}
	return menu
}

func (a *app) languageLabel(tag string) string {
	if tag == i18n.Auto {
		return a.text("language.auto", nil)
	}
	return a.text("language.name."+tag, nil)
}

func (a *app) selectLanguage(tag string) {
	go func() {
		if a.opts.SetLanguage == nil {
			a.applyLanguage(tag)
			return
		}
		if err := a.opts.SetLanguage(tag); err != nil {
			a.logger.Warn("could not store the language", "error", err)
			a.notify("tray.notify.language_failed", nil)
			return
		}
		a.applyLanguage(tag)
	}()
}

func (a *app) applyLanguage(tag string) {
	if tag == "" {
		return
	}
	a.languageMu.Lock()
	changed := a.language != tag
	a.language = tag
	a.languageMu.Unlock()
	if !changed {
		return
	}
	a.relabel()
}

func (a *app) relabel() {
	localized := a.translator()
	state, addr, lastErr := a.opts.Control.State()
	a.itemDashboard.SetLabel(localized.Text("tray.menu.open_dashboard", nil))
	a.relabelUpdate(localized)
	a.itemRestart.SetLabel(localized.Text("tray.menu.restart", nil))
	a.itemForce.SetLabel(localized.Text("tray.menu.force_restart", nil))
	a.itemAutostart.SetLabel(localized.Text("tray.menu.autostart", nil))
	a.itemLanguage.SetLabel(localized.Text("tray.menu.language", nil))
	a.itemQuit.SetLabel(localized.Text("tray.menu.quit", nil))
	a.itemQuitHide.SetLabel(localized.Text("tray.menu.quit_hide", nil))
	a.itemQuitStop.SetLabel(localized.Text("tray.menu.quit_shutdown", nil))
	a.itemStatus.SetLabel(statusLine(state, addr, lastErr, localized))
	a.tray.SetTooltip(localized.Text("tray.tooltip", nil))
	a.refreshRows()
	a.relabelLanguages()
}

func (a *app) relabelUpdate(localized *i18n.Translator) {
	if a.itemUpdate == nil {
		return
	}
	if a.itemUpdate.IsDisabled() {
		a.itemUpdate.SetLabel(localized.Text("tray.menu.version", map[string]any{
			"Version": displayOr(a.opts.Version, dash),
		}))
		return
	}
	a.updateMu.Lock()
	latest := a.updateLatest
	a.updateMu.Unlock()
	a.itemUpdate.SetLabel(localized.Text("tray.menu.update_version", map[string]any{"Version": latest}))
}

func (a *app) relabelLanguages() {
	for tag, item := range a.languageItems {
		item.SetLabel(a.languageLabel(tag))
		item.SetChecked(tag == a.languageNow())
	}
}
