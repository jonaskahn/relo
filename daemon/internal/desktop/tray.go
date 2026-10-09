// Package desktop runs the tray icon that fronts the daemon: one process
// serves the proxy and shows the icon, the icon carries the proxy's status,
// the day's numbers and its lifecycle actions, and it opens the browser
// dashboard.
//
// The icon pumps the platform message loop on the calling goroutine, which
// must be the process main goroutine (cmd/relo locks it). A process the
// platform gives no desktop session, or one whose display server is
// missing, serves without an icon.
package desktop

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gogpu/systray"

	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/internal/platform/autostart"
)

const refreshInterval = 5 * time.Second

const activityInterval = 30 * time.Second

const updateInterval = time.Hour

// Control is the proxy lifecycle the tray drives. The supervisor in the
// composition root implements it.
type Control interface {
	// Restart cycles the proxy.
	Restart() error
	// ForceRestart cycles the proxy after freeing the configured ports.
	ForceRestart() error
	// Stop ends the proxy and returns once it has drained.
	Stop() error
	// State reports where the proxy stands, its address while running, and
	// the error of the last failed run.
	State() (platform.DaemonState, string, error)
	// Busy reports a lifecycle action in progress.
	Busy() bool
	// OnStateChange registers a callback invoked after every change, so the
	// menu redraws from what State reports.
	OnStateChange(func())
}

type tray interface {
	// Show puts the icon in the notification area and pumps the platform
	// loop on the calling goroutine.
	Show() *systray.SystemTray
	// Remove takes the icon down and ends the loop.
	Remove()
	// SetTooltip replaces the hover text.
	SetTooltip(text string) *systray.SystemTray
	// ShowNotification raises one OS notification.
	ShowNotification(title, message string) *systray.SystemTray
	// Hide takes the icon out of the notification area without ending the
	// process.
	Hide() *systray.SystemTray
}

// Options configures the tray.
type Options struct {
	// Home is the state directory this run serves: where the startup file
	// and the database live.
	Home string
	// Control drives the proxy running in this process.
	Control Control
	// Version is the build version the menu reports.
	Version string
	// URL is the dashboard address to open while the proxy is stopped and
	// its live address is unknown.
	URL string
	// AdminToken authenticates the tray's own reads of the management API.
	AdminToken string
	// Autostart is the configured default: when true, the run registers Relo
	// to start at login if no registration exists yet.
	Autostart bool
	// Language is the language the tray speaks: what this run resolved for
	// the operator before it started.
	Language string
	// Catalogs holds the messages the tray renders.
	Catalogs *i18n.Catalogs
	// SetLanguage stores the language an operator picked from the menu. The
	// command line owns the startup file, so the tray asks for the write.
	SetLanguage func(tag string) error
	// LogPath is the daemon log a failure notification points at.
	LogPath string
	// Logger receives the tray's own log lines.
	Logger *slog.Logger
	// Source reads the last day's usage the menu shows. The command line
	// injects the database-backed reader; without one the menu shows no
	// activity.
	Source ActivitySource
}

// Run shows the tray and blocks until it quits: the operator quit, the
// proxy ended, or ctx was cancelled. Without a desktop session it waits for
// the proxy and carries its failure back instead.
func Run(ctx context.Context, opts Options) error {
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if showTray() {
		return newApp(opts).run(ctx)
	}
	opts.Logger.Info("the proxy serves without a tray icon", "display", displayAvailable())
	return waitForProxy(ctx, opts.Control)
}

func showTray() bool {
	return !testing.Testing() && displayAvailable()
}

func waitForProxy(ctx context.Context, control Control) error {
	ended := make(chan struct{}, 1)
	control.OnStateChange(func() {
		state, _, _ := control.State()
		if state == platform.StateStopped || state == platform.StateFailed {
			select {
			case ended <- struct{}{}:
			default:
			}
		}
	})
	select {
	case <-ctx.Done():
		return nil
	case <-ended:
	}
	if state, _, err := control.State(); state == platform.StateFailed {
		return err
	}
	return nil
}

