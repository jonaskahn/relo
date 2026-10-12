// Listing vocabulary: targets, requests, and authorizations.
package catalog

import (
	"errors"
	"net/http"

	"github.com/jonaskahn/relo/internal/account"
)

var (
	// ErrListUnsupported reports a provider that publishes no model list
	// Relo reads.
	ErrListUnsupported = errors.New("the provider publishes no model list Relo reads")
	// ErrListRejected reports a listing the provider refused.
	ErrListRejected = errors.New("the provider refused the model list")
)

// Listed is one model a provider's listing returned, before the roster
// rules decide which of them a connection serves.
type Listed struct {
	ID            string
	Name          string
	ContextWindow *int64
	MaxOutput     *int64
	Prices        *Prices
	// IsFree is the provider's own mark that the entry needs no payment. It is
	// a pointer because a listing that never states the flag says nothing, and
	// a name that mentions no price proves neither way.
	IsFree *bool
	// SupportsTools, SupportsReasoning, and SupportsVision carry a
	// provider-declared capability. Nil means the listing said nothing.
	SupportsTools     *bool
	SupportsReasoning *bool
	SupportsVision    *bool
	// ReasoningEfforts are the effort values the listing stated the model
	// accepts. Nil means unstated; an explicitly empty list states none.
	ReasoningEfforts []string
	// ReasoningToggle reports that the listing stated an on/off thinking
	// switch. Nil means the listing said nothing about a toggle.
	ReasoningToggle *bool
	// RequestConfigs carries the exact per-account request tuples behind one
	// grouped model. Nil on providers whose listing needs no exact tuple.
	RequestConfigs []ModelRequestConfig
}

// ModelRequestConfig is one account's exact request tuple for one selection
// inside a grouped model: the shared identity the roster and selection use,
// plus the opaque wire payload only the owning codec understands.
type ModelRequestConfig struct {
	ModelID   string
	Selection string
	Effort    string
	// Default marks the member an empty effort request resolves to.
	Default   bool
	APIFormat APIFormat
	// WireConfig is the codec's exact request tuple as JSON. Its shape
	// belongs to the owning codec; storage never reads inside it.
	WireConfig string
	Context    int64
	Images     bool
	Reasoning  bool
}

// Cursor selection kinds name how one CursorVariant is chosen inside its
// group: the default, thinking off, thinking on without an effort, or one
// canonical effort.
const (
	CursorSelectDefault = "default"
	CursorSelectOff     = "off"
	CursorSelectToggle  = "toggle"
)

// CursorParam is one Cursor model parameter pair.
type CursorParam struct {
	ID    string
	Value string
}

// CursorVariant is one exact Cursor request configuration: the tuple the
// relay sends for one selection inside a group.
type CursorVariant struct {
	GroupID   string
	Selection string
	Effort    string
	// Default marks the member an empty effort request resolves to.
	Default   bool
	ModelID   string
	DetailsID string
	MaxMode   bool
	Params    []CursorParam
	Context   int64
	Images    bool
	Reasoning bool
}

// ListTarget names the endpoint and credential one listing runs against.
type ListTarget struct {
	Format  ModelsFormat
	BaseURL string
	Auth    Authorization
	Headers map[string]string
	// OpenCodeFree asks the lister to send the signed-out OpenCode CLI headers.
	OpenCodeFree bool
}

// AuthRequest names the provider one attempt is for.
type AuthRequest struct {
	ProviderID string
	Auth       Auth
	KeyHeader  KeyHeader
	Selection  account.Selection
	Region     string
	Service    string
}

// Authorization is one resolved credential: what to send, where, and what
// to call it in a log.
type Authorization struct {
	CredentialID string
	Label        string
	Auth         Auth
	KeyHeader    KeyHeader
	Token        string
	Headers      map[string]string
	BaseURL      string
	Project      string
	Signer       func(req *http.Request) error
	// AccountID is the subscription account the selected credential belongs
	// to. Codecs that attribute usage to an account read it here.
	AccountID string
}

// PerAccountRoster reports whether one connection's model list is the
// account's own entitlement rather than a roster every account shares. Only
// a dialect whose roster narrows per account is read once per account;
// anything else would send one listing per key to learn the same ids.
func PerAccountRoster(format ModelsFormat) bool {
	switch format {
	case ModelsCodex, ModelsAntigravity, ModelsCursor:
		return true
	default:
		return false
	}
}
