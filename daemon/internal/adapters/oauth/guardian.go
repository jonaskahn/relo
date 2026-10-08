// Credential guardian: refresh policy for stored OAuth credentials.
package oauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/jonaskahn/relo/internal/clock"
)

// Credential pairs one stored account with the tokens a flow refreshes
// on its behalf.
type Credential struct {
	ID          string
	ProviderID  string
	Value       OAuthCredential
	NeedsReauth bool
}

// CredentialStore persists what the guardian refreshes and what it
// concludes about a credential.
type CredentialStore interface {
	List(ctx context.Context) ([]Credential, error)
	Save(ctx context.Context, credential Credential) error
	MarkNeedsReauth(ctx context.Context, id string) error
}

// FlowLookup resolves provider identifiers to the flows Relo ships.
type FlowLookup interface {
	Flow(providerID string) (OAuthFlow, error)
}

// Guardian refreshes stored credentials before they expire, so no
// request waits for a refresh and no idle account ages out server side.
type Guardian struct {
	flows    FlowLookup
	store    CredentialStore
	clock    clock.Clock
	margin   time.Duration
	logger   *slog.Logger
	policy   RefreshPolicy
	mu       sync.Mutex
	inflight map[string]bool
}

// RefreshPolicy reports whether one provider may renew an access token on
// its own. A nil policy allows every provider to refresh.
type RefreshPolicy interface {
	AutoRefresh(ctx context.Context, providerID string) bool
}

// GuardianOptions tunes the guardian.
type GuardianOptions struct {
	Clock  clock.Clock
	Margin time.Duration
	Logger *slog.Logger
	Policy RefreshPolicy
}

// NewGuardian returns a guardian that refreshes every credential
// expiring inside the margin, which defaults to five minutes.
func NewGuardian(flows FlowLookup, store CredentialStore, options GuardianOptions) *Guardian {
	guardian := &Guardian{
		flows:    flows,
		store:    store,
		clock:    options.Clock,
		margin:   options.Margin,
		logger:   options.Logger,
		policy:   options.Policy,
		inflight: map[string]bool{},
	}
	if guardian.clock == nil {
		guardian.clock = clock.New()
	}
	if guardian.margin <= 0 {
		guardian.margin = defaultRefreshMargin
	}
	if guardian.logger == nil {
		guardian.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return guardian
}

// RefreshDue refreshes every credential that expires inside the margin
// and returns the identifiers it refreshed. A single account that cannot
// refresh never stops the sweep.
func (g *Guardian) RefreshDue(ctx context.Context) ([]string, error) {
	credentials, err := g.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credentials to refresh: %w", err)
	}
	refreshed := make([]string, 0, len(credentials))
	for _, credential := range credentials {
		if !g.due(ctx, credential) {
			continue
		}
		if err := g.refreshOne(ctx, credential); err != nil {
			continue
		}
		refreshed = append(refreshed, credential.ID)
	}
	return refreshed, nil
}

// Run refreshes due credentials on every tick until the context ends.
func (g *Guardian) Run(ctx context.Context, interval time.Duration) {
	ticker := g.clock.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C():
			if _, err := g.RefreshDue(ctx); err != nil {
				g.logger.Warn("refresh sweep failed", "error", err)
			}
		}
	}
}

func (g *Guardian) due(ctx context.Context, credential Credential) bool {
	if credential.NeedsReauth || !expires(&credential.Value, g.clock.Now(), g.margin) {
		return false
	}
	if g.policy != nil && !g.policy.AutoRefresh(ctx, credential.ProviderID) {
		return false
	}
	return true
}

func (g *Guardian) refreshOne(ctx context.Context, credential Credential) error {
	if !g.begin(credential.ID) {
		return nil
	}
	defer g.end(credential.ID)
	flow, err := g.flows.Flow(credential.ProviderID)
	if err != nil {
		g.logger.Warn("no flow to refresh with", "provider", credential.ProviderID, "error", err)
		return err
	}
	refreshed, err := flow.Refresh(ctx, &credential.Value)
	if err != nil {
		return g.recordFailure(ctx, credential, err)
	}
	credential.Value = *refreshed
	credential.NeedsReauth = false
	return g.store.Save(ctx, credential)
}

func (g *Guardian) recordFailure(ctx context.Context, credential Credential, refreshErr error) error {
	g.logger.Warn("credential refresh failed", "credential", credential.ID, "provider", credential.ProviderID, "error", refreshErr)
	if !errors.Is(refreshErr, ErrRefreshRejected) {
		return refreshErr
	}
	if g.peerRotated(ctx, credential) {
		return refreshErr
	}
	if err := g.store.MarkNeedsReauth(ctx, credential.ID); err != nil {
		return fmt.Errorf("mark credential %s for a new login: %w", credential.ID, err)
	}
	return refreshErr
}

func (g *Guardian) peerRotated(ctx context.Context, attempted Credential) bool {
	current, err := g.store.List(ctx)
	if err != nil {
		return false
	}
	for _, row := range current {
		if row.ID != attempted.ID {
			continue
		}
		return row.Value.RefreshToken != attempted.Value.RefreshToken
	}
	return false
}

func (g *Guardian) begin(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inflight[id] {
		return false
	}
	g.inflight[id] = true
	return true
}

func (g *Guardian) end(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inflight, id)
}
