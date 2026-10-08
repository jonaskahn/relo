// Package pool holds the credentials of every provider and picks one for
// each request, uniformly, whatever the provider is.
package account

import "errors"

// Credential states are the account lifecycle a pool manages: serving,
// held by the operator, or waiting on a fresh login.
const (
	StatusActive      = "active"
	StatusPaused      = "paused"
	StatusNeedsReauth = "needs_reauth"
)

// Pool errors name the refusals callers map to their own wording: no usable
// credential, a missing row, and a secret that cannot be read.
var (
	ErrNoCredentials = errors.New("no active credentials available for this provider")
	// ErrNoModelAccount reports a provider whose accounts are all in rotation
	// but none of which is known to serve the requested model, which is what
	// an account-level entitlement means.
	ErrNoModelAccount = errors.New("no account of this provider can serve the model")
	ErrNotFound       = errors.New("credential not found in pool")
	ErrNoSecretStore  = errors.New("no secret store is configured")
	ErrNoSecretRef    = errors.New("credential has no secret reference")
)

// ModelAccess answers which models one credential can serve. A pool without
// one allows every credential to serve every model.
type ModelAccess interface {
	CanServe(credentialID string, modelIDs []string) bool
}

// PoolEntry is one credential in a pool. It carries the reference to the
// secret, never the secret itself.
type PoolEntry struct {
	ID         string
	ProviderID string
	Kind       string
	Label      string
	SecretRef  string
	Status     string
	Priority   int
}

// Selection carries the per-request hints account selection needs, so a pool
// never reads a wire type to choose an account.
type Selection struct {
	ConversationID string
	Surface        string
	RequestID      string
	// Exclude names credentials this attempt must not pick, which is how a
	// retry inside one route reaches the next account of the same connection.
	// It is honoured while another account could serve the model; a pool left
	// with nothing but the accounts the caller named answers with one of them
	// rather than reporting that the provider has no account at all.
	Exclude []string
	// Models names the identifiers a request may reach the model under — the
	// catalog id, the upstream id, and the model a clone was copied from — so
	// an account whose roster lists any of them is eligible.
	Models []string
}
