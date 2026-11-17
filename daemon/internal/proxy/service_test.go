package proxy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// failingStore reports a chosen failure from one method so the pool's own
// refusals can be measured without a real database.
type failingStore struct {
	*memoryStore
	listErr   error
	getErr    error
	createErr error
	updateErr error
	deleteErr error
	countErr  error
}

func (s *failingStore) List(ctx context.Context) ([]Entry, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.memoryStore.List(ctx)
}

func (s *failingStore) Get(ctx context.Context, id string) (Entry, bool, error) {
	if s.getErr != nil {
		return Entry{}, false, s.getErr
	}
	return s.memoryStore.Get(ctx, id)
}

func (s *failingStore) Create(ctx context.Context, entry Entry) (Entry, error) {
	if s.createErr != nil {
		return Entry{}, s.createErr
	}
	return s.memoryStore.Create(ctx, entry)
}

func (s *failingStore) Update(ctx context.Context, entry Entry) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.memoryStore.Update(ctx, entry)
}

func (s *failingStore) Delete(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.memoryStore.Delete(ctx, id)
}

func (s *failingStore) Count(ctx context.Context) (int, error) {
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.memoryStore.Count(ctx)
}

// failingSecrets reports a chosen failure and a mode the test controls.
type failingSecrets struct {
	*memorySecrets
	setErr    error
	getErr    error
	deleteErr error
	mode      string
}

func (s *failingSecrets) Set(ref, value string) error {
	if s.setErr != nil {
		return s.setErr
	}
	return s.memorySecrets.Set(ref, value)
}

func (s *failingSecrets) Get(ref string) (string, error) {
	if s.getErr != nil {
		return "", s.getErr
	}
	return s.memorySecrets.Get(ref)
}

func (s *failingSecrets) Delete(ref string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.memorySecrets.Delete(ref)
}

func (s *failingSecrets) Mode() string {
	if s.mode != "" {
		return s.mode
	}
	return s.memorySecrets.Mode()
}

func newPool(t *testing.T, store Store, secrets SecretStore) *Service {
	t.Helper()
	return New(Options{
		Store: store, Secrets: secrets, Clock: &testClock{now: time.Unix(1000, 0)},
		Backoff: func() []time.Duration { return []time.Duration{time.Minute, 2 * time.Minute} },
		Check:   func(context.Context, string) error { return nil },
	})
}

func mustCreate(t *testing.T, service *Service, draft Draft) Item {
	t.Helper()
	item, err := service.Create(context.Background(), draft)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return item
}

// TestPageBoundsTheWindow covers the page rules the API relies on: the default
// and maximum window, the cursor, and the empty page past the end.
func TestPageBoundsTheWindow(t *testing.T) {
	service, _, _, _ := testService(t)
	for port := 7000; port < 7003; port++ {
		mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: port, Enabled: true})
	}
	if items, next, err := service.Page(context.Background(), Query{}); err != nil || len(items) != 3 || next != 0 {
		t.Fatalf("default page = %d items, next %d, %v", len(items), next, err)
	}
	items, next, err := service.Page(context.Background(), Query{Offset: 1, Limit: 1})
	if err != nil || len(items) != 1 || next != 2 {
		t.Fatalf("windowed page = %d items, next %d, %v", len(items), next, err)
	}
	if items, next, err := service.Page(context.Background(), Query{Offset: 9}); err != nil || len(items) != 0 || next != 0 {
		t.Fatalf("past the end = %d items, next %d, %v", len(items), next, err)
	}
	// A negative offset is the first page, and a limit over the cap is the cap.
	if items, _, err := service.Page(context.Background(), Query{Offset: -5, Limit: 500}); err != nil || len(items) != 3 {
		t.Fatalf("clamped page = %d items, %v", len(items), err)
	}
}

