// Connection probes: verifying a credential before saving.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jonaskahn/relo/internal/account"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	"github.com/jonaskahn/relo/internal/catalog"
)

// ProbeCheck is one verification a connection probe ran: the model listing,
// the credential, or the model index, and whether it passed.
type ProbeCheck struct {
	Name   string
	Status string // pass, fail, warn, skip
	Detail string
}

// ProbeModel is one model a probe found, with its match against the index
// and the rates review shows before the operator confirms.
type ProbeModel struct {
	ID            string
	Name          string
	ContextWindow *int64
	MaxOutput     *int64
	Match         string
	ModelsDevRef  string
	Source        string
	Prices        ModelPrices
	Enabled       bool
}

// ProbeRequest is the connection an operator wants to verify: a template or
// an existing connection, plus the credential to test with.
type ProbeRequest struct {
	TemplateID   string
	ProviderID   string
	CustomID     string
	Label        string
	APIFormat    string
	BaseURL      string
	KeyHeader    string
	ModelsFormat string
	Variables    map[string]string
	Headers      map[string]string
	Credential   struct {
		Kind     string
		Secret   string
		Priority int
	}
	ManualModels []ManualModel
}

// ManualModel is one model an operator typed, which is how Azure deployments
// and a provider with no list of its own arrive at a probe.
type ManualModel struct {
	ModelID  string
	PricedAs string
}

// ProbeResult is what a verified connection would become: its checks, the
// roster it publishes, and the session id that commits it.
type ProbeResult struct {
	ProbeID     string
	ExpiresAtMs int64
	Checks      []ProbeCheck
	Counts      struct {
		Listed      int
		Matched     int
		Priced      int
		FromListing int
		FromManual  int
	}
	Models         []ProbeModel
	ModelsDevState catalog.ModelsDevState
	// TargetProviderID names the provider an add-key probe would store its
	// credential on. It is empty for a probe that would create a provider.
	TargetProviderID string
}

// CommitProbeRequest confirms a verified probe: the connection to create or
// extend and the label review chose for it.
type CommitProbeRequest struct {
	ProviderID   string
	Label        string
	AccountLabel string
	// DisabledModels is the switch state Review collected. Absent means the
	// probe's own decision stands; present means the list is the decision.
	DisabledModels *[]string
}

type probeSession struct {
	ID        string
	ExpiresAt time.Time
	Provider  catalog.ConnectionRecord
	Cred      account.PoolEntry
	Secret    string
	Models    []catalog.ModelRecord
	Facts     []catalog.FactsRecord
	// Target is the provider an add-key probe stores its credential on, which
	// is a provider that already exists rather than one this probe creates.
	Target string
}

const probeLifetime = 10 * time.Minute

const credentialRefreshTimeout = 2 * time.Minute

func (s *Service) finishCredentialProbe(providerID string, req ProbeRequest, checks []ProbeCheck, auth catalog.Auth, state catalog.ModelsDevState) (ProbeResult, error) {
	probeID := randomID()
	expiresAt := time.Now().Add(probeLifetime)
	session := &probeSession{
		ID: probeID, ExpiresAt: expiresAt, Target: providerID,
		Cred: account.PoolEntry{
			Kind: storedCredentialKind(req.Credential.Kind, auth), Label: "default",
			Status: account.StatusActive, Priority: req.Credential.Priority,
		},
		Secret: req.Credential.Secret,
	}
	s.probes.Store(probeID, session)
	return ProbeResult{
		ProbeID: probeID, ExpiresAtMs: expiresAt.UnixMilli(), Checks: checks,
		TargetProviderID: providerID, Models: []ProbeModel{}, ModelsDevState: state,
	}, nil
}

func storedCredentialKind(requested string, auth catalog.Auth) string {
	kind := credentialKind(auth)
	if requested != "" && requested != string(auth) && requested != "none" {
		return requested
	}
	return kind
}

func credentialKind(auth catalog.Auth) string {
	switch auth {
	case catalog.AuthOAuth:
		return "oauth"
	case catalog.AuthAWS:
		return "aws_keys"
	case catalog.AuthGCP:
		return "gcp_service_account"
	case catalog.AuthNone:
		return "none"
	default:
		return "api_key"
	}
}