type app struct {
	opts   Options
	logger *slog.Logger
	client *http.Client
	source *desktopSource
	tray   tray
	// trayApp is the same tray as its own type, for the calls the tray
	// interface does not carry: the platform loop and its menu roles.
	trayApp *systray.SystemTray

	itemStatus    *systray.MenuItem
	itemDashboard *systray.MenuItem
	itemRestart   *systray.MenuItem
	itemForce     *systray.MenuItem
	itemAutostart *systray.MenuItem
	itemLanguage  *systray.MenuItem
	itemQuit      *systray.MenuItem
	itemUpdate    *systray.MenuItem
	// The three rows carry the last day's numbers.
	itemRequests *systray.MenuItem
	itemTokens   *systray.MenuItem
	itemSpend    *systray.MenuItem

	// languageItems are the language choices, keyed by tag, so a change
	// re-labels them in the language that just took effect.
	languageItems map[string]*systray.MenuItem

	languageMu sync.Mutex
	language   string
	configMu   sync.Mutex
	configTime time.Time

	activityMu sync.Mutex
	activity   activityData

	updateMu     sync.Mutex
	updateURL    string
	updateMethod string
	updateLatest string

	// lastState is the state the menu last drew, so a failure is reported
	// once per transition.
	lastState platform.DaemonState
	// ended is closed when the proxy ends on its own, which is when the icon
	// leaves with it. A failed run keeps the icon so it can be restarted.
	ended   chan struct{}
	endOnce sync.Once
	// quitting freezes actions until the bounded record save finishes.
	quitting    atomic.Bool
	quit        sync.Once
	menuMu      sync.Mutex
	stopPolling context.CancelFunc
	pollContext context.Context
}

func newApp(opts Options) *app {
	a := &app{
		opts:      opts,
		logger:    opts.Logger,
		client:    &http.Client{},
		language:  opts.Language,
		ended:     make(chan struct{}),
		lastState: platform.StateRunning,
	}
	if opts.Source != nil {
		a.source = newDesktopSource(opts.Source)
	}
	a.trayApp = a.buildTray()
	a.tray = a.trayApp
	return a
}

func (a *app) run(ctx context.Context) error {
	pollCtx, stopPolling := context.WithCancel(ctx)
	a.stopPolling = stopPolling
	a.pollContext = pollCtx
	defer stopPolling()
	a.opts.Control.OnStateChange(a.refresh)
	a.tray.Show()
	// The Dock presence is the operator's: a tray app is an accessory, and
	// leaving the regular policy on would put an icon in the Dock with no
	// window to reach.
	applyAccessoryPresence(a.logger)
	a.trayApp.RemoveAppMenuRoles("orderFrontStandardAboutPanel:", "orderFrontPreferencesPanel:")
	a.reconcileAutostart()

	done := make(chan struct{})
	go a.refreshLoop(pollCtx, done)
	go a.activityLoop(pollCtx, done)
	go a.updateLoop(pollCtx, done)
	go func() {
		select {
		case <-ctx.Done():
			_ = a.opts.Control.Stop()
		case <-a.ended:
		}
		a.leave()
	}()

	err := a.trayApp.Run()
	close(done)
	return err
}

func (a *app) buildTray() *systray.SystemTray {
	tray := systray.New()
	menu := systray.NewMenu()

	a.addDashboardItems(menu)
	a.addMetricItems(menu)
	a.addProxyItems(menu)
	a.addPreferenceItems(menu)
	a.addUpdateItem(menu)
	a.addQuitItems(menu)

	tray.SetTooltip(a.text("tray.tooltip", nil)).SetMenu(menu).SetAppName("Relo")
	if runtime.GOOS == "darwin" {
		// The menu bar wants a monochrome template image; the system tints
		// it to match the bar in either appearance.
		tray.SetTemplateIcon(TemplateIcon())
	} else {
		tray.SetIcon(Icon())
	}
	// A left click opens the dashboard; the menu stays on right click.
	tray.OnClick(a.openDashboard)
	return tray
}

func (a *app) addDashboardItems(menu *systray.Menu) {
	a.itemDashboard = menu.Add(a.text("tray.menu.open_dashboard", nil), a.openDashboard)
	menu.AddSeparator()
	a.itemStatus = menu.Add(statusLine(platform.StateRunning, "", nil, a.translator()), nil)
	a.itemStatus.SetDisabled(true)
}

func (a *app) addMetricItems(menu *systray.Menu) {
	a.itemRequests = menu.Add(a.text("tray.info.requests", map[string]any{"Value": dash}), nil)
	a.itemTokens = menu.Add(a.text("tray.info.tokens", map[string]any{"Value": dash}), nil)
	a.itemSpend = menu.Add(a.text("tray.info.spend", map[string]any{"Value": dash}), nil)
	a.itemRequests.SetDisabled(true)
	a.itemTokens.SetDisabled(true)
	a.itemSpend.SetDisabled(true)
	menu.AddSeparator()
}

func (a *app) addProxyItems(menu *systray.Menu) {
	a.itemRestart = menu.Add(a.text("tray.menu.restart", nil), a.restartProxy)
	a.itemForce = menu.Add(a.text("tray.menu.force_restart", nil), a.forceRestartProxy)
	menu.AddSeparator()
}

