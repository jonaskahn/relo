// Callback broker: routing provider redirects to waiting logins.
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// The phases one login passes through, as the page watching it reports them.
const (
	// CallbackWaiting is a login the browser has not come back from yet.
	CallbackWaiting = "waiting"
	// CallbackExchanging is a login whose authorization code has arrived and
	// is being traded for tokens.
	CallbackExchanging = "exchanging"
	// CallbackConnected is a login that stored an account.
	CallbackConnected = "connected"
	// CallbackFailed is a login that did not finish.
	CallbackFailed = "failed"
)

// The failure categories the page names in prose, so a browser reads what
// happened without being shown anything about the grant.
const (
	CallbackDenied   = "denied"
	CallbackState    = "state"
	CallbackExchange = "exchange"
	CallbackTimeout  = "timeout"
)

// ErrCallbackExpired reports a provider redirect that matches no login still
// waiting for one: an unknown provider, or a state that was never handed out
// or has already been spent.
var ErrCallbackExpired = errors.New("the callback does not match a pending login")

const callbackTTL = 30 * time.Minute

// CallbackPagePath is the path the callback page answers on, with the
// provider named after it.
const CallbackPagePath = "/callback/"

// CallbackBrokerOptions tunes a broker.
type CallbackBrokerOptions struct {
	// ManagementPort is the port the callback page answers on: the listener
	// that serves the console, the management API, and the login callbacks.
	ManagementPort int
	// Now reads wall time, which a test drives by hand.
	Now func() time.Time
}

// CallbackBroker is the meeting point between a browser redirect and the
// login waiting for it. A flow registers the state it generated; the
// management listener serves the page the browser lands on, resolves the
// state against the pending login, and hands the code over. Neither side
// needs the other's address.
type CallbackBroker struct {
	mu       sync.Mutex
	port     int
	now      func() time.Time
	byState  map[string]*CallbackRegistration
	byTicket map[string]*CallbackRegistration
}

// NewCallbackBroker returns an empty broker for the management listener on
// the given port.
func NewCallbackBroker(options CallbackBrokerOptions) *CallbackBroker {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &CallbackBroker{
		port: options.ManagementPort, now: now,
		byState: map[string]*CallbackRegistration{}, byTicket: map[string]*CallbackRegistration{},
	}
}

// PageURL is where a browser watching one provider's login belongs: the
// callback page on the management listener.
func (b *CallbackBroker) PageURL(provider string) string {
	return "http://localhost:" + strconv.Itoa(b.port) + CallbackPagePath + url.PathEscape(provider)
}

// TicketURL is the page address one login is watched at. The ticket, never
// the grant, is what the browser ends up carrying.
func (b *CallbackBroker) TicketURL(provider, ticket string) string {
	return b.PageURL(provider) + "?" + url.Values{callbackTicketQuery: {ticket}}.Encode()
}