// ProbeProvider verifies a credential against its vendor before anything is
// saved, so a mistyped key is refused at setup rather than at first request.
type probePlan struct {
	template            catalog.Template
	origin              catalog.Origin
	targetID            string
	targetVariables     map[string]string
	apiFormat           string
	keyHeader           string
	baseURL             string
	modelsFormat        string
	authMethod          catalog.Auth
	modelsDevProviderID string
	authz               catalog.Authorization
	canList             bool
}

func (p *probePlan) resolveEndpoint(req ProbeRequest) {
	t := p.template
	p.apiFormat = req.APIFormat
	if p.apiFormat == "" {
		p.apiFormat = string(t.DefaultFormat)
	}
	chosen, hasFormat := t.FormatOption(catalog.APIFormat(p.apiFormat))
	p.baseURL = req.BaseURL
	if p.baseURL == "" {
		p.baseURL = t.DefaultBaseURL
	}
	p.keyHeader = req.KeyHeader
	if p.keyHeader == "" && hasFormat && chosen.KeyHeader != "" {
		p.keyHeader = string(chosen.KeyHeader)
	}
	if p.keyHeader == "" {
		p.keyHeader = string(t.KeyHeader)
	}
	if p.keyHeader == "" {
		p.keyHeader = string(catalog.KeyHeaderBearer)
	}
	p.modelsFormat = req.ModelsFormat
	if p.modelsFormat == "" && hasFormat && chosen.ModelsFormat != "" {
		p.modelsFormat = string(chosen.ModelsFormat)
	}
	if p.modelsFormat == "" {
		p.modelsFormat = string(t.ModelsFormat)
	}
}

func (p *probePlan) resolveIdentity(req ProbeRequest) {
	p.authMethod = p.template.Auth
	if req.Credential.Kind == "none" || (p.template.Auth == catalog.AuthNone && req.Credential.Secret == "") {
		p.authMethod = catalog.AuthNone
	}
	p.modelsDevProviderID = p.template.ModelsDevProviderID
	if p.modelsDevProviderID == "" {
		p.modelsDevProviderID = p.template.ID
	}
}

func (s *Service) resolveProbeTemplate(ctx context.Context, req ProbeRequest, mdIdx *catalog.ModelsDevIndex) (catalog.Template, catalog.Origin, string, map[string]string, bool) {
	if req.TemplateID != "" {
		t, found := s.templates.Get(req.TemplateID, mdIdx)
		if !found {
			return catalog.Template{}, "", "", nil, false
		}
		return t, t.Origin, "", nil, true
	}
	if req.ProviderID != "" {
		return s.providerProbeTemplate(ctx, req, mdIdx)
	}
	return catalog.Template{}, "", "", nil, false
}

func (s *Service) providerProbeTemplate(ctx context.Context, req ProbeRequest, mdIdx *catalog.ModelsDevIndex) (catalog.Template, catalog.Origin, string, map[string]string, bool) {
	existing, err := s.store.GetProvider(ctx, req.ProviderID)
	if err != nil {
		return catalog.Template{}, "", "", nil, false
	}
	t := catalog.Template{
		ID:                  existing.ID,
		Label:               existing.Label,
		Origin:              catalog.Origin(existing.Origin),
		Auth:                catalog.Auth(existing.Auth),
		DefaultFormat:       catalog.APIFormat(existing.APIFormat),
		KeyHeader:           catalog.KeyHeader(existing.KeyHeader),
		DefaultBaseURL:      existing.BaseURL,
		ModelsSource:        existing.ModelsSource,
		ModelsFormat:        catalog.ModelsFormat(existing.ModelsFormat),
		ModelsDevProviderID: existing.ModelsDevProviderID,
		Headers:             existing.Headers,
	}
	// The stored row keeps connection details, not the catalog policy the
	// template declares, so the roster mode and the model filter come from
	// the template this row was added from.
	if catalogTemplate, ok := s.templates.Get(existing.TemplateID, mdIdx); ok {
		t.Kind = catalogTemplate.Kind
		t.ModelsSource = catalogTemplate.ModelsSource
		t.FilterModels = catalogTemplate.FilterModels
	}
	return t, catalog.Origin(existing.Origin), existing.ID, existing.Variables, true
}