// TestPageFiltersBySearchAndState covers the three list filters the console
// sends: a search across label, host, username, and id, and one state word.
func TestPageFiltersBySearchAndState(t *testing.T) {
	service, _, _, _ := testService(t)
	kept := mustCreate(t, service, Draft{Label: "Edge One", Scheme: "http", Host: "127.0.0.1", Port: 7000, Username: "alice", Enabled: true})
	mustCreate(t, service, Draft{Scheme: "http", Host: "10.0.0.2", Port: 7001, Enabled: false})
	service.Cool(kept.ID, time.Minute)

	byLabel, _, err := service.Page(context.Background(), Query{Search: "edge"})
	if err != nil || len(byLabel) != 1 || byLabel[0].ID != kept.ID {
		t.Fatalf("search by label = %+v, %v", byLabel, err)
	}
	byUsername, _, err := service.Page(context.Background(), Query{Search: "ALICE"})
	if err != nil || len(byUsername) != 1 {
		t.Fatalf("search by username = %+v, %v", byUsername, err)
	}
	byID, _, err := service.Page(context.Background(), Query{Search: kept.ID})
	if err != nil || len(byID) != 1 {
		t.Fatalf("search by id = %+v, %v", byID, err)
	}
	byHost, _, err := service.Page(context.Background(), Query{Search: "10.0.0"})
	if err != nil || len(byHost) != 1 {
		t.Fatalf("search by host = %+v, %v", byHost, err)
	}
	cooling, _, err := service.Page(context.Background(), Query{Status: "cooling"})
	if err != nil || len(cooling) != 1 || cooling[0].State != StateCooling || cooling[0].CoolingUntil == nil {
		t.Fatalf("cooling filter = %+v, %v", cooling, err)
	}
	enabled, _, err := service.Page(context.Background(), Query{Status: "enabled"})
	if err != nil || len(enabled) != 1 {
		t.Fatalf("enabled filter = %+v, %v", enabled, err)
	}
	disabled, _, err := service.Page(context.Background(), Query{Status: "disabled"})
	if err != nil || len(disabled) != 1 {
		t.Fatalf("disabled filter = %+v, %v", disabled, err)
	}
}

// TestSummaryCountsThePool covers the providers card's counts, including the
// all-cooling case the card names.
func TestSummaryCountsThePool(t *testing.T) {
	service, _, _, _ := testService(t)
	one := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true})
	two := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7001, Enabled: true})
	mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7002, Enabled: false})

	summary, err := service.Summary(context.Background())
	if err != nil || summary.Total != 3 || summary.Enabled != 2 || summary.Cooling != 0 || summary.AllCooling {
		t.Fatalf("Summary() = %+v, %v", summary, err)
	}
	service.Cool(one.ID, time.Minute)
	service.Cool(two.ID, time.Minute)
	summary, err = service.Summary(context.Background())
	if err != nil || summary.Cooling != 2 || !summary.AllCooling {
		t.Fatalf("all cooling Summary() = %+v, %v", summary, err)
	}
}

// TestHealthMarksDriveTheReportedState covers the three health writes and the
// state each one makes the list report.
func TestHealthMarksDriveTheReportedState(t *testing.T) {
	service, _, _, _ := testService(t)
	item := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true})

	service.MarkUnreachable(item.ID, errors.New("connection refused"))
	refreshed, err := service.Test(context.Background(), item.ID)
	if err != nil || refreshed.State != StateReady {
		t.Fatalf("Test() after a passing check = %+v, %v", refreshed, err)
	}

	failure := errors.New("weird proxy")
	service, _, _, clk := testService(t)
	item = mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true})
	service.check = func(context.Context, string) error { return failure }
	refreshed, err = service.Test(context.Background(), item.ID)
	if err != nil || refreshed.State != StateUnreachable {
		t.Fatalf("Test() after a failing check = %+v, %v", refreshed, err)
	}

	// An empty id names no proxy, so every mark is a no-op.
	service.MarkServed("")
	service.MarkUnreachable("", failure)
	service.Cool("", time.Minute)

	// A state recorded as cooling reads as unverified once the cooldown ends.
	service.Cool(item.ID, time.Minute)
	clk.now = clk.now.Add(2 * time.Minute)
	items, err := service.Items(context.Background())
	if err != nil || items[0].State != StateUnverified || items[0].CoolingUntil != nil {
		t.Fatalf("expired cooldown = %+v, %v", items[0], err)
	}
}

// TestTestReportsItsOwnRefusals covers the two ways a test cannot run: a proxy
// that cannot be read and a pool with no check configured.
func TestTestReportsItsOwnRefusals(t *testing.T) {
	service, _, _, _ := testService(t)
	if _, err := service.Test(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Test(missing) error = %v, want ErrNotFound", err)
	}
	item := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true})
	service.check = nil
	if _, err := service.Test(context.Background(), item.ID); err == nil {
		t.Fatal("Test() without a check error = nil, want a refusal")
	}
}

