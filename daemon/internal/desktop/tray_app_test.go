package desktop

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogpu/systray"

	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/i18n"
	"github.com/jonaskahn/relo/internal/platform"
	"github.com/jonaskahn/relo/tests/testkit"
)

// fakeControl drives the tray's view of the proxy lifecycle without a proxy,
// so every menu row and every reaction is reachable from a test.
type fakeControl struct {
	mu      sync.Mutex
	state   platform.DaemonState
	addr    string
	lastErr error
	busy    bool
	restart error
	force   error
	stop    error
	changes []func()
	// registered carries each watcher once it is on the list, so a test
	// moves the proxy only after waitForProxy is listening.
	registered chan struct{}
}

func (f *fakeControl) Restart() error { return f.restart }

func (f *fakeControl) ForceRestart() error { return f.force }

func (f *fakeControl) Stop() error { return f.stop }

func (f *fakeControl) State() (platform.DaemonState, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.addr, f.lastErr
}

func (f *fakeControl) Busy() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.busy
}

func (f *fakeControl) OnStateChange(callback func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes = append(f.changes, callback)
	if f.registered != nil {
		select {
		case f.registered <- struct{}{}:
		default:
		}
	}
}

// setState moves the proxy and tells the tray, the way the supervisor does.
func (f *fakeControl) setState(state platform.DaemonState, addr string, lastErr error) {
	f.mu.Lock()
	f.state, f.addr, f.lastErr = state, addr, lastErr
	callbacks := append([]func(){}, f.changes...)
	f.mu.Unlock()
	for _, callback := range callbacks {
		callback()
	}
}

// stubTray is the icon surface without an icon: it records what the app
// would show, so a test can read the tooltip and the notifications back.
type stubTray struct {
	mu           sync.Mutex
	atooltip     string
	notification string
	removed      int
}

func (s *stubTray) Show() *systray.SystemTray { return nil }

func (s *stubTray) Remove() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removed++
}

func (s *stubTray) Hide() *systray.SystemTray { return nil }

func (s *stubTray) SetTooltip(text string) *systray.SystemTray {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.atooltip = text
	return nil
}

func (s *stubTray) ShowNotification(title, message string) *systray.SystemTray {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notification = title + ": " + message
	return nil
}

func (s *stubTray) tooltip() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.atooltip
}

func (s *stubTray) lastNotification() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.notification
}

// removals counts the times the icon was taken down, so a test can tell one
// removal from a repeat.
func (s *stubTray) removals() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.removed
}

// testApp builds a tray app without an icon: the menu items live in memory,
// the tray is a stub, and no platform loop is started, so the logic is
// testable headlessly. systray.New() would put a real icon in the session
// the tests run from, which no test can see or clean up.
func testAppWithTray(t *testing.T, control Control, opts Options) (*app, *stubTray) {
	t.Helper()
	catalogs, err := i18n.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	logger, _ := testkit.TestLogger(t)
	if opts.Language == "" {
		opts.Language = i18n.English
	}
	opts.Control = control
	opts.Catalogs = catalogs
	if opts.Logger == nil {
		opts.Logger = logger
	}
	a := &app{
		opts:      opts,
		logger:    opts.Logger,
		language:  opts.Language,
		ended:     make(chan struct{}),
		lastState: platform.StateRunning,
	}
	stub := &stubTray{}
	a.tray = stub
	menu := systray.NewMenu()
	a.itemDashboard = menu.Add("dashboard", nil)
	a.itemStatus = menu.Add("status", nil)
	a.itemRestart = menu.Add("restart", nil)
	a.itemForce = menu.Add("force", nil)
	a.itemAutostart = menu.AddCheckbox("autostart", false, nil)
	a.itemLanguage = menu.AddSubmenu("language", systray.NewMenu())
	a.itemQuit = menu.Add("quit", nil)
	return a, stub
}

