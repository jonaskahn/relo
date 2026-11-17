// Package proxy owns the outbound proxy pool: stored entries, their secrets,
// selection, and the in-memory health the relay rotates through.
package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
	"github.com/jonaskahn/relo/internal/config"
)

// Errors name the refusals the API maps to its own wording.
var (
	ErrNotFound   = errors.New("proxy not found")
	ErrDuplicate  = errors.New("proxy already exists")
	ErrInvalid    = config.ErrInvalidProxy
	ErrNoSecret   = errors.New("no secret store is configured")
	ErrNoEnabled  = errors.New("no enabled proxy")
	ErrAllCooling = errors.New("every proxy is cooling down")
)

// AllCoolingError names a pool whose every enabled proxy is cooling, and how
// long the caller should wait before the first one is usable again.
type AllCoolingError struct {
	RetryAfter time.Duration
}

func (e *AllCoolingError) Error() string {
	return fmt.Sprintf("%s; retry after %s", ErrAllCooling, e.RetryAfter.Round(time.Second))
}

func (e *AllCoolingError) Unwrap() error { return ErrAllCooling }

// Entry is one stored proxy. SecretRef points into the secret store; no
// password or canonical URL is stored here.
type Entry struct {
	ID        string
	Label     string
	Scheme    string
	Host      string
	Port      int
	Username  string
	SecretRef string
	Enabled   bool
	Rank      int
	CreatedAt int64
	UpdatedAt int64
}

// Store persists proxy rows. It never sees a password.
type Store interface {
	List(ctx context.Context) ([]Entry, error)
	Get(ctx context.Context, id string) (Entry, bool, error)
	Create(ctx context.Context, entry Entry) (Entry, error)
	Update(ctx context.Context, entry Entry) error
	Delete(ctx context.Context, id string) error
	Count(ctx context.Context) (int, error)
}

// SecretStore is the vault the proxy passwords live in.
type SecretStore interface {
	Set(ref, secret string) error
	Get(ref string) (string, error)
	Delete(ref string) error
	Mode() string
}

// Draft is one proxy a caller wants to store.
type Draft struct {
	Label    string
	Scheme   string
	Host     string
	Port     int
	Username string
	Password string
	Enabled  bool
	Rank     int
}

// Patch changes only the named fields. An empty Password keeps the stored
// password.
type Patch struct {
	Label    *string
	Scheme   *string
	Host     *string
	Port     *int
	Username *string
	Enabled  *bool
	Rank     *int
	Password *string
}

// State names what the pool knows about one proxy right now.
type State string

// The health states the API reports for one pool entry.
const (
	StateReady       State = "ready"
	StateCooling     State = "cooling"
	StateUnverified  State = "unverified"
	StateUnreachable State = "unreachable"
)

// Item is one proxy as the API reports it.
type Item struct {
	Entry
	HasPassword  bool
	State        State
	CoolingUntil *time.Time
}

// Summary totals the pool for the providers card and the doctor.
type Summary struct {
	Total      int
	Enabled    int
	Cooling    int
	AllCooling bool
}

// Selection is the proxy one attempt uses.
type Selection struct {
	ID  string
	URL string
}

// Query filters one page of proxies.
type Query struct {
	Search string
	Status string
	Offset int
	Limit  int
}

// ImportResult reports one pasted line.
type ImportResult struct {
	Line   int
	Raw    string
	Status string
	Reason string
	Item   *Item
}

// Options configure the pool service.
type Options struct {
	Store   Store
	Secrets SecretStore
	Clock   clock.Clock
	Backoff func() []time.Duration
	Check   func(ctx context.Context, rawURL string) error
}

type cachedURL struct {
	url string
}

type health struct {
	state      State
	until      time.Time
	failures   int
	lastError  string
	lastTested time.Time
}

// Service owns proxy writes, cached decrypted URLs, and runtime health.
type Service struct {
	store   Store
	secrets SecretStore
	clock   clock.Clock
	backoff func() []time.Duration
	check   func(ctx context.Context, rawURL string) error

	mu         sync.Mutex
	cache      map[string]cachedURL
	secretMode string
	health     map[string]health
}

// New returns the pool service.
func New(options Options) *Service {
	clk := options.Clock
	if clk == nil {
		clk = clock.New()
	}
	service := &Service{
		store: options.Store, secrets: options.Secrets, clock: clk,
		backoff: options.Backoff, check: options.Check,
		cache: map[string]cachedURL{}, health: map[string]health{},
	}
	if service.secrets != nil {
		service.secretMode = service.secrets.Mode()
	}
	return service
}

