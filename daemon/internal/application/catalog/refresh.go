// Catalog refresh: re-reading rosters and pricing.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
)

// EnsureProvider creates the row a template needs and returns the row that
// already carries that id. signInOnly refuses a template Relo cannot log into,
// which is what a login path needs: a login only writes a credential, so the
// provider it belongs to has to exist before a browser opens.
func (s *Service) EnsureProvider(ctx context.Context, templateID string, signInOnly bool) (Provider, error) {
	if existing, err := s.Provider(ctx, templateID); err == nil {
		return existing, nil
	}

	mdIdx, _ := s.modelsDev.Get(ctx, catalog.FetchOnlineFirst)
	t, found := s.templates.Get(templateID, mdIdx)
	if !found || t.UnsupportedReason != "" {
		return Provider{}, ErrProviderNotFound
	}
	if signInOnly && len(t.LoginFlows) == 0 {
		return Provider{}, ErrProviderNotFound
	}

	id := s.uniqueProviderID(ctx, templateID)
	if err := s.store.SaveProvider(ctx, newProviderRow(t, id)); err != nil {
		return Provider{}, err
	}
	if err := s.reload(ctx); err != nil {
		return Provider{}, err
	}
	return s.Provider(ctx, id)
}

func newProviderRow(t catalog.Template, id string) catalog.ConnectionRecord {
	now := catalog.NowMs()
	row := catalog.ConnectionRecord{
		ID:                  id,
		TemplateID:          t.ID,
		Origin:              string(t.Origin),
		Label:               t.Label,
		Auth:                string(t.Auth),
		APIFormat:           string(t.DefaultFormat),
		KeyHeader:           string(t.KeyHeader),
		BaseURL:             t.DefaultBaseURL,
		ModelsSource:        t.ModelsSource,
		ModelsFormat:        string(t.ModelsFormat),
		ModelsDevProviderID: t.ModelsDevProviderID,
		Headers:             t.Headers,
		DocURL:              t.DocURL,
		KeyEnv:              t.KeyEnv,
		LoginFlows:          t.LoginFlows,
		Enabled:             true,
		Rank:                100,
		PoolStrategy:        "least-loaded",
		CreatedAtMs:         now,
		UpdatedAtMs:         now,
	}
	if row.ModelsDevProviderID == "" {
		row.ModelsDevProviderID = t.ID
	}
	return row
}

// The statuses one connection's refresh reports. An updated connection had its
// roster read from the provider, a skipped one publishes no model list to
// read, and a failed one is the vendor's answer to keep.
const (
	RefreshUpdated = "updated"
	RefreshSkipped = "skipped"
	RefreshFailed  = "failed"
)

// RefreshResult is what one connection's roster re-read changed: how many
// models it lists now and how many arrived, left, or gained prices.
type RefreshResult struct {
	Status      string
	Listed      int
	Added       int
	Unavailable int
	Matched     int
	Priced      int
	// Accounts is how many accounts answered with a roster of their own, which
	// is only ever more than zero on a connection whose list is per account.
	Accounts int
	// AccountFailures names the accounts whose own roster could not be read, so
	// a partly read connection is reported rather than passed off as whole.
	AccountFailures []string
	ModelsDev       catalog.ModelsDevState
}

// RefreshProviderModels reads one connection's own model list again and
// commits the roster it publishes. The catalog is reloaded once the attempt is
// over either way, because a listing that failed still records why.
func (s *Service) RefreshProviderModels(ctx context.Context, providerID string) (RefreshResult, error) {
	defer func() { _ = s.reload(ctx) }()
	return s.refreshProvider(ctx, providerID)
}

func (s *Service) refreshProvider(ctx context.Context, providerID string) (RefreshResult, error) {
	atMs := catalog.NowMs()
	host, t, mdIdx, mdState, existing, err := s.openRefreshHost(ctx, providerID)
	if err != nil {
		return RefreshResult{}, err
	}

	// Only a connection that declares a listing dialect is ever asked for its
	// models.
	canList := host.ModelsFormat != catalog.ModelsNone && s.discover != nil && host.BaseURL != ""
	if !canList {
		// A connection that publishes no model list has nothing to verify: its
		// models are the ids an operator typed, and reading a catalog is not
		// evidence that this connection serves any of them.
		return RefreshResult{Status: RefreshSkipped, ModelsDev: mdState}, nil
	}

	listed, accountRosters, accountFailures, err := s.fetchRefreshRoster(ctx, host)
	if err != nil {
		return s.recordRefreshFailure(ctx, providerID, atMs, mdState, err)
	}
	s.keepAccountRosters(ctx, accountRosters)

	existingMap := indexExistingModels(existing)
	assembly := assembleRefreshRoster(t, host, listed, mdIdx, providerID, existingMap)

	return s.finishRefresh(ctx, providerID, refreshFinish{listed: listed, assembly: assembly, existing: existing, accountRosters: accountRosters, accountFailures: accountFailures, atMs: atMs, mdState: mdState})
}