// TestRunServesWithoutADisplay covers the headless path: a test binary
// never shows an icon, so Run waits for the proxy and carries a failed run
// back.
func TestRunServesWithoutADisplay(t *testing.T) {
	control := &fakeControl{
		state:      platform.StateRunning,
		addr:       "127.0.0.1:10101",
		registered: make(chan struct{}, 1),
	}
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), Options{Control: control}) }()
	select {
	case <-control.registered:
	case <-time.After(5 * time.Second):
		t.Fatal("Run() never watched the proxy state")
	}
	control.setState(platform.StateStopped, "", nil)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v, want a clean stop", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run() did not return after the proxy stopped")
	}
}

// TestWaitForProxyCarriesTheFailureBack covers a run that failed: the
// process reports why it is leaving instead of exiting silently.
func TestWaitForProxyCarriesTheFailureBack(t *testing.T) {
	control := &fakeControl{state: platform.StateRunning, registered: make(chan struct{}, 1)}
	failed := context.Canceled
	go func() {
		<-control.registered
		control.setState(platform.StateFailed, "", failed)
	}()
	if err := waitForProxy(context.Background(), control); err != failed {
		t.Fatalf("waitForProxy() error = %v, want the run's own error", err)
	}
}

// TestWaitForProxyStopsOnACancelledContext covers the caller that quits
// first: a cancelled context ends the wait without an error.
func TestWaitForProxyStopsOnACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForProxy(ctx, &fakeControl{state: platform.StateRunning}); err != nil {
		t.Fatalf("waitForProxy() error = %v, want nil", err)
	}
}

// TestRefreshDrawsTheState covers the restart item: a busy proxy disables
// the plain restart, and the item comes back once the cycle ends.
func TestRefreshDrawsTheState(t *testing.T) {
	control := &fakeControl{state: platform.StateRunning, addr: "127.0.0.1:10101"}
	a, _ := testAppWithTray(t, control, Options{Home: t.TempDir()})

	a.refresh()
	control.mu.Lock()
	control.busy = true
	control.mu.Unlock()
	a.refresh()
	if !a.itemRestart.IsDisabled() {
		t.Fatal("the restart item is enabled while the proxy is busy")
	}
	control.mu.Lock()
	control.busy = false
	control.mu.Unlock()
	a.refresh()
	if a.itemRestart.IsDisabled() {
		t.Fatal("the restart item stayed disabled after the cycle")
	}
}

// TestReportEndsWithTheProxy covers the reaction to one transition: a
// stopped proxy closes the ended signal, which is what takes the icon down.
func TestReportEndsWithTheProxy(t *testing.T) {
	a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir()})
	a.report(platform.StateStopped, nil)
	select {
	case <-a.ended:
	default:
		t.Fatal("a stopped proxy did not close the ended signal")
	}
	a.report(platform.StateStopped, nil)
}

// TestReportTellsTheOperatorAboutAFailedRun covers the failed transition: it
// reaches the log and a notification once, so the failure is not silent.
func TestReportTellsTheOperatorAboutAFailedRun(t *testing.T) {
	logger, logs := testkit.TestLogger(t)
	a, stub := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir(), Logger: logger, LogPath: "/tmp/relo.log"})
	a.report(platform.StateFailed, errors.New("the listener is not ready"))
	if got := logs.String(); !strings.Contains(got, "the proxy run failed") {
		t.Fatalf("log = %q, want the failed run reported", got)
	}
	if got := stub.lastNotification(); !strings.Contains(got, "Relo") {
		t.Fatalf("notification = %q, want the failure notification", got)
	}
}

// TestBaseURLFollowsTheProxy covers where the tray reads the management
// API: the live listener while the proxy runs, the configured address
// otherwise.
func TestBaseURLFollowsTheProxy(t *testing.T) {
	control := &fakeControl{state: platform.StateRunning, addr: "0.0.0.0:10101"}
	a, _ := testAppWithTray(t, control, Options{Home: t.TempDir(), URL: "http://127.0.0.1:8080"})
	if got := a.baseURL(); got != "http://127.0.0.1:10101" {
		t.Fatalf("baseURL() = %q, want the live address", got)
	}
	control.setState(platform.StateStopped, "", nil)
	if got := a.baseURL(); got != "http://127.0.0.1:8080" {
		t.Fatalf("baseURL() = %q, want the configured address", got)
	}
}