func (s *Service) now() time.Time { return s.clock.Now() }

// ClearCache forgets every decrypted URL, which every write and a vault-mode
// change requires.
func (s *Service) ClearCache() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache = map[string]cachedURL{}
	if s.secrets != nil {
		s.secretMode = s.secrets.Mode()
	}
}

func (s *Service) modeChanged() {
	if s.secrets == nil {
		return
	}
	if s.secrets.Mode() != s.secretMode {
		s.cache = map[string]cachedURL{}
		s.secretMode = s.secrets.Mode()
	}
}

// List returns every stored proxy in rank order.
func (s *Service) List(ctx context.Context) ([]Entry, error) {
	rows, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Rank != rows[j].Rank {
			return rows[i].Rank < rows[j].Rank
		}
		return rows[i].ID < rows[j].ID
	})
	return rows, nil
}

// Items returns every proxy with its health, for counts and summaries.
func (s *Service) Items(ctx context.Context) ([]Item, error) {
	rows, err := s.List(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, s.itemLocked(row, now))
	}
	return items, nil
}

func (s *Service) itemLocked(row Entry, now time.Time) Item {
	item := Item{Entry: row, HasPassword: row.SecretRef != "", State: StateUnverified}
	h := s.health[row.ID]
	if now.Before(h.until) {
		until := h.until
		item.State = StateCooling
		item.CoolingUntil = &until
		return item
	}
	switch h.state {
	case StateReady, StateUnreachable:
		item.State = h.state
	case StateCooling:
		item.State = StateUnverified
	}
	return item
}

// Page returns one filtered page, with the cursor as the next offset.
func (s *Service) Page(ctx context.Context, query Query) ([]Item, int, error) {
	items, err := s.Items(ctx)
	if err != nil {
		return nil, 0, err
	}
	filtered := filterProxyItems(items, query)
	limit := query.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := max(query.Offset, 0)
	if offset >= len(filtered) {
		return []Item{}, 0, nil
	}
	end := min(offset+limit, len(filtered))
	next := 0
	if end < len(filtered) {
		next = end
	}
	return filtered[offset:end], next, nil
}

func filterProxyItems(items []Item, query Query) []Item {
	search := strings.ToLower(strings.TrimSpace(query.Search))
	filtered := make([]Item, 0, len(items))
	for _, item := range items {
		if search != "" && !proxyMatches(item, search) {
			continue
		}
		switch query.Status {
		case "enabled":
			if !item.Enabled {
				continue
			}
		case "cooling":
			if item.State != StateCooling {
				continue
			}
		case "disabled":
			if item.Enabled {
				continue
			}
		}
		filtered = append(filtered, item)
	}
	return filtered
}

func proxyMatches(item Item, search string) bool {
	return strings.Contains(strings.ToLower(item.Label+" "+item.Host+" "+item.Username+" "+item.ID), search)
}

// Summary counts the pool by enabled and cooling state.
func (s *Service) Summary(ctx context.Context) (Summary, error) {
	items, err := s.Items(ctx)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Total: len(items)}
	for _, item := range items {
		if item.Enabled {
			summary.Enabled++
			if item.State == StateCooling {
				summary.Cooling++
			}
		}
	}
	summary.AllCooling = summary.Enabled > 0 && summary.Cooling == summary.Enabled
	return summary, nil
}

// FirstURL returns one enabled proxy URL, or empty. It is for discovery,
// which has no request-local rotation to follow.
func (s *Service) FirstURL(ctx context.Context) string {
	selection, err := s.Select(ctx, nil)
	if err != nil {
		return ""
	}
	return selection.URL
}

// URL resolves and caches the decrypted URL of one proxy.
func (s *Service) URL(ctx context.Context, id string) (string, error) {
	s.mu.Lock()
	s.modeChanged()
	if cached, found := s.cache[id]; found {
		clone := cached.url
		s.mu.Unlock()
		return clone, nil
	}
	s.mu.Unlock()
	row, found, err := s.store.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNotFound
	}
	raw, err := s.urlOf(row)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.cache[id] = cachedURL{url: raw}
	s.mu.Unlock()
	return raw, nil
}

func (s *Service) urlOf(row Entry) (string, error) {
	password := ""
	if row.SecretRef != "" {
		if s.secrets == nil {
			return "", ErrNoSecret
		}
		value, err := s.secrets.Get(row.SecretRef)
		if err != nil {
			return "", err
		}
		password = value
	}
	address := &url.URL{Scheme: row.Scheme, Host: net.JoinHostPort(row.Host, fmt.Sprintf("%d", row.Port))}
	if row.Username != "" || password != "" {
		address.User = url.UserPassword(row.Username, password)
	}
	return address.String(), nil
}