func indexExistingModels(existing []catalog.Model) map[string]catalog.Model {
	indexed := make(map[string]catalog.Model, len(existing))
	for _, m := range existing {
		indexed[m.ID] = m
	}
	return indexed
}

func assembleRefreshRoster(t catalog.Template, host catalog.Provider, listed []catalog.Listed, mdIdx *catalog.ModelsDevIndex, providerID string, existingMap map[string]catalog.Model) refreshAssembly {
	modelsDevProviderID := host.ModelsDevProviderID
	if modelsDevProviderID == "" {
		modelsDevProviderID = host.ID
	}
	roster := buildRoster(t, rosterModeFor(t), listed, nil)
	return assembleRefreshModels(roster, mdIdx, providerID, modelsDevProviderID, existingMap)
}

func (s *Service) openRefreshHost(ctx context.Context, providerID string) (catalog.Provider, catalog.Template, *catalog.ModelsDevIndex, catalog.ModelsDevState, []catalog.Model, error) {
	snap, err := s.snapshot()
	if err != nil {
		return catalog.Provider{}, catalog.Template{}, nil, catalog.ModelsDevState{}, nil, err
	}
	host, found := snap.Provider(providerID)
	if !found {
		return catalog.Provider{}, catalog.Template{}, nil, catalog.ModelsDevState{}, nil, ErrProviderNotFound
	}

	mdIdx, _ := s.modelsDev.Get(ctx, catalog.FetchOnlineFirst)
	mdState := s.modelsDev.CurrentState()

	// The stored row keeps connection details, not the catalog policy the
	// template declares, so the roster mode and the listing dialect come from
	// the template this row was added from. A row whose template is gone keeps
	// what it was stored with.
	t, found := s.templates.Get(host.TemplateID, mdIdx)
	if found {
		host.ModelsSource = t.ModelsSource
		host.ModelsFormat = t.ModelsFormat
	}
	return host, t, mdIdx, mdState, snap.Models(providerID), nil
}

func (s *Service) fetchRefreshRoster(ctx context.Context, host catalog.Provider) ([]catalog.Listed, map[string][]string, []string, error) {
	accounts := s.activeAccounts(host.ID)
	if catalog.PerAccountRoster(host.ModelsFormat) && len(accounts) > 0 {
		// An account's own entitlement can be narrower than the connection's,
		// so every account is asked and the union is what the connection
		// lists. What each account answered is remembered against it.
		return s.discoverAccounts(ctx, host, accounts)
	}
	listed, err := s.DiscoverModels(ctx, host)
	return listed, nil, nil, err
}

func (s *Service) recordRefreshFailure(ctx context.Context, providerID string, atMs int64, mdState catalog.ModelsDevState, err error) (RefreshResult, error) {
	// A listing that failed says nothing about which models the provider
	// serves, so the roster the row already has stays as it is.
	if recordErr := s.store.RecordRefresh(ctx, providerID, atMs, err.Error()); recordErr != nil {
		return RefreshResult{}, recordErr
	}
	// A refused credential is the operator's to correct and any other
	// failure is the provider's, so the console can say which it was.
	if errors.Is(err, catalog.ErrListRejected) {
		return RefreshResult{Status: RefreshFailed, ModelsDev: mdState}, fmt.Errorf("%s: %w", err, ErrCredentialRejected)
	}
	return RefreshResult{Status: RefreshFailed, ModelsDev: mdState}, fmt.Errorf("%s: %w", err, ErrListingFailed)
}

func (s *Service) keepAccountRosters(ctx context.Context, accountRosters map[string][]string) {
	// A roster that was read is stored whether or not the rest of the refresh
	// succeeds: it is what the next request filters accounts by.
	for credentialID, modelIDs := range accountRosters {
		if err := s.pools.SetRoster(ctx, credentialID, modelIDs, account.ModelSourceListing); err != nil {
			s.logger.Warn("store an account's model roster", "credential", credentialID, "error", err)
		}
	}
}