// TestFirstURLPrefersAHealthyProxy covers the discovery path, which has no
// request-local rotation and falls back to no proxy rather than failing.
func TestFirstURLPrefersAHealthyProxy(t *testing.T) {
	service, _, _, _ := testService(t)
	if got := service.FirstURL(context.Background()); got != "" {
		t.Fatalf("FirstURL(empty pool) = %q", got)
	}
	item := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true})
	if got := service.FirstURL(context.Background()); !strings.Contains(got, "127.0.0.1:7000") {
		t.Fatalf("FirstURL() = %q", got)
	}
	// A proxy whose secret cannot be read reports no proxy at all.
	unreadable := newPool(t, &memoryStore{rows: map[string]Entry{
		item.ID: {ID: item.ID, Scheme: "http", Host: "127.0.0.1", Port: 7000, SecretRef: "proxy/" + item.ID, Enabled: true},
	}}, &failingSecrets{memorySecrets: newMemorySecrets(), getErr: errors.New("vault is locked")})
	if got := unreadable.FirstURL(context.Background()); got != "" {
		t.Fatalf("FirstURL() with an unreadable secret = %q", got)
	}
}

// TestURLResolvesCachesAndRejects covers the read path: the cache, the missing
// row, a secret store that refuses, and an entry with no credentials.
func TestURLResolvesCachesAndRejects(t *testing.T) {
	store := newMemoryStore()
	vault := &failingSecrets{memorySecrets: newMemorySecrets()}
	service := newPool(t, store, vault)
	item := mustCreate(t, service, Draft{Scheme: "socks5", Host: "127.0.0.1", Port: 1080, Username: "bob", Password: "pw", Enabled: true})

	first, err := service.URL(context.Background(), item.ID)
	if err != nil || !strings.Contains(first, "bob:pw@") {
		t.Fatalf("URL() = %q, %v", first, err)
	}
	// The cache answers after the vault stops answering.
	vault.getErr = errors.New("vault is locked")
	if again, err := service.URL(context.Background(), item.ID); err != nil || again != first {
		t.Fatalf("cached URL() = %q, %v", again, err)
	}
	service.ClearCache()
	if _, err := service.URL(context.Background(), item.ID); err == nil {
		t.Fatal("URL() after ClearCache() error = nil, want the vault refusal")
	}
	if _, err := service.URL(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("URL(missing) error = %v, want ErrNotFound", err)
	}

	// An entry with no secret reference needs no vault at all.
	bare := newPool(t, store, nil)
	plain := mustCreate(t, bare, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7009, Enabled: true})
	resolved, err := bare.URL(context.Background(), plain.ID)
	if err != nil || resolved != "http://127.0.0.1:7009" {
		t.Fatalf("URL() of a credential-free entry = %q, %v", resolved, err)
	}
	// A stored reference without a vault is the pool's own refusal.
	orphan := Entry{ID: "orphan", Scheme: "http", Host: "127.0.0.1", Port: 7010, SecretRef: "proxy/orphan"}
	if _, err := bare.urlOf(orphan); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("urlOf(orphan) error = %v, want ErrNoSecret", err)
	}
}

// TestSelectRefusesWhenNothingCanBeUsed covers the two selection refusals: a
// pool with no enabled proxy, and one whose only proxy cannot be read.
func TestSelectRefusesWhenNothingCanBeUsed(t *testing.T) {
	service, _, _, _ := testService(t)
	mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: false})
	if _, err := service.Select(context.Background(), nil); !errors.Is(err, ErrNoEnabled) {
		t.Fatalf("Select() error = %v, want ErrNoEnabled", err)
	}

	unreadable := newPool(t, &memoryStore{rows: map[string]Entry{}}, &failingSecrets{memorySecrets: newMemorySecrets()})
	entry := mustCreate(t, unreadable, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7001, Password: "pw", Enabled: true})
	unreadable.secrets.(*failingSecrets).getErr = errors.New("vault is locked")
	var cooling *AllCoolingError
	if _, err := unreadable.Select(context.Background(), nil); !errors.As(err, &cooling) {
		t.Fatalf("Select() with an unreadable secret error = %v, want AllCoolingError", err)
	}
	// Cooling until a known time waits exactly that long.
	unreadable.Cool(entry.ID, 90*time.Second)
	if _, err := unreadable.Select(context.Background(), nil); !errors.As(err, &cooling) || cooling.RetryAfter != 90*time.Second {
		t.Fatalf("Select() while cooling error = %v, want a 90s wait", err)
	}
	if !strings.Contains(cooling.Error(), ErrAllCooling.Error()) || !errors.Is(cooling, ErrAllCooling) {
		t.Fatalf("AllCoolingError = %q, want it to unwrap to %q", cooling, ErrAllCooling)
	}
}

