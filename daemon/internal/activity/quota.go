// Package quota keeps the quota state of every credential: the current
// snapshot, a bounded history of observations, the probes that read both
// from the providers that publish them, and the worker that runs them.
package activity

import (
	"context"
	"errors"
	"time"
)

// WindowSample is one quota window a provider reported.
type WindowSample struct {
	Window      string
	UsedPercent float64
	Amount      *float64
	Currency    string
	ResetAt     int64
	// Seconds is how long the window lasts, and zero when the provider
	// reports only a percentage.
	Seconds int64
}

// Credential is what a probe needs to ask one account about its quota.
type Credential struct {
	ID          string
	ProviderID  string
	AccessToken string
	// RefreshToken is the second token a provider keeps, which the Copilot
	// quota endpoint asks for: the Copilot token authenticates inference while
	// the GitHub token behind it authenticates the usage call.
	RefreshToken string
	BaseURL      string
	Extra        map[string]string
}

// Snapshot is one stored quota window.
type Snapshot struct {
	CredentialID string
	Window       string
	UsedPercent  float64
	Amount       *float64
	Currency     string
	ResetAt      int64
	Seconds      int64
	Source       string
	UpdatedAt    time.Time
}

// The window names Relo shows for the limits the coding clients publish.
const (
	WindowFiveHours  = "5h"
	WindowSevenDays  = "7d"
	WindowThirtyDays = "30d"
)

// Headroom returns the remaining percentage of a window.
func (s Snapshot) Headroom() float64 {
	return 100 - s.UsedPercent
}

// QuotaProber reads the quota windows a provider publishes for one
// credential.
type QuotaProber interface {
	Probe(ctx context.Context, credential Credential) ([]WindowSample, error)
}

// Recorder stores the windows a provider reported, which is how a live
// response keeps the pool's view fresh without a probe.
type Recorder interface {
	Record(ctx context.Context, credentialID string, samples []WindowSample) error
}

// Freshness reports whether the stored windows are an older observation.
// Stale is true after a failed probe until a later probe or a live reading
// replaces it. NoteFresh records that replacement.
type Freshness interface {
	Stale(credentialID string) bool
	NoteFresh(credentialID string)
}

// QuotaRefresh reads quota again for one account or every account of one
// connection. A missing prober is reported. A failed probe leaves the last
// good snapshot and marks it stale.
type QuotaRefresh interface {
	ProbeCredential(ctx context.Context, credentialID string) ([]Snapshot, error)
	ProbeProvider(ctx context.Context, providerID string) ([]Snapshot, error)
}

// Repository persists quota snapshots and their history.
type Repository interface {
	UpsertSnapshots(ctx context.Context, snapshots []Snapshot) error
	ListSnapshots(ctx context.Context, credentialID string) ([]Snapshot, error)
	AppendHistory(ctx context.Context, snapshots []Snapshot) error
	TrimHistory(ctx context.Context, bounds HistoryBounds) error
}

// Quota errors name the refusals a probe maps to its own wording: no prover
// for this vendor, a missing token, and a vendor that refused or misanswered.
var (
	ErrNoProber      = errors.New("no quota prober is registered for this provider")
	ErrNoCredential  = errors.New("the quota probe needs an access token")
	ErrProbeRejected = errors.New("the provider refused the quota probe")
	ErrProbeResponse = errors.New("the quota response could not be read")
)