type refreshAssembly struct {
	records []catalog.ModelRecord
	facts   []catalog.FactsRecord
	seen    map[string]bool
	matched int
	priced  int
	added   int
}

func assembleRefreshModels(roster []rosterEntry, mdIdx *catalog.ModelsDevIndex, providerID, modelsDevID string, existing map[string]catalog.Model) refreshAssembly {
	out := refreshAssembly{seen: make(map[string]bool, len(roster))}
	for _, entry := range roster {
		out.add(entry, mdIdx, providerID, modelsDevID, existing)
	}
	return out
}

func (a *refreshAssembly) add(entry rosterEntry, mdIdx *catalog.ModelsDevIndex, providerID, modelsDevID string, existing map[string]catalog.Model) {
	a.seen[entry.ID] = true
	pmMatch, pmRef, mdModel, foundMD := matchRefreshEntry(entry, mdIdx, modelsDevID)
	if foundMD {
		a.matched++
	}
	if priceRefreshEntry(entry, mdModel, foundMD) {
		a.priced++
	}
	enabled, pmMatch, pmRef, added := refreshModelEnabled(entry, existing, pmMatch, pmRef)
	if added {
		a.added++
	}
	available := true
	a.records = append(a.records, catalog.ModelRecord{
		ProviderID:   providerID,
		ModelID:      entry.ID,
		Source:       entry.Source,
		ModelsDevRef: pmRef,
		Match:        pmMatch,
		Enabled:      enabled,
		Available:    &available,
	})
	a.facts = append(a.facts, factsForRefresh(providerID, entry, mdModel, foundMD)...)
}

func matchRefreshEntry(entry rosterEntry, mdIdx *catalog.ModelsDevIndex, modelsDevID string) (string, string, catalog.ModelsDevModel, bool) {
	pmMatch := string(catalog.ModelsDevMatchNone)
	var mdModel catalog.ModelsDevModel
	var foundMD bool
	if mdIdx != nil {
		var matchType catalog.ModelsDevMatch
		mdModel, matchType, foundMD = mdIdx.Match(modelsDevID, entry.ID)
		if foundMD {
			pmMatch = string(matchType)
		}
	}
	if !foundMD {
		return pmMatch, "", mdModel, false
	}
	return pmMatch, mdModel.ID, mdModel, true
}

func priceRefreshEntry(entry rosterEntry, mdModel catalog.ModelsDevModel, foundMD bool) bool {
	var provPrices Prices
	if entry.Prices != nil {
		provPrices = toPrices(*entry.Prices)
	}
	var mdPrices Prices
	if foundMD {
		mdPrices = toPrices(mdModel.Prices)
	}
	effPrices, _ := resolvePrices(catalog.PriceRecord{}, provPrices.row(), mdPrices.row())
	return effPrices.Known()
}

func refreshModelEnabled(entry rosterEntry, existing map[string]catalog.Model, pmMatch, pmRef string) (bool, string, string, bool) {
	// A model the provider lists is on: a catalog that labels it deprecated
	// describes it, and the operator's own switch is what decides whether it
	// routes.
	if prev, ok := existing[entry.ID]; ok {
		// The switch belongs to the operator, and a hand-picked price stays
		// hand-picked, so a refresh re-decides neither.
		if prev.Match == string(catalog.ModelsDevMatchManual) {
			return prev.Enabled, prev.Match, prev.ModelsDevRef, false
		}
		return prev.Enabled, pmMatch, pmRef, false
	}
	return true, pmMatch, pmRef, true
}

func factsForRefresh(providerID string, entry rosterEntry, mdModel catalog.ModelsDevModel, foundMD bool) []catalog.FactsRecord {
	var facts []catalog.FactsRecord
	var provPrices Prices
	if entry.Prices != nil {
		provPrices = toPrices(*entry.Prices)
	}
	// The provider's own facts are recorded whatever it publishes, so a
	// provider that states a name or a context window and no price keeps
	// what it said.
	providerFact := catalog.FactsRecord{
		ProviderID:    providerID,
		ModelID:       entry.ID,
		Layer:         "provider",
		Name:          stringPtrOrNil(entry.Name),
		ContextWindow: entry.ContextWindow,
		MaxOutput:     entry.MaxOutput,
		Prices:        provPrices.row(),
	}
	if !providerFact.Empty() {
		facts = append(facts, providerFact)
	}
	if foundMD {
		facts = append(facts, factsFor(providerID, entry.ID, mdModel))
	}
	return facts
}