func customProbeTemplate(req ProbeRequest) catalog.Template {
	t := catalog.Template{
		ID:             req.CustomID,
		Label:          req.Label,
		Origin:         catalog.OriginCustom,
		Auth:           catalog.AuthAPIKey,
		KeyHeader:      catalog.KeyHeader(req.KeyHeader),
		DefaultFormat:  catalog.APIFormat(req.APIFormat),
		DefaultBaseURL: req.BaseURL,
		ModelsSource:   "listing",
		ModelsFormat:   catalog.ModelsFormat(req.ModelsFormat),
		Headers:        req.Headers,
	}
	if t.KeyHeader == "" {
		t.KeyHeader = catalog.KeyHeaderBearer
	}
	if t.ModelsFormat == "" {
		t.ModelsFormat = catalog.ModelsOpenAI
	}
	return t
}

func reportListingFailure(checks []ProbeCheck, state catalog.ModelsDevState, listErr error) (ProbeResult, error) {
	if errors.Is(listErr, catalog.ErrListRejected) {
		checks = append(checks, ProbeCheck{
			Name:   "credential",
			Status: "fail",
			Detail: "credential rejected by upstream (401/403)",
		})
		return ProbeResult{Checks: checks, ModelsDevState: state}, ErrCredentialRejected
	}
	checks = append(checks, ProbeCheck{
		Name:   "listing",
		Status: "fail",
		Detail: listErr.Error(),
	})
	return ProbeResult{Checks: checks, ModelsDevState: state}, ErrListingFailed
}

func appendListingChecks(checks []ProbeCheck, listed []catalog.Listed, canList bool, req ProbeRequest, auth catalog.Auth) []ProbeCheck {
	if len(listed) > 0 {
		checks = append(checks, ProbeCheck{
			Name:   "credential",
			Status: "pass",
			Detail: "credential verified",
		})
		return append(checks, ProbeCheck{
			Name:   "listing",
			Status: "pass",
			Detail: fmt.Sprintf("%d models listed from provider", len(listed)),
		})
	}
	if !canList && len(req.ManualModels) > 0 {
		return appendManualListingChecks(checks, req)
	}
	checks = append(checks, ProbeCheck{
		Name:   "listing",
		Status: "skip",
		Detail: "this provider publishes no model list",
	})
	return appendUnlistedCredentialCheck(checks, canList, req, auth)
}

func appendManualListingChecks(checks []ProbeCheck, req ProbeRequest) []ProbeCheck {
	checks = append(checks, ProbeCheck{
		Name:   "credential",
		Status: "pass",
		Detail: "manual models specified",
	})
	return append(checks, ProbeCheck{
		Name:   "listing",
		Status: "pass",
		Detail: fmt.Sprintf("%d manual models declared", len(req.ManualModels)),
	})
}

func appendUnlistedCredentialCheck(checks []ProbeCheck, canList bool, req ProbeRequest, auth catalog.Auth) []ProbeCheck {
	if canList {
		return checks
	}
	// Nothing was sent, so the credential is only exercised when it had
	// to be exchanged first. Vertex publishes no model list, so a
	// service account that minted a token here is a check that passed.
	switch {
	case auth == catalog.AuthGCP && req.Credential.Secret != "":
		return append(checks, ProbeCheck{
			Name: "credential", Status: "pass",
			Detail: "credential exchanged for an access token",
		})
	case auth == catalog.AuthNone:
		return append(checks, ProbeCheck{
			Name: "credential", Status: "skip",
			Detail: "this provider needs no credential",
		})
	default:
		return append(checks, ProbeCheck{
			Name: "credential", Status: "skip",
			Detail: "this provider publishes no model list",
		})
	}
}

func (p *probePlan) probeRoster(t catalog.Template, req ProbeRequest, listed []catalog.Listed) []rosterEntry {
	// typed is what the operator typed, and it is the whole roster of a
	// connection that publishes no list of its own: Azure deployment names,
	// Vertex model ids, and the models a Kiro account serves. A connection that
	// does publish a list is never asked for ids by hand, because its list is
	// what decides which models it serves.
	typed := make([]catalog.Listed, 0, len(req.ManualModels))
	if !p.canList {
		for _, mm := range req.ManualModels {
			typed = append(typed, catalog.Listed{ID: mm.ModelID, Name: mm.ModelID})
		}
	}
	return buildRoster(t, rosterModeFor(t), listed, typed)
}