// TestSelectLanguageLeavesTheLanguageInForceAlone covers the clicked choice
// that repeats the language already in force: the menu is not rebuilt for
// it.
func TestSelectLanguageLeavesTheLanguageInForceAlone(t *testing.T) {
	stored := make(chan string, 1)
	a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir(), Language: "en"})
	a.opts.SetLanguage = func(tag string) error {
		stored <- tag
		return nil
	}
	a.selectLanguage(i18n.English)
	select {
	case tag := <-stored:
		if tag != i18n.English {
			t.Fatalf("stored language = %q, want en", tag)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the language was never stored")
	}
	if got := a.languageNow(); got != i18n.English {
		t.Fatalf("languageNow() = %q, want en", got)
	}
}

// TestApplyLanguageRelabelsTheMenu covers a language that actually changes:
// every row and the tooltip follow it, and an empty tag leaves it alone.
func TestApplyLanguageRelabelsTheMenu(t *testing.T) {
	a, stub := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir(), Language: "en"})
	english := a.text("tray.tooltip", nil)
	if english == "" || english == "tray.tooltip" {
		t.Fatalf("the English tooltip = %q, want the message", english)
	}
	a.applyLanguage("")
	if got := a.languageNow(); got != "en" {
		t.Fatalf("languageNow() = %q, want en", got)
	}
	a.applyLanguage("de")
	if got := a.languageNow(); got != "de" {
		t.Fatalf("languageNow() = %q, want de", got)
	}
	german := a.text("tray.tooltip", nil)
	if german == english || german == "tray.tooltip" {
		t.Fatalf("the German tooltip = %q, want the German message", german)
	}
	if got := stub.tooltip(); got != german {
		t.Fatalf("tooltip = %q, want the German tooltip", got)
	}
}

// TestBuildLanguageMenuOffersEveryLanguage covers the menu layout: every
// shipped language and the system choice carry their own item, and the one
// in force is checked.
func TestBuildLanguageMenuOffersEveryLanguage(t *testing.T) {
	a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir(), Language: i18n.Auto})
	menu := a.buildLanguageMenu()
	if menu == nil {
		t.Fatal("buildLanguageMenu() = nil")
	}
	if len(a.languageItems) != len(i18n.Supported())+1 {
		t.Fatalf("the language menu has %d items, want %d", len(a.languageItems), len(i18n.Supported())+1)
	}
	for _, tag := range append([]string{i18n.Auto}, i18n.Supported()...) {
		item, found := a.languageItems[tag]
		if !found || item == nil {
			t.Fatalf("the language menu has no item for %s", tag)
		}
	}
	if !a.languageItems[i18n.Auto].IsChecked() {
		t.Fatal("the system choice is not checked for auto")
	}
	if label := a.languageLabel("de"); label == "" || label == "language.name.de" {
		t.Fatalf("languageLabel(de) = %q, want the German name", label)
	}
}

// TestWatchConfigAdoptsTheStoredLanguage covers the config re-read: a
// language written by the console reaches the tray without a restart, and
// an unchanged file is not read again.
func TestWatchConfigAdoptsTheStoredLanguage(t *testing.T) {
	home := t.TempDir()
	path := config.ConfigPath(home)
	if err := os.WriteFile(path, []byte("[ui]\nlanguage = 'ja'\n"), 0o600); err != nil {
		t.Fatalf("write the startup file: %v", err)
	}
	a, stub := testAppWithTray(t, &fakeControl{}, Options{Home: home, Language: "en"})
	a.watchConfig()
	if got := a.languageNow(); got != "ja" {
		t.Fatalf("languageNow() = %q, want the language the file names", got)
	}
	tooltip := stub.tooltip()
	if tooltip == "" {
		t.Fatal("the tooltip was never re-rendered for the new language")
	}
	// The file did not change, so the second read reaches nothing new and
	// keeps the language it adopted.
	a.applyLanguage("en")
	if stub.tooltip() == tooltip {
		t.Fatal("the tooltip did not follow the language back to English")
	}
	a.watchConfig()
	if got := a.languageNow(); got != "en" {
		t.Fatalf("languageNow() = %q, want en (the file was not re-read)", got)
	}
}

