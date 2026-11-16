package platform

import (
	"context"
	"log/slog"
	"runtime"

	"github.com/jonaskahn/relo/internal/adapters/codingclients"
	"github.com/jonaskahn/relo/internal/adapters/ostrust"
	"github.com/jonaskahn/relo/internal/adapters/proxy"
	appintegration "github.com/jonaskahn/relo/internal/application/integration"
)

// desktopPickerParts is what the picker is built over.
type desktopPickerParts struct {
	home    string
	paths   codingclients.Paths
	runtime *ProxyRuntime
	store   appintegration.Store
	catalog func() []proxy.CatalogRow
	logger  *slog.Logger
}

// newDesktopPicker builds the picker's runtime over this machine: its OS trust
// store, Desktop's config library, and Relo's one root.
func newDesktopPicker(parts desktopPickerParts) *DesktopPicker {
	return NewDesktopPicker(DesktopPickerOptions{
		Home: parts.home, Platform: runtime.GOOS, Library: parts.paths.DesktopLibrary,
		Installed: parts.paths.DesktopInstalled, Authority: parts.runtime.Authority,
		Trust: ostrust.New(runtime.GOOS, homeDirectory(""), nil), Catalog: parts.catalog,
		DesktopWired: func(ctx context.Context) bool { return desktopInProxyMode(ctx, parts.store) },
		Intercept:    parts.runtime.Intercept, Logger: parts.logger,
	})
}

// desktopInProxyMode reports whether Claude Desktop is on and wired through the
// intercept, which is what the picker rides.
func desktopInProxyMode(ctx context.Context, store appintegration.Store) bool {
	record, found, err := store.Integration(ctx, appintegration.ProxyDesktop)
	if err != nil || !found || !record.Enabled {
		return false
	}
	mode, _ := appintegration.NormalizeMode(record.Mode)
	return mode == appintegration.ModeProxy
}