func (a *app) addPreferenceItems(menu *systray.Menu) {
	a.itemAutostart = menu.AddCheckbox(a.text("tray.menu.autostart", nil), a.autostartEnabled(), a.toggleAutostart)
	a.itemLanguage = menu.AddSubmenu(a.text("tray.menu.language", nil), a.buildLanguageMenu())
	menu.AddSeparator()
}

func (a *app) addUpdateItem(menu *systray.Menu) {
	a.itemUpdate = menu.Add(a.text("tray.menu.version", map[string]any{"Version": displayOr(a.opts.Version, dash)}), a.openUpdate)
	a.itemUpdate.SetDisabled(true)
	// The version stays out of the menu until there is something to say;
	// refreshUpdate shows the row when a newer release exists.
	a.itemUpdate.SetVisible(false)
	menu.AddSeparator()
}

func (a *app) addQuitItems(menu *systray.Menu) {
	// Quitting ends the proxy along with the icon.
	a.itemQuit = menu.Add(a.text("tray.menu.quit", nil), func() {
		a.beginQuit()
		go a.stopThenLeave()
	})
}

func (a *app) refresh() {
	control := a.opts.Control
	state, addr, lastErr := control.State()
	if state == platform.StateStopping {
		a.beginQuit()
		return
	}
	a.menuMu.Lock()
	defer a.menuMu.Unlock()
	if a.quitting.Load() {
		if state == platform.StateStopped {
			a.report(state, lastErr)
		}
		return
	}
	busy := control.Busy()
	localized := a.translator()
	a.itemStatus.SetLabel(statusLine(state, addr, lastErr, localized))
	// A restart is never queued behind another: the force item exists to
	// escape a run that is already stuck.
	a.itemRestart.SetDisabled(busy)
	if state != a.lastState {
		a.lastState = state
		a.report(state, lastErr)
	}
}

func (a *app) report(state platform.DaemonState, lastErr error) {
	switch state {
	case platform.StateStopped:
		a.endOnce.Do(func() { close(a.ended) })
	case platform.StateFailed:
		a.logger.Warn("the proxy run failed", "error", errText(lastErr), "log", a.opts.LogPath)
		a.notify("tray.notify.failed", map[string]any{"Detail": shortError(lastErr, a.translator())})
	}
}

func (a *app) refreshLoop(ctx context.Context, done <-chan struct{}) {
	a.watchConfig()
	a.refresh()
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			a.watchConfig()
			a.refresh()
		}
	}
}

func (a *app) activityLoop(ctx context.Context, done <-chan struct{}) {
	a.loadActivity()
	a.refreshRows()
	ticker := time.NewTicker(activityInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			a.loadActivity()
			a.refreshRows()
		}
	}
}

func (a *app) updateLoop(ctx context.Context, done <-chan struct{}) {
	a.refreshUpdate()
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		case <-ticker.C:
			a.refreshUpdate()
		}
	}
}

func (a *app) loadActivity() {
	if a.source == nil {
		return
	}
	data, err := a.source.Read(a.pollingContext(), time.Now())
	if err != nil {
		if a.quitting.Load() {
			return
		}
		a.logger.Warn("the tray could not read activity", "error", err)
		data.err = err.Error()
	}
	a.activityMu.Lock()
	a.activity = data
	a.activityMu.Unlock()
}

func (a *app) activityNow() activityData {
	a.activityMu.Lock()
	defer a.activityMu.Unlock()
	return a.activity
}

func (a *app) refreshRows() {
	a.menuMu.Lock()
	defer a.menuMu.Unlock()
	if a.quitting.Load() {
		return
	}
	a.relabelMetricRows()
}

func (a *app) relabelMetricRows() {
	activity := a.activityNow()
	items := [3]*systray.MenuItem{a.itemRequests, a.itemTokens, a.itemSpend}
	keys := [3]string{"tray.info.requests", "tray.info.tokens", "tray.info.spend"}
	for i, item := range items {
		if item == nil {
			continue
		}
		value := dash
		if activity.ok {
			value = formatMetric(activity.totals.value(metric(i)), metric(i))
		}
		item.SetLabel(a.text(keys[i], map[string]any{"Value": value}))
	}
}

func (a *app) baseURL() string {
	if state, addr, _ := a.opts.Control.State(); state == platform.StateRunning && addr != "" {
		return httpURL(addr)
	}
	return a.opts.URL
}

func (a *app) restartProxy() {
	if a.opts.Control.Busy() || a.quitting.Load() {
		return
	}
	go func() {
		if !a.quitting.Load() {
			a.reportCycle(a.opts.Control.Restart())
		}
	}()
}

func (a *app) forceRestartProxy() {
	if a.quitting.Load() {
		return
	}
	go func() {
		if !a.quitting.Load() {
			a.reportCycle(a.opts.Control.ForceRestart())
		}
	}()
}