// Select picks the healthiest enabled proxy not named in exclude.
func (s *Service) Select(ctx context.Context, exclude []string) (Selection, error) {
	return s.SelectSticky(ctx, "", exclude)
}

// SelectSticky reuses the connection's proxy while it is healthy and enabled,
// then falls through to the first healthy proxy in rank order.
func (s *Service) SelectSticky(ctx context.Context, stickyID string, exclude []string) (Selection, error) {
	items, err := s.Items(ctx)
	if err != nil {
		return Selection{}, err
	}
	skipped := map[string]bool{}
	for _, id := range exclude {
		skipped[id] = true
	}
	choice := healthyProxy(ctx, proxyPick{service: s, items: items, stickyID: stickyID, skipped: skipped})
	if choice.selection != nil {
		return *choice.selection, nil
	}
	if choice.enabled == 0 {
		return Selection{}, ErrNoEnabled
	}
	wait := time.Second
	if choice.earliest != nil {
		if remaining := choice.earliest.Sub(s.now()); remaining > 0 {
			wait = remaining
		}
	}
	return Selection{}, &AllCoolingError{RetryAfter: wait}
}

type proxyChoice struct {
	selection *Selection
	earliest  *time.Time
	enabled   int
}

type proxyPick struct {
	service  *Service
	items    []Item
	stickyID string
	skipped  map[string]bool
}

// nolint:funlen
func healthyProxy(ctx context.Context, pick proxyPick) proxyChoice {
	var earliest *time.Time
	enabled := 0
	for _, item := range pick.items {
		if !item.Enabled {
			continue
		}
		enabled++
		if item.ID == pick.stickyID && item.State != StateCooling && !pick.skipped[item.ID] {
			raw, err := pick.service.URL(ctx, item.ID)
			if err == nil {
				return proxyChoice{selection: &Selection{ID: item.ID, URL: raw}, earliest: earliest, enabled: enabled}
			}
		}
	}
	for _, item := range pick.items {
		if !item.Enabled {
			continue
		}
		if item.State == StateCooling {
			if item.CoolingUntil != nil && (earliest == nil || item.CoolingUntil.Before(*earliest)) {
				until := *item.CoolingUntil
				earliest = &until
			}
			continue
		}
		if pick.skipped[item.ID] {
			continue
		}
		raw, err := pick.service.URL(ctx, item.ID)
		if err != nil {
			continue
		}
		return proxyChoice{selection: &Selection{ID: item.ID, URL: raw}, earliest: earliest, enabled: enabled}
	}
	return proxyChoice{earliest: earliest, enabled: enabled}
}

// Cool begins or extends one proxy's cooldown. The wait walks the configured
// failover ladder and honours a longer provider Retry-After.
func (s *Service) Cool(id string, retryAfter time.Duration) time.Duration {
	if id == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.health[id]
	h.failures++
	wait := retryAfter
	if s.backoff != nil {
		steps := s.backoff()
		if len(steps) > 0 {
			index := min(max(h.failures-1, 0), len(steps)-1)
			if steps[index] > wait {
				wait = steps[index]
			}
		}
	}
	if wait < 0 {
		wait = 0
	}
	h.state = StateCooling
	h.until = s.now().Add(wait)
	s.health[id] = h
	delete(s.cache, id)
	return wait
}

// MarkServed records that a proxy carried a response.
func (s *Service) MarkServed(id string) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health[id] = health{state: StateReady, lastTested: s.now()}
}

// MarkUnreachable records a failed test without starting the rotation ladder.
func (s *Service) MarkUnreachable(id string, cause error) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.health[id]
	h.state = StateUnreachable
	h.lastError = ""
	if cause != nil {
		h.lastError = cause.Error()
	}
	h.lastTested = s.now()
	s.health[id] = h
}