// TestSelectStickySkipsExcludedAndCooling covers the rotation: an excluded id
// is not offered again, and a cooling proxy is not offered while it cools.
func TestSelectStickySkipsExcludedAndCooling(t *testing.T) {
	service, _, _, _ := testService(t)
	first := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true, Rank: 0})
	second := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7001, Enabled: true, Rank: 1})
	selection, err := service.SelectSticky(context.Background(), "", []string{first.ID})
	if err != nil || selection.ID != second.ID {
		t.Fatalf("SelectSticky(excluding first) = %+v, %v", selection, err)
	}
	service.Cool(second.ID, time.Minute)
	selection, err = service.Select(context.Background(), []string{first.ID})
	var cooling *AllCoolingError
	if !errors.As(err, &cooling) {
		t.Fatalf("Select() with both unusable = %+v, %v", selection, err)
	}
}

// TestCoolWalksTheLadder covers the failover ladder: each failure moves one
// step, a provider's longer Retry-After wins, and the wait never goes negative.
func TestCoolWalksTheLadder(t *testing.T) {
	service, _, _, _ := testService(t)
	item := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true})
	if wait := service.Cool(item.ID, 0); wait != time.Minute {
		t.Fatalf("first Cool() = %v, want 1m", wait)
	}
	if wait := service.Cool(item.ID, 0); wait != 2*time.Minute {
		t.Fatalf("second Cool() = %v, want 2m", wait)
	}
	// Past the end of the ladder the last step repeats.
	if wait := service.Cool(item.ID, 0); wait != 2*time.Minute {
		t.Fatalf("third Cool() = %v, want the last step", wait)
	}
	if wait := service.Cool(item.ID, 10*time.Minute); wait != 10*time.Minute {
		t.Fatalf("Cool() with a longer Retry-After = %v, want 10m", wait)
	}
	if wait := service.Cool(item.ID, -time.Hour); wait != 2*time.Minute {
		t.Fatalf("Cool() with a negative Retry-After = %v, want the ladder", wait)
	}
	// Without a ladder a negative Retry-After is no wait rather than a past time.
	ladderless := New(Options{Store: newMemoryStore(), Secrets: newMemorySecrets(), Clock: &testClock{now: time.Unix(1000, 0)}})
	other := mustCreate(t, ladderless, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7001, Enabled: true})
	if wait := ladderless.Cool(other.ID, -time.Hour); wait != 0 {
		t.Fatalf("Cool() without a ladder = %v, want 0", wait)
	}
}