func (s *Service) authorizeProbe(ctx context.Context, req ProbeRequest, t catalog.Template, plan *probePlan, targetVariables map[string]string) ([]ProbeCheck, error) {
	// The credential is authorized the same way a routed request authorizes
	// it, so a key that fails here fails for the reason a request would have
	// failed for, and an AWS key is signed and a service account exchanged
	// before either is judged.
	var authErr error
	plan.authz, authErr = s.direct(ctx, catalog.AuthRequest{
		ProviderID: t.ID,
		Auth:       plan.authMethod,
		KeyHeader:  catalog.KeyHeader(plan.keyHeader),
		Region:     firstNonEmpty(req.Variables["region"], targetVariables["region"]),
		Service:    "bedrock",
	}, req.Credential.Secret)
	// A sign-in credential cannot be resolved without the account it belongs
	// to, so a probe reports it as unchecked rather than refused: the login
	// that produces it is what a console checks, and the listing below still
	// reports what an unsigned request is told.
	if authErr != nil && plan.authMethod != catalog.AuthNone && plan.authMethod != catalog.AuthOAuth {
		return []ProbeCheck{{
			Name:   "credential",
			Status: "fail",
			Detail: authErr.Error(),
		}}, ErrCredentialRejected
	}
	return nil, nil
}

func (s *Service) fetchProbeListing(ctx context.Context, req ProbeRequest, t catalog.Template, plan *probePlan, targetProviderID string) ([]catalog.Listed, error) {
	// canList is whether this provider publishes a model list Relo can read. A
	// provider without one gets its models from the operator instead.
	plan.canList = plan.modelsFormat != string(catalog.ModelsNone) && s.discover != nil && plan.baseURL != ""
	if !plan.canList {
		return nil, nil
	}
	target := catalog.ListTarget{
		Format:  catalog.ModelsFormat(plan.modelsFormat),
		BaseURL: plan.baseURL,
		Auth:    plan.authz,
		Headers: req.Headers,
	}
	target.OpenCodeFree = t.ID == catalog.OpenCodeFreeTemplate
	return s.discover.List(s.withProxy(ctx, s.probeUsesProxy(ctx, t.ID, targetProviderID)), target)
}

func (s *Service) openProbePlan(ctx context.Context, req ProbeRequest) (probePlan, *catalog.ModelsDevIndex, catalog.ModelsDevState, error) {
	mdIdx, mdErr := s.modelsDev.Get(ctx, catalog.FetchOnlineFirst)
	mdState := s.modelsDev.CurrentState()
	t, origin, targetProviderID, targetVariables, templateFound := s.resolveProbeTemplate(ctx, req, mdIdx)
	if !templateFound {
		origin = catalog.OriginCustom
		t = customProbeTemplate(req)
	}
	plan := probePlan{
		template: t, origin: origin,
		targetID: targetProviderID, targetVariables: targetVariables,
	}
	plan.resolveEndpoint(req)
	plan.resolveIdentity(req)
	return plan, mdIdx, mdState, mdErr
}

// ProbeProvider stages one connection's listing for review: what it publishes,
// what it costs, and what checks it failed, without writing anything.
func (s *Service) ProbeProvider(ctx context.Context, req ProbeRequest) (ProbeResult, error) {
	plan, mdIdx, mdState, mdErr := s.openProbePlan(ctx, req)
	t := plan.template
	targetProviderID := plan.targetID

	var checks []ProbeCheck

	if failChecks, err := s.authorizeProbe(ctx, req, t, &plan, plan.targetVariables); err != nil {
		return ProbeResult{Checks: failChecks, ModelsDevState: mdState}, err
	}
	listed, listErr := s.fetchProbeListing(ctx, req, t, &plan, targetProviderID)

	if listErr != nil {
		return reportListingFailure(checks, mdState, listErr)
	}
	checks = appendListingChecks(checks, listed, plan.canList, req, plan.authMethod)

	roster := plan.probeRoster(t, req, listed)

	checks = append(checks, modelsDevCheck(mdIdx, mdState, mdErr))

	if targetProviderID != "" {
		return s.finishCredentialProbe(targetProviderID, req, checks, plan.authMethod, mdState)
	}

	assembly := assembleProbeModels(roster, mdIdx, plan.modelsDevProviderID)
	checks = appendPricingCheck(checks, assembly.priced, len(assembly.models))

	return s.storeProbe(plan, req, listed, assembly, checks, mdState), nil
}