// Test checks one proxy and records what it found.
func (s *Service) Test(ctx context.Context, id string) (Item, error) {
	raw, err := s.URL(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if s.check == nil {
		return Item{}, errors.New("proxy check is not configured")
	}
	if err := s.check(ctx, raw); err != nil {
		s.MarkUnreachable(id, err)
		return s.itemByID(ctx, id)
	}
	s.MarkServed(id)
	return s.itemByID(ctx, id)
}

func (s *Service) itemByID(ctx context.Context, id string) (Item, error) {
	row, found, err := s.store.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if !found {
		return Item{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.itemLocked(row, s.now()), nil
}

// Create stores one proxy and its password.
func (s *Service) Create(ctx context.Context, draft Draft) (Item, error) {
	entry, err := s.buildEntry(draft)
	if err != nil {
		return Item{}, err
	}
	if err := s.ensureUnique(ctx, entry, ""); err != nil {
		return Item{}, err
	}
	entry.ID = newID()
	entry.SecretRef = "proxy/" + entry.ID
	if draft.Password == "" {
		entry.SecretRef = ""
	}
	if err := s.writeNewSecret(entry.SecretRef, draft.Password); err != nil {
		return Item{}, err
	}
	now := s.now().Unix()
	entry.CreatedAt, entry.UpdatedAt = now, now
	stored, err := s.store.Create(ctx, entry)
	if err != nil {
		if entry.SecretRef != "" {
			_ = s.secrets.Delete(entry.SecretRef)
		}
		return Item{}, err
	}
	s.ClearCache()
	return s.itemByID(ctx, stored.ID)
}

func (s *Service) writeNewSecret(ref, password string) error {
	if ref == "" {
		return nil
	}
	if s.secrets == nil {
		return ErrNoSecret
	}
	return s.secrets.Set(ref, password)
}

func (s *Service) buildEntry(draft Draft) (Entry, error) {
	scheme := proxyScheme(draft.Scheme)
	host := strings.TrimSpace(draft.Host)
	if host == "" || draft.Port < 1 || draft.Port > 65535 {
		return Entry{}, ErrInvalid
	}
	raw := proxyDraftURL(scheme, draft)
	normalized, err := config.NormalizeProxyURL(raw)
	if err != nil {
		return Entry{}, err
	}
	parsed, _ := url.Parse(normalized)
	port := parsed.Port()
	portNumber := 0
	_, err = fmt.Sscanf(port, "%d", &portNumber)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return Entry{}, ErrInvalid
	}
	label := strings.TrimSpace(draft.Label)
	if label == "" {
		label = net.JoinHostPort(parsed.Hostname(), port)
	}
	username := ""
	if parsed.User != nil {
		username = parsed.User.Username()
	}
	return Entry{
		Label: label, Scheme: parsed.Scheme, Host: parsed.Hostname(), Port: portNumber,
		Username: username, Enabled: draft.Enabled, Rank: max(draft.Rank, 0),
	}, nil
}

func proxyScheme(raw string) string {
	scheme := strings.ToLower(strings.TrimSpace(raw))
	if scheme == "" {
		return "http"
	}
	return scheme
}

func proxyDraftURL(scheme string, draft Draft) string {
	raw := scheme + "://"
	if draft.Username != "" || draft.Password != "" {
		raw += url.UserPassword(draft.Username, draft.Password).String() + "@"
	}
	return raw + net.JoinHostPort(draft.Host, fmt.Sprintf("%d", draft.Port))
}

func (s *Service) ensureUnique(ctx context.Context, entry Entry, exceptID string) error {
	rows, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.ID == exceptID {
			continue
		}
		if row.Scheme == entry.Scheme && strings.EqualFold(row.Host, entry.Host) &&
			row.Port == entry.Port && row.Username == entry.Username {
			return ErrDuplicate
		}
	}
	return nil
}

// Update changes one proxy. An empty password keeps the current one.
func (s *Service) Update(ctx context.Context, id string, patch Patch) (Item, error) {
	row, found, err := s.store.Get(ctx, id)
	if err != nil {
		return Item{}, err
	}
	if !found {
		return Item{}, ErrNotFound
	}
	applyProxyPatch(&row, patch)
	if err := s.validateProxyEntry(&row); err != nil {
		return Item{}, err
	}
	if err := s.ensureUnique(ctx, row, id); err != nil {
		return Item{}, err
	}
	if err := s.writeChangedSecret(&row, patch); err != nil {
		return Item{}, err
	}
	row.UpdatedAt = s.now().Unix()
	if err := s.store.Update(ctx, row); err != nil {
		return Item{}, err
	}
	s.ClearCache()
	return s.itemByID(ctx, id)
}

func applyProxyPatch(row *Entry, patch Patch) {
	if patch.Label != nil {
		row.Label = strings.TrimSpace(*patch.Label)
		if row.Label == "" {
			row.Label = net.JoinHostPort(row.Host, fmt.Sprintf("%d", row.Port))
		}
	}
	if patch.Scheme != nil {
		row.Scheme = strings.ToLower(strings.TrimSpace(*patch.Scheme))
	}
	if patch.Host != nil {
		row.Host = strings.TrimSpace(*patch.Host)
	}
	if patch.Port != nil {
		row.Port = *patch.Port
	}
	if patch.Username != nil {
		row.Username = strings.TrimSpace(*patch.Username)
	}
	if patch.Enabled != nil {
		row.Enabled = *patch.Enabled
	}
	if patch.Rank != nil {
		row.Rank = max(*patch.Rank, 0)
	}
}

func (s *Service) validateProxyEntry(row *Entry) error {
	normalized, err := config.NormalizeProxyURL(fmt.Sprintf("%s://%s", row.Scheme,
		net.JoinHostPort(row.Host, fmt.Sprintf("%d", row.Port))))
	if err != nil {
		return err
	}
	parsed, _ := url.Parse(normalized)
	row.Scheme, row.Host = parsed.Scheme, parsed.Hostname()
	return nil
}

func (s *Service) writeChangedSecret(row *Entry, patch Patch) error {
	if patch.Password != nil && *patch.Password != "" {
		if s.secrets == nil {
			return ErrNoSecret
		}
		if row.SecretRef == "" {
			row.SecretRef = "proxy/" + row.ID
		}
		if err := s.secrets.Set(row.SecretRef, *patch.Password); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes one proxy and its vault entry.
func (s *Service) Delete(ctx context.Context, id string) error {
	row, found, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}
	if row.SecretRef != "" && s.secrets != nil {
		_ = s.secrets.Delete(row.SecretRef)
	}
	s.ClearCache()
	s.mu.Lock()
	delete(s.health, id)
	s.mu.Unlock()
	return nil
}

// Import accepts one proxy per line, ignoring blank lines and # comments.
func (s *Service) Import(ctx context.Context, text string) ([]ImportResult, error) {
	results := make([]ImportResult, 0)
	for index, line := range strings.Split(text, "\n") {
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		result := ImportResult{Line: index + 1, Raw: raw}
		draft, err := DraftFromURL(raw)
		if err != nil {
			result.Status, result.Reason = "invalid", err.Error()
			results = append(results, result)
			continue
		}
		item, err := s.Create(ctx, draft)
		if errors.Is(err, ErrDuplicate) {
			result.Status, result.Reason = "duplicate", "proxy already exists"
		} else if err != nil {
			result.Status, result.Reason = "invalid", err.Error()
		} else {
			result.Status = "accepted"
			result.Item = &item
		}
		results = append(results, result)
	}
	return results, nil
}

// DraftFromURL parses one full proxy URL or shorthand into a draft.
func DraftFromURL(raw string) (Draft, error) {
	normalized, err := config.NormalizeProxyURL(raw)
	if err != nil || normalized == "" {
		return Draft{}, ErrInvalid
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return Draft{}, ErrInvalid
	}
	port := 0
	if _, err := fmt.Sscanf(parsed.Port(), "%d", &port); err != nil || port < 1 || port > 65535 {
		return Draft{}, ErrInvalid
	}
	username, password := "", ""
	if parsed.User != nil {
		username = parsed.User.Username()
		password, _ = parsed.User.Password()
	}
	return Draft{
		Label: net.JoinHostPort(parsed.Hostname(), parsed.Port()), Scheme: parsed.Scheme,
		Host: parsed.Hostname(), Port: port, Username: username, Password: password, Enabled: true,
	}, nil
}

// DraftFromEntry returns the editable shape of one stored entry, with the
// password omitted.
func DraftFromEntry(entry Entry) Draft {
	return Draft{
		Label: entry.Label, Scheme: entry.Scheme, Host: entry.Host, Port: entry.Port,
		Username: entry.Username, Enabled: entry.Enabled, Rank: entry.Rank,
	}
}

// Backfill moves a legacy [proxy] url into the pool once. The clear callback
// runs only after the entry and its secret are stored.
func (s *Service) Backfill(ctx context.Context, legacyURL string, clear func() error) (bool, error) {
	legacyURL = strings.TrimSpace(legacyURL)
	if legacyURL == "" {
		return false, nil
	}
	count, err := s.store.Count(ctx)
	if err != nil {
		return false, err
	}
	if count > 0 {
		return false, nil
	}
	draft, err := DraftFromURL(legacyURL)
	if err != nil {
		return false, err
	}
	if _, err := s.Create(ctx, draft); err != nil {
		return false, err
	}
	if clear != nil {
		if err := clear(); err != nil {
			return true, err
		}
	}
	return true, nil
}

func newID() string {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw)
}