// TestCreateRefusesAndCleansUp covers the write path's refusals and the vault
// cleanup an entry that cannot be stored owes.
func TestCreateRefusesAndCleansUp(t *testing.T) {
	ctx := context.Background()
	service, _, vault, _ := testService(t)
	mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true})
	if _, err := service.Create(ctx, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Enabled: true}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Create(duplicate) error = %v, want ErrDuplicate", err)
	}
	// An address that no normalizer accepts is refused before any write.
	if _, err := service.Create(ctx, Draft{Host: "", Port: 7000}); err == nil {
		t.Fatal("Create(no host) error = nil, want a refusal")
	}
	if _, err := service.Create(ctx, Draft{Scheme: "http", Host: "127.0.0.1", Port: 0}); err == nil {
		t.Fatal("Create(no port) error = nil, want a refusal")
	}

	// A password with no vault is refused rather than stored in the clear.
	unbacked := newPool(t, newMemoryStore(), nil)
	if _, err := unbacked.Create(ctx, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7001, Password: "pw"}); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("Create() without a vault error = %v, want ErrNoSecret", err)
	}

	// A store that refuses the row leaves no orphaned secret behind.
	broken := &failingStore{memoryStore: newMemoryStore(), createErr: errors.New("disk is full")}
	orphanVault := &failingSecrets{memorySecrets: newMemorySecrets()}
	brokenService := newPool(t, broken, orphanVault)
	if _, err := brokenService.Create(ctx, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7002, Password: "pw"}); err == nil {
		t.Fatal("Create() with a failing store error = nil, want the refusal")
	}
	if len(orphanVault.values) != 0 {
		t.Fatalf("vault kept %d orphaned secrets", len(orphanVault.values))
	}

	// A vault that refuses the secret never reaches the store.
	refusing := &failingSecrets{memorySecrets: newMemorySecrets(), setErr: errors.New("vault is locked")}
	refusingService := newPool(t, newMemoryStore(), refusing)
	if _, err := refusingService.Create(ctx, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7003, Password: "pw"}); err == nil {
		t.Fatal("Create() with a failing vault error = nil, want the refusal")
	}

	// A duplicate is found by the store listing, so its failure is reported.
	unlistable := newPool(t, &failingStore{memoryStore: newMemoryStore(), listErr: errors.New("db is closed")}, vault)
	if _, err := unlistable.Create(ctx, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7004}); err == nil {
		t.Fatal("Create() with an unlistable store error = nil, want the refusal")
	}
}

// TestBuildEntryNormalizesTheDraft covers the shape the store keeps: the
// default scheme, the label the console shows, and the username in the URL.
func TestBuildEntryNormalizesTheDraft(t *testing.T) {
	service, _, _, _ := testService(t)
	entry, err := service.buildEntry(Draft{Host: "127.0.0.1", Port: 7890, Rank: -3})
	if err != nil || entry.Scheme != "http" || entry.Label != "127.0.0.1:7890" || entry.Rank != 0 || entry.Host != "127.0.0.1" {
		t.Fatalf("buildEntry() = %+v, %v", entry, err)
	}
	entry, err = service.buildEntry(Draft{Scheme: " SOCKS5 ", Host: "proxy.internal", Port: 1080, Username: "u", Password: "p", Label: " Named "})
	if err != nil || entry.Scheme != "socks5" || entry.Username != "u" || entry.Label != "Named" {
		t.Fatalf("buildEntry() with credentials = %+v, %v", entry, err)
	}
	if _, err := service.buildEntry(Draft{Scheme: "ftp", Host: "127.0.0.1", Port: 21}); err == nil {
		t.Fatal("buildEntry(ftp) error = nil, want a refusal")
	}
}