// Register opens one pending login for a provider and the state the provider
// must echo, and returns the registration a flow waits on.
func (b *CallbackBroker) Register(provider, state string) *CallbackRegistration {
	registration := &CallbackRegistration{
		provider: provider, state: state, ticket: newTicket(),
		created: b.now(), phase: CallbackWaiting, outcomes: make(chan callbackOutcome, 1),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked(b.now())
	b.byState[stateKey(provider, state)] = registration
	b.byTicket[registration.ticket] = registration
	return registration
}

// Deliver resolves one provider redirect against the login waiting for it,
// and returns the ticket the browser is watched with. The state names the
// login, so a redirect that names another provider's login, or one already
// spent, fails instead of delivering a code to the wrong flow. A ticket comes
// back whenever a login was claimed, even one the provider refused: the page
// it names is where the browser reads what happened.
func (b *CallbackBroker) Deliver(provider string, query url.Values) (string, error) {
	state := strings.TrimSpace(query.Get(callbackStateQuery))
	registration, found := b.claim(provider, state)
	if !found {
		return "", fmt.Errorf("provider %s: %w", provider, ErrCallbackExpired)
	}
	result, err := parseCallbackQuery(query, registration.state)
	if err != nil {
		registration.fail(CallbackCategory(err))
		return registration.ticket, err
	}
	registration.begin()
	registration.deliver(callbackOutcome{result: result})
	return registration.ticket, nil
}

func (b *CallbackBroker) claim(provider, state string) (*CallbackRegistration, bool) {
	if state == "" {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked(b.now())
	key := stateKey(provider, state)
	registration, found := b.byState[key]
	if !found || !registration.waiting() {
		return nil, false
	}
	delete(b.byState, key)
	return registration, true
}

// Status reports how far one login got, and whether the process still tracks
// it. It never carries the grant or the credential.
func (b *CallbackBroker) Status(ticket string) (CallbackStatus, bool) {
	if ticket == "" {
		return CallbackStatus{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pruneLocked(b.now())
	registration, found := b.byTicket[ticket]
	if !found {
		return CallbackStatus{}, false
	}
	return registration.status(), true
}

// Complete records the outcome of one login for the page watching it. A
// ticket the broker does not track is simply forgotten: the login it belongs
// to has already left the books.
func (b *CallbackBroker) Complete(ticket, account string, err error) {
	if ticket == "" {
		return
	}
	b.mu.Lock()
	registration, found := b.byTicket[ticket]
	b.mu.Unlock()
	if !found {
		return
	}
	if err != nil {
		registration.fail(CallbackCategory(err))
		return
	}
	registration.succeed(account)
}

func (b *CallbackBroker) pruneLocked(now time.Time) {
	for key, registration := range b.byState {
		if now.Sub(registration.created) > callbackTTL {
			delete(b.byState, key)
		}
	}
	for ticket, registration := range b.byTicket {
		if now.Sub(registration.created) > callbackTTL {
			delete(b.byTicket, ticket)
		}
	}
}

// CallbackStatus is what the page watching a login is told: how far it got,
// the account it stored, and the category of the failure.
type CallbackStatus struct {
	Provider string `json:"provider"`
	Phase    string `json:"phase"`
	Account  string `json:"account,omitempty"`
	Error    string `json:"error,omitempty"`
}

// CallbackRegistration is one login waiting for its authorization code. The
// page reads its phase; the flow waits on its code.
type CallbackRegistration struct {
	provider string
	state    string
	ticket   string
	created  time.Time
	outcomes chan callbackOutcome
	// abandoned marks a login whose flow stopped waiting for its browser. The
	// page reporting it is still on the broker's books, but a redirect that
	// arrives now has nothing to hand its code to: waking the page would leave
	// it reporting an exchange that never happens.
	abandoned atomic.Bool

	mu     sync.Mutex
	phase  string
	acct   string
	failed string
}

// Ticket returns the identifier the page watches this login with.
func (r *CallbackRegistration) Ticket() string { return r.ticket }

// Provider returns the provider this login belongs to.
func (r *CallbackRegistration) Provider() string { return r.provider }

func (r *CallbackRegistration) abandon() { r.abandoned.Store(true) }

func (r *CallbackRegistration) waiting() bool { return !r.abandoned.Load() }

// Wait returns the authorization code the browser delivered, or the reason
// the login did not finish.
func (r *CallbackRegistration) Wait(ctx context.Context) (string, error) {
	select {
	case outcome := <-r.outcomes:
		if outcome.err != nil {
			return "", outcome.err
		}
		return outcome.result.Code, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", ErrCallbackTimeout
		}
		return "", ErrLoginCancelled
	}
}

func (r *CallbackRegistration) begin() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phase = CallbackExchanging
}

func (r *CallbackRegistration) deliver(outcome callbackOutcome) {
	select {
	case r.outcomes <- outcome:
	default:
	}
}

func (r *CallbackRegistration) succeed(account string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phase, r.acct = CallbackConnected, account
}

func (r *CallbackRegistration) fail(category string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.phase, r.failed = CallbackFailed, category
	select {
	case r.outcomes <- callbackOutcome{err: callbackFailure(category)}:
	default:
	}
}

func (r *CallbackRegistration) status() CallbackStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return CallbackStatus{Provider: r.provider, Phase: r.phase, Account: r.acct, Error: r.failed}
}

func callbackFailure(category string) error {
	switch category {
	case CallbackDenied:
		return ErrLoginCancelled
	case CallbackState:
		return ErrInvalidState
	case CallbackTimeout:
		return ErrCallbackTimeout
	default:
		return ErrTokenExchange
	}
}

// CallbackCategory names a failure the page can describe without naming the
// grant, and reports an empty string for a failure that is not one.
func CallbackCategory(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrLoginCancelled), errors.Is(err, ErrDeviceDenied):
		return CallbackDenied
	case errors.Is(err, ErrInvalidState), errors.Is(err, ErrMissingCode), errors.Is(err, ErrCallbackExpired):
		return CallbackState
	case errors.Is(err, ErrCallbackTimeout):
		return CallbackTimeout
	default:
		return CallbackExchange
	}
}

func stateKey(provider, state string) string { return provider + "\x00" + state }

func newTicket() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	return hex.EncodeToString(raw)
}