type probeAssembly struct {
	models      []ProbeModel
	rows        []catalog.ModelRecord
	facts       []catalog.FactsRecord
	matched     int
	priced      int
	fromListing int
	fromManual  int
}

func assembleProbeModels(roster []rosterEntry, mdIdx *catalog.ModelsDevIndex, providerID string) probeAssembly {
	var out probeAssembly
	for _, entry := range roster {
		out.add(entry, mdIdx, providerID)
	}
	return out
}

func (a *probeAssembly) add(entry rosterEntry, mdIdx *catalog.ModelsDevIndex, providerID string) {
	pm := ProbeModel{
		ID:            entry.ID,
		Name:          entry.Name,
		ContextWindow: entry.ContextWindow,
		MaxOutput:     entry.MaxOutput,
		Match:         string(catalog.ModelsDevMatchNone),
		Source:        entry.Source,
		Enabled:       true,
	}
	switch entry.Source {
	case SourceListing:
		a.fromListing++
	default:
		a.fromManual++
	}
	mdModel, foundMD := matchProbeModel(&pm, entry, mdIdx, providerID)
	if foundMD {
		a.matched++
	}
	if priceProbeModel(&pm, entry, mdModel, foundMD).Known() {
		a.priced++
	}
	a.models = append(a.models, pm)
	a.appendProbeRow(entry, pm)
	a.facts = append(a.facts, factsForEntry(entry, pm, mdModel, foundMD)...)
}

func (a *probeAssembly) appendProbeRow(entry rosterEntry, pm ProbeModel) {
	a.rows = append(a.rows, catalog.ModelRecord{
		ModelID:      entry.ID,
		Source:       entry.Source,
		APIFormat:    string(entry.Format),
		ModelsDevRef: pm.ModelsDevRef,
		Match:        pm.Match,
		Enabled:      pm.Enabled,
	})
}

func matchProbeModel(pm *ProbeModel, entry rosterEntry, mdIdx *catalog.ModelsDevIndex, providerID string) (catalog.ModelsDevModel, bool) {
	var mdModel catalog.ModelsDevModel
	var matchType catalog.ModelsDevMatch
	var found bool
	if mdIdx != nil {
		mdModel, matchType, found = mdIdx.Match(providerID, entry.ID)
	}
	if !found {
		return mdModel, false
	}
	pm.Match = string(matchType)
	pm.ModelsDevRef = mdModel.ID
	if pm.Name == entry.ID && mdModel.Name != "" {
		pm.Name = mdModel.Name
	}
	if pm.ContextWindow == nil {
		pm.ContextWindow = mdModel.ContextWindow
	}
	if pm.MaxOutput == nil {
		pm.MaxOutput = mdModel.MaxOutput
	}
	return mdModel, true
}

func priceProbeModel(pm *ProbeModel, entry rosterEntry, mdModel catalog.ModelsDevModel, foundMD bool) catalog.Prices {
	var provPrices Prices
	if entry.Prices != nil {
		provPrices = toPrices(*entry.Prices)
	}
	var mdPrices Prices
	if foundMD {
		mdPrices = toPrices(mdModel.Prices)
	}
	effPrices, effSrc := resolvePrices(catalog.PriceRecord{}, provPrices.row(), mdPrices.row())
	pm.Prices = ModelPrices{
		Provider:        provPrices,
		ModelsDev:       mdPrices,
		Effective:       toPrices(effPrices),
		EffectiveSource: PriceSourceMap(effSrc),
	}
	return effPrices
}

func factsForEntry(entry rosterEntry, pm ProbeModel, mdModel catalog.ModelsDevModel, foundMD bool) []catalog.FactsRecord {
	var facts []catalog.FactsRecord
	// The provider's own facts are recorded whatever it publishes, so a
	// provider that states a name or a context window and no price still
	// keeps what it said.
	providerFact := catalog.FactsRecord{
		ModelID:       entry.ID,
		Layer:         "provider",
		Name:          stringPtrOrNil(entry.Name),
		ContextWindow: entry.ContextWindow,
		MaxOutput:     entry.MaxOutput,
		Prices:        pm.Prices.Provider.row(),
	}
	if !providerFact.Empty() {
		facts = append(facts, providerFact)
	}
	if foundMD {
		facts = append(facts, factsFor("", entry.ID, mdModel))
	}
	return facts
}

