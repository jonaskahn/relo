// Catalog records: connections, models, and facts.
package catalog

import (
	"context"
	"errors"
)

var (
	// ErrProviderNotFound reports a connection id that matches no stored row.
	ErrProviderNotFound = errors.New("provider not found")
	// ErrModelNotFound reports a model no stored row holds.
	ErrModelNotFound = errors.New("model not found")
	// ErrConflict reports a catalog row a route still references, so it
	// cannot be removed.
	ErrConflict = errors.New("the catalog row is in use")
)

// Reader is the catalog's read port. The storage adapter maps its own rows to
// these records, so a catalog rule never sees a database row and this package
// never depends on a storage implementation.
type Reader interface {
	ListConnections(ctx context.Context) ([]ConnectionRecord, error)
	ListModels(ctx context.Context) ([]ModelRecord, error)
	ListModelFacts(ctx context.Context) ([]FactsRecord, error)
	ListRoutes(ctx context.Context) ([]RouteRecord, error)
}

// ConnectionRecord is one stored connection as the catalog reads it.
type ConnectionRecord struct {
	ID                  string
	TemplateID          string
	Origin              string
	Label               string
	Auth                string
	APIFormat           string
	KeyHeader           string
	BaseURL             string
	ModelsSource        string
	ModelsFormat        string
	ModelsDevProviderID string
	Headers             map[string]string
	DocURL              string
	KeyEnv              []string
	LoginFlows          []string
	Variables           map[string]string
	Enabled             bool
	Rank                int
	PoolStrategy        string
	LastRefreshedAtMs   *int64
	LastRefreshError    string
	CreatedAtMs         int64
	UpdatedAtMs         int64
	UseProxy            bool
	// TimeoutSeconds is this connection's own call wait in seconds; nil
	// inherits the global value.
	TimeoutSeconds *int
	// RetryBackoff is this connection's own retry windows; nil inherits the
	// global windows.
	RetryBackoff [][2]int
	// SwitchOn4xx and SwitchOn5xx fail this connection over on a status the
	// relay does not always retry; nil is the shipped default, which does.
	SwitchOn4xx *bool
	SwitchOn5xx *bool
}

// ModelRecord is one stored model.
type ModelRecord struct {
	ProviderID   string
	ModelID      string
	Source       string
	APIFormat    string
	BaseURL      string
	ModelsDevRef string
	Match        string
	Enabled      bool
	Available    *bool
	ListedAtMs   *int64
	UpdatedAtMs  int64
	// UpstreamModelID is the identifier Relo sends to the provider, which a
	// clone sets to reach a model the provider has not listed yet. ClonedFrom
	// names the model a clone was copied from, empty on every other row.
	UpstreamModelID string
	ClonedFrom      string
}

// FactsRecord is one stored layer of model details.
type FactsRecord struct {
	ProviderID        string
	ModelID           string
	Layer             string
	Name              *string
	Description       *string
	Family            *string
	Category          *string
	ContextWindow     *int64
	MaxInput          *int64
	MaxOutput         *int64
	SupportsTools     *bool
	SupportsReasoning *bool
	SupportsVision    *bool
	// ReasoningEfforts are the effort values the layer states the model
	// accepts, or nil when it states none. An empty slice means the layer
	// stated no effort list.
	ReasoningEfforts []string
	// ReasoningToggle reports that the layer stated an on/off thinking
	// switch. Nil means the layer said nothing about a toggle.
	ReasoningToggle *bool
	// ReasoningBudget reports that the layer stated a token budget. Nil
	// means the layer said nothing about a budget. The bounds are nil when
	// a budget was stated without a limit.
	ReasoningBudget    *bool
	ReasoningBudgetMin *int64
	ReasoningBudgetMax *int64
	Status             *string
	ReleaseDate        *string
	Prices             PriceRecord
}

// PriceRecord is one stored set of rates in integer USD micros per million
// tokens.
type PriceRecord struct {
	Input         *int64
	Output        *int64
	CacheRead     *int64
	CacheWrite    *int64
	ExtThreshold  *int64
	ExtInput      *int64
	ExtOutput     *int64
	ExtCacheRead  *int64
	ExtCacheWrite *int64
}

// Known reports whether any rate is stated. A layer that states none is
// silent rather than free, which is what keeps a nil rate distinct from a
// rate of zero.
func (p PriceRecord) Known() bool {
	return p.Input != nil || p.Output != nil
}

// Empty reports whether every field of one layer is unset, which is what
// tells a cleared override from one that still holds a value.
func (f FactsRecord) Empty() bool {
	return f.Name == nil && f.Description == nil && f.Family == nil && f.Category == nil &&
		f.ContextWindow == nil && f.MaxInput == nil && f.MaxOutput == nil &&
		f.SupportsTools == nil && f.SupportsReasoning == nil && f.SupportsVision == nil &&
		f.ReasoningToggle == nil && f.ReasoningBudget == nil &&
		f.ReasoningBudgetMin == nil && f.ReasoningBudgetMax == nil &&
		f.Status == nil && f.ReleaseDate == nil && !f.Prices.Known()
}

// RouteRecord is one stored route with its ordered members.
type RouteRecord struct {
	ID          string
	Label       string
	Strategy    string
	Enabled     bool
	Listed      bool
	Members     []RouteMemberRecord
	CreatedAtMs int64
	UpdatedAtMs int64
	// SwitchOn4xx and SwitchOn5xx move this route to its next member on a
	// status the relay does not always retry; nil is the shipped default,
	// which does.
	SwitchOn4xx *bool
	SwitchOn5xx *bool
}

// RouteMemberRecord is one member of a route.
type RouteMemberRecord struct {
	Position   int
	ProviderID string
	ModelID    string
	Kind       string
	Weight     int
	Enabled    bool
}

// ModelsDevStateRecord is the last fetch state of the models.dev copy as it
// is stored, which a page reports beside the live state.
type ModelsDevStateRecord struct {
	SourceURL       string
	FetchedAtMs     int64
	ETag            string
	LastModified    string
	ProvidersCount  int
	ModelsCount     int
	LastAttemptAtMs int64
	LastError       string
}