type refreshFinish struct {
	listed          []catalog.Listed
	assembly        refreshAssembly
	existing        []catalog.Model
	accountRosters  map[string][]string
	accountFailures []string
	atMs            int64
	mdState         catalog.ModelsDevState
}

func (s *Service) finishRefresh(ctx context.Context, providerID string, fin refreshFinish) (RefreshResult, error) {
	// A model the provider no longer lists is kept for the groups that still
	// reference it, and marked unavailable. A listing that answered with nothing
	// at all is the same evidence as a listing that dropped one id.
	unavailable := markRefreshUnavailable(fin.existing, fin.assembly.seen)
	if err := s.store.RefreshProviderModels(ctx, providerID, fin.assembly.records, fin.assembly.facts, unavailable, fin.atMs, ""); err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{
		Status:          RefreshUpdated,
		Listed:          len(fin.listed),
		Added:           fin.assembly.added,
		Unavailable:     len(unavailable),
		Matched:         fin.assembly.matched,
		Priced:          fin.assembly.priced,
		Accounts:        len(fin.accountRosters),
		AccountFailures: fin.accountFailures,
		ModelsDev:       fin.mdState,
	}, nil
}

func markRefreshUnavailable(existing []catalog.Model, seen map[string]bool) []string {
	var unavailable []string
	for _, m := range existing {
		// A clone is not part of the provider's own list, so a refresh is not
		// evidence about it and never marks it unavailable. The listing takes
		// the row over only when it claims the clone's id.
		if m.ClonedFrom != "" {
			continue
		}
		if !seen[m.ID] {
			unavailable = append(unavailable, m.ID)
		}
	}
	return unavailable
}

type accountRosters struct {
	union    []catalog.Listed
	rosters  map[string][]string
	failures []string
	firstErr error
	seen     map[string]bool
}

func (s *Service) discoverAccounts(ctx context.Context, host catalog.Provider, entries []account.PoolEntry) ([]catalog.Listed, map[string][]string, []string, error) {
	acc := &accountRosters{
		union:   make([]catalog.Listed, 0, 64),
		rosters: make(map[string][]string, len(entries)),
		seen:    map[string]bool{},
	}
	for _, entry := range entries {
		s.discoverAccountRoster(ctx, host, entry, acc)
	}
	if len(acc.rosters) == 0 {
		if acc.firstErr != nil {
			return nil, nil, acc.failures, acc.firstErr
		}
		return nil, nil, acc.failures, fmt.Errorf("%s: %w", host.ID, account.ErrNoCredentials)
	}
	return acc.union, acc.rosters, acc.failures, nil
}

func (s *Service) discoverAccountRoster(ctx context.Context, host catalog.Provider, entry account.PoolEntry, acc *accountRosters) {
	listed, err := s.DiscoverAccountModels(ctx, host, entry.ID)
	if err != nil {
		name := entry.Label
		if strings.TrimSpace(name) == "" {
			name = entry.ID
		}
		acc.failures = append(acc.failures, fmt.Sprintf("%s: %v", name, err))
		if acc.firstErr == nil {
			acc.firstErr = err
		}
		return
	}
	modelIDs := make([]string, 0, len(listed))
	for _, item := range listed {
		modelIDs = append(modelIDs, item.ID)
		if acc.seen[item.ID] {
			continue
		}
		acc.seen[item.ID] = true
		acc.union = append(acc.union, item)
	}
	acc.rosters[entry.ID] = modelIDs
}

func (s *Service) activeAccounts(providerID string) []account.PoolEntry {
	if s.pools == nil {
		return nil
	}
	entries := s.pools.GetPool(providerID).Entries()
	active := make([]account.PoolEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Status == account.StatusActive {
			active = append(active, entry)
		}
	}
	return active
}

// CatalogRefreshOutcome is how one connection fared in an update of every
// connected provider, named so the console can show the connections that
// failed beside the ones that worked.
type CatalogRefreshOutcome struct {
	ProviderID  string
	Label       string
	Status      string
	Detail      string
	Listed      int
	Added       int
	Unavailable int
	Matched     int
	Priced      int
	Accounts    int
	// AccountFailures names the accounts whose own roster could not be read,
	// so a connection that answered in part says which part was missing.
	AccountFailures []string
}