func appendPricingCheck(checks []ProbeCheck, priced, total int) []ProbeCheck {
	if priced > 0 {
		return append(checks, ProbeCheck{
			Name:   "pricing",
			Status: "pass",
			Detail: fmt.Sprintf("%d/%d models priced", priced, total),
		})
	}
	if total > 0 {
		return append(checks, ProbeCheck{
			Name:   "pricing",
			Status: "warn",
			Detail: "no pricing found in models.dev",
		})
	}
	return checks
}

func (p *probePlan) probeConnectionRecord(req ProbeRequest) catalog.ConnectionRecord {
	t := p.template
	return catalog.ConnectionRecord{
		TemplateID:          t.ID,
		Origin:              string(p.origin),
		Label:               t.Label,
		Auth:                string(p.authMethod),
		APIFormat:           p.apiFormat,
		KeyHeader:           p.keyHeader,
		BaseURL:             p.baseURL,
		ModelsSource:        t.ModelsSource,
		ModelsFormat:        p.modelsFormat,
		ModelsDevProviderID: p.modelsDevProviderID,
		Headers:             req.Headers,
		DocURL:              t.DocURL,
		KeyEnv:              t.KeyEnv,
		LoginFlows:          t.LoginFlows,
		Variables:           req.Variables,
		Enabled:             true,
		Rank:                100,
		PoolStrategy:        "least-loaded",
		UseProxy:            t.ID == catalog.OpenCodeFreeTemplate,
	}
}

func newProbeSession(plan probePlan, req ProbeRequest, assembly probeAssembly, probeID string, expiresAt time.Time) *probeSession {
	return &probeSession{
		ID:        probeID,
		ExpiresAt: expiresAt,
		Provider:  plan.probeConnectionRecord(req),
		Cred: account.PoolEntry{
			Kind:     storedCredentialKind(req.Credential.Kind, plan.authMethod),
			Label:    "default",
			Status:   "active",
			Priority: req.Credential.Priority,
		},
		Secret: req.Credential.Secret,
		Models: assembly.rows,
		Facts:  assembly.facts,
	}
}

func (s *Service) storeProbe(plan probePlan, req ProbeRequest, listed []catalog.Listed, assembly probeAssembly, checks []ProbeCheck, state catalog.ModelsDevState) ProbeResult {
	probeID := randomID()
	expiresAt := time.Now().Add(probeLifetime)
	s.probes.Store(probeID, newProbeSession(plan, req, assembly, probeID, expiresAt))
	res := ProbeResult{
		ProbeID:        probeID,
		ExpiresAtMs:    expiresAt.UnixMilli(),
		Checks:         checks,
		Models:         assembly.models,
		ModelsDevState: state,
	}
	res.Counts.Listed = len(listed)
	res.Counts.Matched = assembly.matched
	res.Counts.Priced = assembly.priced
	res.Counts.FromListing = assembly.fromListing
	res.Counts.FromManual = assembly.fromManual
	return res
}