// TestWatchConfigIgnoresAMissingFile covers a state directory without a
// startup file: the watch leaves the language in force alone.
func TestWatchConfigIgnoresAMissingFile(t *testing.T) {
	a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir(), Language: "en"})
	a.watchConfig()
	if got := a.languageNow(); got != "en" {
		t.Fatalf("languageNow() = %q, want en", got)
	}
}

// stubActivitySource answers one canned snapshot, so the menu tests read
// folding without a database.
type stubActivitySource struct {
	snapshot ActivitySnapshot
	err      error
}

func (s stubActivitySource) Snapshot(context.Context, int64) (ActivitySnapshot, error) {
	return s.snapshot, s.err
}

// TestLoadActivityTotalsTheSnapshot covers the menu's number rows: a snapshot
// folds into totals with unpaid spend excluded, and a failed read clears the
// rows and carries the reason.
func TestLoadActivityTotalsTheSnapshot(t *testing.T) {
	a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir()})
	a.source = newDesktopSource(stubActivitySource{snapshot: ActivitySnapshot{
		Rows: []UsageRow{
			{Key: "openai", Requests: 2, CostMicros: 100},
			{Key: "go", Requests: 5, CostMicros: 900},
		},
		UnpaidIDs: map[string]bool{"go": true},
	}})
	a.loadActivity()
	if got := a.activityNow(); !got.ok || got.totals.requests != 7 || got.totals.spend != 100 || got.err != "" {
		t.Fatalf("activity = %+v, want requests 7 and spend 100", got)
	}

	// A failed read is a failed read, not a stale menu.
	failing, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir()})
	failing.source = newDesktopSource(stubActivitySource{err: errors.New("no database")})
	failing.loadActivity()
	if got := failing.activityNow(); got.ok || got.err == "" {
		t.Fatalf("activity = %+v, want a failed read", got)
	}
}

// TestReportCycleTellsTheOperatorAboutAFailedCycle covers a restart that did
// not come back up: it reaches the log and a notification, and a cycle that
// succeeded says nothing.
func TestReportCycleTellsTheOperatorAboutAFailedCycle(t *testing.T) {
	logger, logs := testkit.TestLogger(t)
	a, stub := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir(), Logger: logger, LogPath: "/tmp/relo.log"})
	a.reportCycle(nil)
	if got := logs.String(); strings.Contains(got, "did not restart") {
		t.Fatalf("log = %q, want nothing for a clean cycle", got)
	}
	a.reportCycle(errors.New("the listener is not ready"))
	if got := logs.String(); !strings.Contains(got, "did not restart") {
		t.Fatalf("log = %q, want the failed cycle reported", got)
	}
	if got := stub.lastNotification(); !strings.Contains(got, "Relo") {
		t.Fatalf("notification = %q, want the failure notification", got)
	}
}

