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
}

// PerAccountRoster reports whether one connection's model list is the
// account's own entitlement rather than a roster every account shares. Only
// a dialect whose roster narrows per account is read once per account;
// anything else would send one listing per key to learn the same ids.
func PerAccountRoster(format ModelsFormat) bool {
	switch format {
	case ModelsCodex, ModelsAntigravity:
		return true
	default:
		return false
	}
}
