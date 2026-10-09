package tray_test

import (
	"strings"
	"testing"

	"github.com/jonaskahn/relo/internal/i18n"
)

// TestTrayMessagesSpeakEveryLanguage covers the text the menu draws: every
// tray message exists in every catalog and renders with the values the tray
// passes it.
func TestTrayMessagesSpeakEveryLanguage(t *testing.T) {
	catalogs, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	data := map[string]any{
		"Addr": "127.0.0.1:10101", "Detail": "connection refused", "Value": "$4.20",
		"Version": "0.2.0",
	}
	ids := []string{
		"tray.tooltip", "tray.menu.open_dashboard", "tray.menu.restart", "tray.menu.force_restart",
		"tray.menu.autostart", "tray.menu.language", "tray.menu.version", "tray.menu.update_version",
		"tray.menu.quit",
		"tray.status.running", "tray.status.stopped", "tray.status.failed",
		"tray.info.requests", "tray.info.tokens", "tray.info.spend",
		"tray.error.unknown", "tray.error.not_ready", "tray.error.port_in_use",
		"tray.error.timed_out", "tray.error.database", "tray.error.start_failed",
		"tray.notify.failed", "tray.notify.browser_failed",
		"tray.notify.autostart_failed", "tray.notify.language_failed",
		"language.auto", "language.name.en", "language.name.de", "language.name.zh-Hans",
		"language.name.cs", "language.name.es", "language.name.fr",
		"language.name.hi", "language.name.id", "language.name.it", "language.name.ja",
		"language.name.ko", "language.name.nl", "language.name.pl", "language.name.pt-BR",
		"language.name.ro", "language.name.ru", "language.name.sv", "language.name.th",
		"language.name.tr", "language.name.uk", "language.name.vi", "language.name.zh-Hant",
	}
	for _, tag := range catalogs.Supported() {
		translator := catalogs.Translate(tag)
		for _, id := range ids {
			text := translator.Text(id, data)
			if strings.TrimSpace(text) == "" || strings.Contains(text, "{{") {
				t.Errorf("%s: %s rendered %q", tag, id, text)
			}
		}
	}
}