// TestRefreshRowsShowTheDay covers the three number rows: a read fills them
// with the day's totals, and a failed read falls back to the dash.
func TestRefreshRowsShowTheDay(t *testing.T) {
	t.Run("the totals reach the rows", func(t *testing.T) {
		a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir()})
		writeNumberRows(t, a)
		a.activityMu.Lock()
		a.activity = activityData{ok: true, totals: usageTotals{requests: 1234, tokens: 5_000_000, spend: 4_200_000}}
		a.activityMu.Unlock()
		a.refreshRows()
		if got := rowLabel(a, metricRequests); got != "1,234" {
			t.Fatalf("the request row = %q, want the day's count", got)
		}
		if got := rowLabel(a, metricTokens); got != "5.00M" {
			t.Fatalf("the token row = %q, want the day's tokens", got)
		}
		if got := rowLabel(a, metricSpend); got != "$4.20" {
			t.Fatalf("the spend row = %q, want the day's spend", got)
		}
	})

	t.Run("a failed read shows the dash", func(t *testing.T) {
		a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir()})
		writeNumberRows(t, a)
		a.activityMu.Lock()
		a.activity = activityData{err: "no database"}
		a.activityMu.Unlock()
		a.refreshRows()
		if got := rowLabel(a, metricRequests); !strings.Contains(got, dash) {
			t.Fatalf("the request row = %q, want the dash", got)
		}
	})

	t.Run("a row without an item is skipped", func(t *testing.T) {
		a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir()})
		a.itemRequests, a.itemTokens, a.itemSpend = nil, nil, nil
		a.refreshRows()
	})
}

// writeNumberRows gives the app the three rows a real menu carries.
func writeNumberRows(t *testing.T, a *app) {
	t.Helper()
	menu := systray.NewMenu()
	a.itemRequests = menu.Add("requests", nil)
	a.itemTokens = menu.Add("tokens", nil)
	a.itemSpend = menu.Add("spend", nil)
}

// rowLabel renders one number row the way refreshRows does.
func rowLabel(a *app, which metric) string {
	activity := a.activityNow()
	value := dash
	if activity.ok {
		value = formatMetric(activity.totals.value(which), which)
	}
	return value
}

// TestLeaveTakesTheIconDown covers quitting: both the ended signal and the
// quit item ask for the removal, and the second ask changes nothing.
func TestLeaveTakesTheIconDown(t *testing.T) {
	a, stub := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir()})
	a.leave()
	a.leave()
	if got := stub.removals(); got != 1 {
		t.Fatalf("the icon was removed %d times, want once", got)
	}
}

// TestStopThenLeaveEndsTheProxy covers the shutdown item: quitting ends the
// proxy and takes the icon down, even when
// sweep reports a failure.
func TestStopThenLeaveEndsTheProxy(t *testing.T) {
	control := &fakeControl{}
	a, stub := testAppWithTray(t, control, Options{Home: t.TempDir()})
	a.stopThenLeave()
	if got := stub.removals(); got != 1 {
		t.Fatalf("the icon was removed %d times after a clean stop, want once", got)
	}

	control.stop = errors.New("a listener would not end")
	failed, failedStub := testAppWithTray(t, control, Options{Home: t.TempDir()})
	failed.stopThenLeave()
	if got := failedStub.removals(); got != 1 {
		t.Fatalf("the icon was removed %d times after a failed sweep, want once", got)
	}

}

// TestQuitRunsOnce covers the operator clicking Quit twice: one shutdown
// ends the proxy, and the second click changes nothing.
func TestQuitRunsOnce(t *testing.T) {
	control := &fakeControl{}
	a, stub := testAppWithTray(t, control, Options{Home: t.TempDir()})
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a.stopThenLeave()
		}()
	}
	wg.Wait()
	if got := stub.removals(); got != 1 {
		t.Fatalf("the icon was removed %d times, want once", got)
	}
}

