// Callback ports: the login handoff the platform injects.
package server

import (
	"net/url"
)

// CallbackOutcome names the phases and failure categories the callback page
// reads: a login that stored an account, one that did not finish, and the
// three refusals the page explains in prose.
type CallbackOutcome string

const (
	// CallbackConnected is a login that stored an account.
	CallbackConnected CallbackOutcome = "connected"
	// CallbackFailed is a login that did not finish.
	CallbackFailed CallbackOutcome = "failed"
	// CallbackDenied is a login the provider refused.
	CallbackDenied CallbackOutcome = "denied"
	// CallbackState is a login whose state did not match.
	CallbackState CallbackOutcome = "state"
	// CallbackTimeout is a login that ran out of time.
	CallbackTimeout CallbackOutcome = "timeout"
)

// CallbackStatus is what the page watching a login is told: how far it got,
// the account it stored, and the category of the failure.
type CallbackStatus struct {
	Provider string          `json:"provider"`
	Phase    CallbackOutcome `json:"phase"`
	Account  string          `json:"account,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// CallbackBroker routes a provider redirect to the login that started it.
// The platform builds the concrete broker once and hands it over, so this
// package never imports the OAuth adapter.
type CallbackBroker interface {
	Deliver(provider string, query url.Values) (string, error)
	Status(ticket string) (CallbackStatus, bool)
	Complete(ticket, account string, err error)
}

// CallbackPorts reports the loopback port one login flow listens on. A
// server built without one treats every port as free, which is what a build
// with no process table to ask does.
type CallbackPorts interface {
	CallbackPortFor(flow string) int
}