func (a *app) reportCycle(err error) {
	if a.quitting.Load() {
		return
	}
	if err == nil {
		return
	}
	a.logger.Warn("the proxy did not restart", "error", err, "log", a.opts.LogPath)
	a.notify("tray.notify.failed", map[string]any{"Detail": shortError(err, a.translator())})
}

func (a *app) openUpdate() {
	if a.quitting.Load() {
		return
	}
	a.updateMu.Lock()
	target := a.updateURL
	method := a.updateMethod
	a.updateMu.Unlock()
	if method == "sparkle" {
		if err := requestSparkleCheck(); err != nil {
			a.notify("tray.notify.browser_failed", nil)
		}
		return
	}
	if target == "" {
		return
	}
	if err := Open(target); err != nil {
		a.notify("tray.notify.browser_failed", nil)
	}
}

func (a *app) refreshUpdate() {
	if a.quitting.Load() {
		return
	}
	info, err := FetchUpdates(a.pollingContext(), a.client, a.baseURL(), a.opts.AdminToken)
	if err != nil || a.itemUpdate == nil {
		return
	}
	a.menuMu.Lock()
	defer a.menuMu.Unlock()
	if a.quitting.Load() {
		return
	}
	a.updateMu.Lock()
	a.updateURL = info.URL
	a.updateMethod = info.Method
	a.updateLatest = info.Latest
	a.updateMu.Unlock()
	if info.Available && (info.URL != "" || info.Method == "sparkle") {
		a.itemUpdate.SetVisible(true)
		a.itemUpdate.SetLabel(a.text("tray.menu.update_version", map[string]any{"Version": info.Latest}))
		a.itemUpdate.SetDisabled(false)
		return
	}
	a.itemUpdate.SetVisible(false)
	a.itemUpdate.SetLabel(a.text("tray.menu.version", map[string]any{"Version": displayOr(info.Current, a.opts.Version)}))
	a.itemUpdate.SetDisabled(true)
}

func (a *app) openDashboard() {
	if a.quitting.Load() {
		return
	}
	if err := Open(a.baseURL()); err != nil {
		a.logger.Warn("could not open the dashboard", "error", err)
		a.notify("tray.notify.browser_failed", nil)
	}
}

func (a *app) autostartEnabled() bool { return autostart.New().IsEnabled() }

func (a *app) reconcileAutostart() {
	manager := autostart.New()
	if manager.IsEnabled() || !a.opts.Autostart {
		return
	}
	exe, err := autostart.Executable()
	if err != nil {
		a.logger.Warn("could not resolve the executable for start at login", "error", err)
		return
	}
	if err := manager.Enable(exe); err != nil {
		a.logger.Warn("could not register start at login", "error", err)
		return
	}
	a.logger.Info("registered the app to start at login", "executable", exe)
}

func (a *app) toggleAutostart() {
	if a.quitting.Load() {
		return
	}
	manager := autostart.New()
	var err error
	if manager.IsEnabled() {
		err = manager.Disable()
	} else {
		exe, resolveErr := autostart.Executable()
		if resolveErr != nil {
			err = resolveErr
		} else {
			err = manager.Enable(exe)
		}
	}
	if err != nil {
		a.logger.Warn("could not change the login item", "error", err)
		a.notify("tray.notify.autostart_failed", nil)
	}
	a.itemAutostart.SetChecked(manager.IsEnabled())
}

func (a *app) beginQuit() {
	if !a.quitting.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(platform.SaveTimeout, a.leave)
	if a.stopPolling != nil {
		a.stopPolling()
	}
	a.menuMu.Lock()
	defer a.menuMu.Unlock()
	for _, item := range []*systray.MenuItem{a.itemDashboard, a.itemRestart, a.itemForce, a.itemAutostart, a.itemLanguage, a.itemUpdate, a.itemQuit} {
		if item != nil {
			item.SetDisabled(true)
		}
	}
	for _, item := range a.languageItems {
		item.SetDisabled(true)
	}
	if a.itemQuit != nil {
		label := a.text("tray.menu.quitting", nil)
		if runtime.GOOS == "windows" {
			label = "◌ " + label
		} else {
			a.itemQuit.SetIcon(processingIcon())
		}
		a.itemQuit.SetLabel(label)
	}
}

func (a *app) stopThenLeave() {
	a.beginQuit()
	err := a.opts.Control.Stop()
	a.leave()
	if err != nil {
		a.logger.Warn("could not complete daemon shutdown", "error", err)
	}
}

func (a *app) leave() {
	a.quit.Do(func() {
		a.tray.Remove()
	})
}

func (a *app) pollingContext() context.Context {
	if a.pollContext != nil {
		return a.pollContext
	}
	return context.Background()
}