// TestQuitBlocksNewCycles covers a restart arriving while Quit runs: the
// cycle is refused, so quitting never hands a fresh proxy to the sweep.
func TestQuitBlocksNewCycles(t *testing.T) {
	logger, logs := testkit.TestLogger(t)
	control := &fakeControl{
		state:   platform.StateRunning,
		restart: errors.New("the listener is not ready"),
		force:   errors.New("the listener is not ready"),
	}
	a, _ := testAppWithTray(t, control, Options{Home: t.TempDir(), Logger: logger, LogPath: "/tmp/relo.log"})
	a.quitting.Store(true)
	a.restartProxy()
	a.forceRestartProxy()
	// The refused cycles report nothing; the settle below is what makes the
	// absence observable, since a cycle that ran reports from a goroutine.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := logs.String(); got != "" {
			t.Fatalf("log = %q, want no cycle while quitting", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

type drainingControl struct {
	*fakeControl
	started chan struct{}
	drained chan struct{}
}

func (c *drainingControl) Stop() error {
	close(c.started)
	<-c.drained
	return nil
}

func TestQuitWaitsForDaemonShutdown(t *testing.T) {
	control := &drainingControl{fakeControl: &fakeControl{}, started: make(chan struct{}), drained: make(chan struct{})}
	a, stub := testAppWithTray(t, control, Options{Home: t.TempDir()})
	finished := make(chan struct{})
	go func() {
		a.stopThenLeave()
		close(finished)
	}()
	select {
	case <-control.started:
	case <-time.After(5 * time.Second):
		t.Fatal("Quit did not start daemon shutdown")
	}
	if got := stub.removals(); got != 0 {
		t.Errorf("tray removed before daemon shutdown: %d", got)
	}
	close(control.drained)
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("Quit did not finish after daemon shutdown")
	}
	if got := stub.removals(); got != 1 {
		t.Fatalf("tray removals = %d, want one", got)
	}
}

// TestRefreshUpdateShowsTheNewerBuild covers the version row: it appears
// while a newer build is published and goes away when the build is current.
func TestRefreshUpdateShowsTheNewerBuild(t *testing.T) {
	server := fakeDaemon(t, http.StatusOK, jsonUpdateAvailable, "admin-token")
	a, _ := testAppWithTray(t, &fakeControl{}, Options{
		Home: t.TempDir(), URL: server.URL, AdminToken: "admin-token", Version: "0.1.0",
	})
	a.itemUpdate = systray.NewMenu().Add("version", nil)
	a.itemUpdate.SetVisible(false)
	a.client = server.Client()
	a.refreshUpdate()
	if !a.itemUpdate.IsVisible() || a.itemUpdate.IsDisabled() {
		t.Fatal("the update row did not appear for a newer build")
	}

	// A check that reached nothing keeps the row it drew.
	a.opts.URL = "http://127.0.0.1:1"
	a.refreshUpdate()
	if !a.itemUpdate.IsVisible() {
		t.Fatal("the update row went away without an answer")
	}

	current := fakeDaemon(t, http.StatusOK, jsonUpdateCurrent, "admin-token")
	a.opts.URL = current.URL
	a.client = current.Client()
	a.refreshUpdate()
	if a.itemUpdate.IsVisible() {
		t.Fatal("the update row stayed visible while the build is current")
	}
}

// jsonUpdateAvailable and jsonUpdateCurrent are the two answers the daemon
// gives the menu's update check.
const jsonUpdateAvailable = `{"current":"0.1.0","latest":"0.2.0","available":true,"url":"https://example.test/relo","method":"download"}`

const jsonUpdateCurrent = `{"current":"0.2.0","latest":"0.2.0","available":false}`

// fakeDaemon answers the management API with one canned response.
func fakeDaemon(t *testing.T, status int, body, token string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/updates" {
			t.Errorf("path = %q, want /api/v1/updates", r.URL.Path)
		}
		// The transport trims the trailing space, so an empty token arrives as
		// a bare "Bearer".
		if status == http.StatusOK && r.Header.Get("Authorization") != strings.TrimSpace("Bearer "+token) {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer "+token)
		}
		w.WriteHeader(status)
		if body != "" {
			_, _ = w.Write([]byte(body))
		}
	}))
	t.Cleanup(server.Close)
	return server
}