// TestUpdateAppliesOnlyTheNamedFields covers the patch: an empty password keeps
// the stored one, every named field is normalized, and the refusals hold.
func TestUpdateAppliesOnlyTheNamedFields(t *testing.T) {
	ctx := context.Background()
	service, store, vault, _ := testService(t)
	item := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Username: "alice", Password: "pw", Enabled: true})

	label, scheme, host, port, username, enabled, rank := "", " SOCKS5 ", " proxy.internal ", 1080, " bob ", false, -2
	empty := ""
	updated, err := service.Update(ctx, item.ID, Patch{
		Label: &label, Scheme: &scheme, Host: &host, Port: &port,
		Username: &username, Enabled: &enabled, Rank: &rank, Password: &empty,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	// The label is rebuilt from the row as the patch found it, so clearing the
	// label and moving the host in one patch leaves the old address as the name.
	if updated.Label != "127.0.0.1:7000" || updated.Scheme != "socks5" || updated.Username != "bob" ||
		updated.Host != "proxy.internal" || updated.Port != 1080 || updated.Rank != 0 || updated.Enabled {
		t.Fatalf("Update() = %+v", updated)
	}
	// An empty password keeps the stored one.
	if vault.values["proxy/"+item.ID] != "pw" {
		t.Fatalf("vault = %#v, want the stored password kept", vault.values)
	}

	if _, err := service.Update(ctx, "missing", Patch{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update(missing) error = %v, want ErrNotFound", err)
	}
	// Moving one row onto another row's address is refused as a duplicate.
	clashHost, clashPort, emptyName := "10.0.0.9", 9000, ""
	clashScheme := "http"
	mustCreate(t, service, Draft{Scheme: "http", Host: clashHost, Port: clashPort, Enabled: true})
	moved := mustCreate(t, service, Draft{Scheme: "http", Host: "10.0.0.10", Port: 9010, Username: "carol", Enabled: true})
	if _, err := service.Update(ctx, moved.ID, Patch{
		Scheme: &clashScheme, Host: &clashHost, Port: &clashPort, Username: &emptyName,
	}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("Update(clashing) error = %v, want ErrDuplicate", err)
	}
	bad := "ftp"
	if _, err := service.Update(ctx, item.ID, Patch{Scheme: &bad}); err == nil {
		t.Fatal("Update(ftp) error = nil, want a refusal")
	}

	// A password change with no vault is refused, and one with a vault is stored.
	unbacked := newPool(t, store, nil)
	if _, err := unbacked.Update(ctx, item.ID, Patch{Password: &[]string{"new"}[0]}); !errors.Is(err, ErrNoSecret) {
		t.Fatalf("Update() without a vault error = %v, want ErrNoSecret", err)
	}
	locked := "new"
	refusing := &failingSecrets{memorySecrets: newMemorySecrets(), setErr: errors.New("vault is locked")}
	refusingService := newPool(t, store, refusing)
	if _, err := refusingService.Update(ctx, item.ID, Patch{Password: &locked}); err == nil {
		t.Fatal("Update() with a failing vault error = nil, want the refusal")
	}

	// A store that refuses the write reports it.
	unwritable := newPool(t, &failingStore{memoryStore: store, updateErr: errors.New("disk is full")}, vault)
	if _, err := unwritable.Update(ctx, item.ID, Patch{Password: &locked}); err == nil {
		t.Fatal("Update() with a failing store error = nil, want the refusal")
	}
	unreadable := newPool(t, &failingStore{memoryStore: store, getErr: errors.New("db is closed")}, vault)
	if _, err := unreadable.Update(ctx, item.ID, Patch{}); err == nil {
		t.Fatal("Update() with an unreadable store error = nil, want the refusal")
	}
}

// TestDeleteReportsItsRefusals covers the three ways a delete can fail, and
// that a delete with no vault still removes the row.
func TestDeleteReportsItsRefusals(t *testing.T) {
	ctx := context.Background()
	service, store, _, _ := testService(t)
	item := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Password: "pw", Enabled: true})
	service.Cool(item.ID, time.Minute)

	if err := service.Delete(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) error = %v, want ErrNotFound", err)
	}
	broken := newPool(t, &failingStore{memoryStore: store, deleteErr: errors.New("disk is full")}, newMemorySecrets())
	if err := broken.Delete(ctx, item.ID); err == nil {
		t.Fatal("Delete() with a failing store error = nil, want the refusal")
	}
	unreadable := newPool(t, &failingStore{memoryStore: store, getErr: errors.New("db is closed")}, newMemorySecrets())
	if err := unreadable.Delete(ctx, item.ID); err == nil {
		t.Fatal("Delete() with an unreadable store error = nil, want the refusal")
	}
	if err := service.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	// No vault is needed once the entry carries no secret of its own.
	plain := newPool(t, store, nil)
	bare := mustCreate(t, plain, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7001})
	if err := plain.Delete(ctx, bare.ID); err != nil {
		t.Fatalf("Delete() without a vault error = %v", err)
	}
}