// CatalogRefreshResult is what the one update action did: the model catalog it
// downloaded, and what each connection's own model list said.
type CatalogRefreshResult struct {
	Metadata      PricesReport
	MetadataError string
	Providers     []CatalogRefreshOutcome
	Updated       int
	Skipped       int
	Failed        int
}

const catalogRefreshConcurrency = 4

// RefreshCatalog downloads the newest model catalog and reads every connected
// provider's own model list again, which is the one update the console offers.
//
// One connection failing never discards another's work: a listing that fails
// keeps the roster it had, every other connection is still updated, and the
// result names each one. The catalog is downloaded first, so a model a
// provider lists for the first time is priced the moment it appears.
func (s *Service) RefreshCatalog(ctx context.Context) (CatalogRefreshResult, error) {
	result := CatalogRefreshResult{Providers: []CatalogRefreshOutcome{}}
	report, _ := s.RefreshModelsDev(ctx)
	result.Metadata = report
	result.MetadataError = metadataErrorOf(report)

	hosts, err := s.Providers(ctx, ProviderQuery{})
	if err != nil {
		return result, err
	}
	targets := refreshTargets(ctx, s, hosts)
	result.Providers, result.Updated, result.Skipped, result.Failed = s.refreshTargets(ctx, targets)
	_ = s.reload(ctx)
	return result, nil
}

func refreshTargets(ctx context.Context, s *Service, hosts []Provider) []Provider {
	// A connection is connected when it holds an active credential, and the
	// stored rows are the half of that a connection added in this run has: the
	// pool the snapshot counts is loaded at startup, so the row is what tells
	// the update about a connection nothing has restarted since.
	holders := map[string]bool{}
	if rows, readErr := s.entries.List(ctx); readErr == nil {
		for _, row := range rows {
			if row.Status == account.StatusActive {
				holders[row.ProviderID] = true
			}
		}
	}
	targets := make([]Provider, 0, len(hosts))
	for _, host := range hosts {
		// A connection with no account cannot reach its provider, and one that
		// publishes no model list has no list to read.
		if !host.Enabled || host.ModelsFormat == string(catalog.ModelsNone) {
			continue
		}
		if !host.Configured && !holders[host.ID] {
			continue
		}
		targets = append(targets, host)
	}
	return targets
}

func (s *Service) refreshTargets(ctx context.Context, targets []Provider) ([]CatalogRefreshOutcome, int, int, int) {
	outcomes := make([]CatalogRefreshOutcome, len(targets))
	slots := make(chan struct{}, catalogRefreshConcurrency)
	var wait sync.WaitGroup
	for i, host := range targets {
		wait.Add(1)
		go func(index int, host Provider) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			outcomes[index] = s.catalogOutcome(ctx, host)
		}(i, host)
	}
	wait.Wait()

	var updated, skipped, failed int
	providers := make([]CatalogRefreshOutcome, 0, len(outcomes))
	for _, outcome := range outcomes {
		providers = append(providers, outcome)
		switch outcome.Status {
		case RefreshUpdated:
			updated++
		case RefreshSkipped:
			skipped++
		default:
			failed++
		}
	}
	return providers, updated, skipped, failed
}

func (s *Service) catalogOutcome(ctx context.Context, host Provider) CatalogRefreshOutcome {
	outcome := CatalogRefreshOutcome{ProviderID: host.ID, Label: host.Label, Status: RefreshFailed}
	res, err := s.refreshProvider(ctx, host.ID)
	if res.Status != "" {
		outcome.Status = res.Status
	}
	if err != nil {
		outcome.Detail = err.Error()
		return outcome
	}
	outcome.Listed = res.Listed
	outcome.Added = res.Added
	outcome.Unavailable = res.Unavailable
	outcome.Matched = res.Matched
	outcome.Priced = res.Priced
	outcome.Accounts = res.Accounts
	outcome.AccountFailures = res.AccountFailures
	return outcome
}

func metadataErrorOf(report PricesReport) string {
	if report.ModelsDev.LastError != "" {
		return report.ModelsDev.LastError
	}
	if !report.ModelsDev.Available {
		return "the model catalog is unavailable"
	}
	return ""
}