func modelsDevCheck(mdIdx *catalog.ModelsDevIndex, state catalog.ModelsDevState, mdErr error) ProbeCheck {
	switch {
	case mdErr != nil && (mdIdx == nil || len(mdIdx.Providers) == 0):
		return ProbeCheck{Name: "modelsdev", Status: "warn", Detail: "models.dev data unavailable"}
	case state.Stale:
		return ProbeCheck{Name: "modelsdev", Status: "warn", Detail: "using the saved models.dev copy"}
	default:
		return ProbeCheck{
			Name: "modelsdev", Status: "pass",
			Detail: fmt.Sprintf("models.dev active (%d providers)", len(mdIdx.Providers)),
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// CommitProbe saves a verified probe as a connection and its first account,
// so review is the last step before the credential goes live.
func (s *Service) CommitProbe(ctx context.Context, probeID string, req CommitProbeRequest) (Provider, error) {
	session, err := s.loadProbeSession(probeID)
	if err != nil {
		return Provider{}, err
	}

	// A probe that named a provider adds one credential to that provider and
	// changes nothing else about it, so no roster and no label are written.
	if session.Target != "" {
		return s.commitCredentialProbe(ctx, probeID, session, req)
	}
	pID := s.assignCommitIDs(ctx, session, req)
	applyCommitModels(session, req)

	if err := s.persistCommitSession(ctx, session); err != nil {
		return Provider{}, err
	}
	// The credential the transaction just stored is put into rotation too: the
	// row and the pool entry are two halves of one account, and a pool that
	// never heard of it reads as a provider with no account, which leaves the
	// connection out of every roster read and every update until a restart.
	if s.pools != nil && session.Cred.ID != "" {
		s.pools.Adopt(session.Cred)
	}

	s.probes.Delete(probeID)
	_ = s.reload(ctx)

	return s.Provider(ctx, pID)
}

func (s *Service) loadProbeSession(probeID string) (*probeSession, error) {
	val, ok := s.probes.Load(probeID)
	if !ok {
		return nil, ErrProbeNotFound
	}
	session := val.(*probeSession)
	if time.Now().After(session.ExpiresAt) {
		s.probes.Delete(probeID)
		return nil, ErrProbeNotFound
	}
	return session, nil
}

func (s *Service) assignCommitIDs(ctx context.Context, session *probeSession, req CommitProbeRequest) string {
	pID := req.ProviderID
	if pID == "" {
		pID = session.Provider.TemplateID
	}
	if pID == "" {
		pID = randomID()
	}

	// Suffix if needed
	pID = s.uniqueProviderID(ctx, pID)
	session.Provider.ID = pID
	if req.Label != "" {
		session.Provider.Label = req.Label
	}

	if session.Provider.Auth != string(catalog.AuthNone) {
		credID := randomID()
		session.Cred.ID = credID
		session.Cred.ProviderID = pID
		if req.AccountLabel != "" {
			session.Cred.Label = req.AccountLabel
		}
		session.Cred.SecretRef = fmt.Sprintf("apikey/%s/%s", pID, credID)
	}
	return pID
}

func applyCommitModels(session *probeSession, req CommitProbeRequest) {
	pID := session.Provider.ID
	for i := range session.Models {
		session.Models[i].ProviderID = pID
	}
	if req.DisabledModels != nil {
		off := make(map[string]bool, len(*req.DisabledModels))
		for _, mID := range *req.DisabledModels {
			off[mID] = true
		}
		for i := range session.Models {
			session.Models[i].Enabled = !off[session.Models[i].ModelID]
		}
	}
	for i := range session.Facts {
		session.Facts[i].ProviderID = pID
	}
}

func (s *Service) persistCommitSession(ctx context.Context, session *probeSession) error {
	if session.Secret != "" && session.Cred.SecretRef != "" && s.secrets != nil {
		if err := s.secrets.Set(session.Cred.SecretRef, session.Secret); err != nil {
			return fmt.Errorf("save secret: %w", err)
		}
	}

	err := s.store.CommitProbe(ctx, session.Provider, session.Cred, session.Models, session.Facts)
	if err != nil {
		if session.Secret != "" && session.Cred.SecretRef != "" && s.secrets != nil {
			_ = s.secrets.Set(session.Cred.SecretRef, "")
		}
		return fmt.Errorf("commit probe: %w", err)
	}
	return nil
}

// DiscardProbe drops a verified probe without saving anything, which is how
// an operator backs out of a setup they no longer want.
func (s *Service) DiscardProbe(probeID string) {
	s.probes.Delete(probeID)
}

func (s *Service) commitCredentialProbe(ctx context.Context, probeID string, session *probeSession, req CommitProbeRequest) (Provider, error) {
	// AddAccount writes the row and starts the roster refresh the new
	// credential needs, so this path reuses it rather than repeating either.
	if _, err := s.accounts.AddAccount(ctx, appaccount.NewAccount{
		ProviderID:  session.Target,
		Kind:        session.Cred.Kind,
		Label:       req.AccountLabel,
		SecretValue: session.Secret,
		Priority:    session.Cred.Priority,
	}); err != nil {
		return Provider{}, err
	}
	s.probes.Delete(probeID)
	_ = s.reload(ctx)
	return s.Provider(ctx, session.Target)
}

// RefreshAfterCredential refreshes a provider's roster once a new credential
// is in rotation, with a context of its own because the request that started
// it has already answered.
func (s *Service) RefreshAfterCredential(providerID string) {
	ctx, cancel := context.WithTimeout(s.lifetime, credentialRefreshTimeout)
	defer cancel()
	_, _ = s.RefreshProviderModels(ctx, providerID)
}