// TestDraftParsing covers the shapes a pasted proxy line takes, the refusals
// that keep a bad line out of the pool, and the editable draft of a row.
func TestDraftParsing(t *testing.T) {
	for _, raw := range []string{
		"http://127.0.0.1:7890",
		"socks5h://user:pass@127.0.0.1:1080",
		"127.0.0.1:7890",
		"127.0.0.1:7890:user:pass",
		"user:pass@127.0.0.1:7890",
	} {
		if _, err := DraftFromURL(raw); err != nil {
			t.Fatalf("DraftFromURL(%q) error = %v", raw, err)
		}
	}
	// socks5h is stored as socks5 and the shorthand keeps its credentials.
	draft, err := DraftFromURL("socks5h://user:pass@127.0.0.1:1080")
	if err != nil || draft.Scheme != "socks5" || draft.Username != "user" || draft.Password != "pass" || !draft.Enabled {
		t.Fatalf("DraftFromURL(socks5h) = %+v, %v", draft, err)
	}
	for _, raw := range []string{"", "not a proxy", "http://127.0.0.1", "ftp://127.0.0.1:21", "http://127.0.0.1:0"} {
		if _, err := DraftFromURL(raw); !errors.Is(err, ErrInvalid) {
			t.Fatalf("DraftFromURL(%q) error = %v, want ErrInvalid", raw, err)
		}
	}
	editable := DraftFromEntry(Entry{Label: "L", Scheme: "http", Host: "h", Port: 1, Username: "u", Enabled: true, Rank: 4})
	if editable.Password != "" || editable.Rank != 4 || !editable.Enabled {
		t.Fatalf("DraftFromEntry() = %+v", editable)
	}
}

// TestBackfillRefusals covers the one-time move of a legacy URL: nothing to
// move, an unreachable store, a pool that already has entries, a bad URL, a
// failed write, and a clear callback that refuses.
func TestBackfillRefusals(t *testing.T) {
	ctx := context.Background()
	service, store, _, _ := testService(t)
	if moved, err := service.Backfill(ctx, "   ", nil); err != nil || moved {
		t.Fatalf("Backfill(empty) = %v, %v", moved, err)
	}
	// A nil clear callback is the "nothing to clear" case, not a refusal.
	if moved, err := service.Backfill(ctx, "http://127.0.0.1:7890", nil); err != nil || !moved {
		t.Fatalf("Backfill(nil clear) = %v, %v", moved, err)
	}
	mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000})
	if moved, err := service.Backfill(ctx, "http://127.0.0.1:7890", nil); err != nil || moved {
		t.Fatalf("Backfill(already populated) = %v, %v", moved, err)
	}

	fresh := newPool(t, newMemoryStore(), newMemorySecrets())
	if moved, err := fresh.Backfill(ctx, "not a proxy", nil); err == nil || moved {
		t.Fatalf("Backfill(bad URL) = %v, %v, want a refusal", moved, err)
	}
	if moved, err := fresh.Backfill(ctx, "http://127.0.0.1:7890", func() error { return errors.New("clear failed") }); !moved || err == nil {
		t.Fatalf("Backfill(failing clear) = %v, %v, want the entry kept and the error returned", moved, err)
	}

	if _, err := newPool(t, &failingStore{memoryStore: store, countErr: errors.New("db is closed")}, newMemorySecrets()).
		Backfill(ctx, "http://127.0.0.1:7890", nil); err == nil {
		t.Fatal("Backfill() with an uncountable store error = nil, want the refusal")
	}
	if _, err := newPool(t, &failingStore{memoryStore: newMemoryStore(), createErr: errors.New("disk is full")}, newMemorySecrets()).
		Backfill(ctx, "http://127.0.0.1:7890", nil); err == nil {
		t.Fatal("Backfill() with a failing store error = nil, want the refusal")
	}
}

// TestClearCacheFollowsTheVaultMode covers the cache a vault-mode change has to
// invalidate: the mode string is what a rotated key changes.
func TestClearCacheFollowsTheVaultMode(t *testing.T) {
	store := newMemoryStore()
	vault := &failingSecrets{memorySecrets: newMemorySecrets(), mode: "encrypted_file"}
	service := newPool(t, store, vault)
	item := mustCreate(t, service, Draft{Scheme: "http", Host: "127.0.0.1", Port: 7000, Password: "pw"})
	if _, err := service.URL(context.Background(), item.ID); err != nil {
		t.Fatalf("URL() error = %v", err)
	}
	// A mode change invalidates the cached URL, which then has to be re-read.
	vault.mode = "memory"
	if _, err := service.URL(context.Background(), item.ID); err != nil {
		t.Fatalf("URL() after a mode change error = %v", err)
	}
	if service.secretMode != "memory" {
		t.Fatalf("secretMode = %q, want the new mode", service.secretMode)
	}
	// No vault at all means nothing to follow.
	vaultless := newPool(t, store, nil)
	vaultless.ClearCache()
	if vaultless.secretMode != "" {
		t.Fatalf("vaultless secretMode = %q", vaultless.secretMode)
	}
}