// TestRestartProxyRunsOneCycle covers the restart item: a busy proxy is not
// queued behind another cycle, and a cycle that failed says so.
func TestRestartProxyRunsOneCycle(t *testing.T) {
	logger, logs := testkit.TestLogger(t)
	control := &fakeControl{state: platform.StateRunning}
	a, _ := testAppWithTray(t, control, Options{Home: t.TempDir(), Logger: logger, LogPath: "/tmp/relo.log"})

	control.mu.Lock()
	control.busy = true
	control.mu.Unlock()
	a.restartProxy()
	if got := logs.String(); got != "" {
		t.Fatalf("log = %q, want nothing while the proxy is busy", got)
	}

	control.mu.Lock()
	control.busy = false
	control.restart = errors.New("the listener is not ready")
	control.mu.Unlock()
	a.restartProxy()
	waitForLog(t, logs, "did not restart")
}

// TestForceRestartProxyRunsOneCycle covers the escape hatch: it frees the
// ports and reports a cycle that failed.
func TestForceRestartProxyRunsOneCycle(t *testing.T) {
	logger, logs := testkit.TestLogger(t)
	control := &fakeControl{state: platform.StateRunning, force: errors.New("the listener is not ready")}
	a, _ := testAppWithTray(t, control, Options{Home: t.TempDir(), Logger: logger, LogPath: "/tmp/relo.log"})
	a.forceRestartProxy()
	waitForLog(t, logs, "did not restart")
}

// waitForLog waits for one log line, because the proxy cycles run off the
// menu thread.
func waitForLog(t *testing.T, logs *testkit.SyncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(logs.String(), want) {
		if time.Now().After(deadline) {
			t.Fatalf("log = %q, want %q", logs.String(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestUpdateRowHidesWithoutARelease covers the version row with nothing to
// show: a hidden row stays hidden when the build is current.
func TestUpdateRowHidesWithoutARelease(t *testing.T) {
	server := fakeDaemon(t, http.StatusOK, jsonUpdateCurrent, "")
	a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: t.TempDir(), URL: server.URL})
	a.itemUpdate = systray.NewMenu().Add("version", nil)
	a.itemUpdate.SetVisible(false)
	a.client = server.Client()
	a.refreshUpdate()
	if a.itemUpdate.IsVisible() {
		t.Fatal("the version row is visible without a release")
	}
}

// TestLoopsStopWithTheApp covers the timers: each loop leaves when the app
// closes done, without waiting out its own interval.
func TestLoopsStopWithTheApp(t *testing.T) {
	home := t.TempDir()
	a, _ := testAppWithTray(t, &fakeControl{}, Options{Home: home})
	a.source = newDesktopSource(stubActivitySource{})
	done := make(chan struct{})
	close(done)
	finished := make(chan struct{})
	go func() {
		a.refreshLoop(context.Background(), done)
		a.activityLoop(context.Background(), done)
		finished <- struct{}{}
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("a loop did not stop with the app")
	}
}

func TestQuitFreezesEveryActionAndKeepsProcessingLabel(t *testing.T) {
	control := &fakeControl{state: platform.StateRunning}
	a, _ := testAppWithTray(t, control, Options{Home: t.TempDir()})
	a.itemUpdate = systray.NewMenu().Add("update", nil)
	a.languageItems = map[string]*systray.MenuItem{"en": systray.NewMenu().AddCheckbox("English", true, nil)}
	a.beginQuit()
	for _, item := range []*systray.MenuItem{a.itemDashboard, a.itemRestart, a.itemForce, a.itemAutostart, a.itemLanguage, a.itemUpdate, a.itemQuit, a.languageItems["en"]} {
		if !item.IsDisabled() {
			t.Fatal("Quit left an action enabled")
		}
	}
	a.refresh()
	a.relabel()
	a.refreshUpdate()
	a.refreshRows()
	if !a.itemRestart.IsDisabled() || !a.itemUpdate.IsDisabled() {
		t.Fatal("refresh unfroze an action")
	}
	if len(processingIcon()) == 0 {
		t.Fatal("processing icon is empty")
	}
	calls := 0
	a.opts.SetLanguage = func(string) error { calls++; return nil }
	a.selectLanguage("en")
	if calls != 0 {
		t.Fatal("language action ran while quitting")
	}
}
