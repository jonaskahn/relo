package oauth_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/adapters/oauth"
)

// forgottenAfter is how long a test waits before a login is past the
// broker's lifetime, which is thirty minutes of wall time.
const forgottenAfter = 24 * time.Hour

// TestCallbackBroker covers the meeting point between a browser redirect and
// the login waiting for it.
func TestCallbackBroker(t *testing.T) {
	newBroker := func(now *time.Time) *oauth.CallbackBroker {
		return oauth.NewCallbackBroker(oauth.CallbackBrokerOptions{
			ManagementPort: 10101,
			Now:            func() time.Time { return *now },
		})
	}

	t.Run("the page address names the provider and the ticket", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		if got, want := broker.PageURL("claude"), "http://localhost:10101/callback/claude"; got != want {
			t.Fatalf("PageURL() = %q, want %q", got, want)
		}
		login := broker.Register("claude", "state-1")
		want := broker.PageURL("claude") + "?ticket=" + login.Ticket()
		if got := broker.TicketURL("claude", login.Ticket()); got != want {
			t.Fatalf("TicketURL() = %q, want %q", got, want)
		}
	})

	t.Run("a delivered grant reaches the login waiting for it", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		login := broker.Register("claude", "state-1")
		ticket, err := broker.Deliver("claude", url.Values{"code": {"code-1"}, "state": {"state-1"}})
		if err != nil {
			t.Fatalf("Deliver() error = %v", err)
		}
		if ticket != login.Ticket() {
			t.Fatalf("ticket = %q, want %q", ticket, login.Ticket())
		}
		code, err := login.Wait(t.Context())
		if err != nil || code != "code-1" {
			t.Fatalf("Wait() = %q, %v, want the delivered code", code, err)
		}
		status, found := broker.Status(ticket)
		if !found || status.Phase != oauth.CallbackExchanging {
			t.Fatalf("status = %+v, want the exchange", status)
		}
	})

	t.Run("a completed login reports its account", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		login := broker.Register("claude", "state-1")
		broker.Complete(login.Ticket(), "work@example.test", nil)
		status, found := broker.Status(login.Ticket())
		if !found || status.Phase != oauth.CallbackConnected || status.Account != "work@example.test" {
			t.Fatalf("status = %+v, want the stored account", status)
		}
	})

	t.Run("a failed login reports the category", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		login := broker.Register("claude", "state-1")
		broker.Complete(login.Ticket(), "", oauth.ErrTokenExchange)
		status, _ := broker.Status(login.Ticket())
		if status.Phase != oauth.CallbackFailed || status.Error != oauth.CallbackExchange {
			t.Fatalf("status = %+v, want the exchange failure", status)
		}
		broker.Complete(login.Ticket(), "", oauth.ErrCallbackTimeout)
		status, _ = broker.Status(login.Ticket())
		if status.Error != oauth.CallbackTimeout {
			t.Fatalf("failure = %q, want the timeout", status.Error)
		}
	})

	t.Run("a state is delivered once", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		broker.Register("claude", "state-1")
		if _, err := broker.Deliver("claude", url.Values{"code": {"c"}, "state": {"state-1"}}); err != nil {
			t.Fatalf("first Deliver() error = %v", err)
		}
		_, err := broker.Deliver("claude", url.Values{"code": {"c"}, "state": {"state-1"}})
		if !errors.Is(err, oauth.ErrCallbackExpired) {
			t.Fatalf("second Deliver() error = %v, want the state spent", err)
		}
	})

	t.Run("a state belongs to the provider that published it", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		broker.Register("claude", "state-1")
		_, err := broker.Deliver("cursor", url.Values{"code": {"c"}, "state": {"state-1"}})
		if !errors.Is(err, oauth.ErrCallbackExpired) {
			t.Fatalf("Deliver() error = %v, want another provider refused", err)
		}
	})

	t.Run("a provider that refused the login still names its page", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		login := broker.Register("claude", "state-1")
		ticket, err := broker.Deliver("claude", url.Values{"error": {"access_denied"}, "state": {"state-1"}})
		if !errors.Is(err, oauth.ErrLoginCancelled) {
			t.Fatalf("Deliver() error = %v, want the refusal", err)
		}
		if ticket != login.Ticket() {
			t.Fatalf("ticket = %q, want the login that was refused", ticket)
		}
		status, _ := broker.Status(ticket)
		if status.Phase != oauth.CallbackFailed || status.Error != oauth.CallbackDenied {
			t.Fatalf("status = %+v, want the denied failure", status)
		}
		if _, err := login.Wait(t.Context()); !errors.Is(err, oauth.ErrLoginCancelled) {
			t.Fatalf("Wait() error = %v, want the login released", err)
		}
	})

	t.Run("a login this process forgot is gone", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		login := broker.Register("claude", "state-1")
		now = now.Add(forgottenAfter)
		if _, found := broker.Status(login.Ticket()); found {
			t.Fatal("Status() found a login past its lifetime")
		}
		_, err := broker.Deliver("claude", url.Values{"code": {"c"}, "state": {"state-1"}})
		if !errors.Is(err, oauth.ErrCallbackExpired) {
			t.Fatalf("Deliver() error = %v, want the expired login refused", err)
		}
	})

	t.Run("an empty ticket names no login", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		if _, found := broker.Status(""); found {
			t.Fatal("Status with an empty ticket found a login")
		}
		broker.Complete("", "account", nil)
	})

	t.Run("a second login prunes the one no browser came back for", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		stale := broker.Register("claude", "state-1")
		now = now.Add(forgottenAfter)
		broker.Register("claude", "state-2")
		if _, found := broker.Status(stale.Ticket()); found {
			t.Fatal("the stale login outlived its lifetime")
		}
	})

	t.Run("a login reports the provider it was registered for", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		login := broker.Register("cursor", "state-1")
		if login.Provider() != "cursor" || login.Ticket() == "" {
			t.Fatalf("registration = %q %q, want the provider and a ticket", login.Provider(), login.Ticket())
		}
	})

	t.Run("a login gives up when its context ends", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		login := broker.Register("claude", "state-1")
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if _, err := login.Wait(ctx); !errors.Is(err, oauth.ErrCallbackTimeout) {
			t.Fatalf("Wait() error = %v, want the timeout", err)
		}
		cancelled, stop := context.WithCancel(context.Background())
		stop()
		if _, err := login.Wait(cancelled); !errors.Is(err, oauth.ErrLoginCancelled) {
			t.Fatalf("Wait() error = %v, want the login cancelled", err)
		}
	})

	t.Run("a completion for a login nobody registered is ignored", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0)
		broker := newBroker(&now)
		broker.Complete("nothing", "account", nil)
		if _, found := broker.Status("nothing"); found {
			t.Fatal("a completion invented a login")
		}
	})
}

// TestCallbackCategory covers the failure names the page describes without
// describing the grant.
func TestCallbackCategory(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{oauth.ErrLoginCancelled, oauth.CallbackDenied},
		{oauth.ErrDeviceDenied, oauth.CallbackDenied},
		{oauth.ErrInvalidState, oauth.CallbackState},
		{oauth.ErrMissingCode, oauth.CallbackState},
		{oauth.ErrCallbackExpired, oauth.CallbackState},
		{oauth.ErrCallbackTimeout, oauth.CallbackTimeout},
		{oauth.ErrTokenExchange, oauth.CallbackExchange},
	}
	for _, test := range cases {
		if got := oauth.CallbackCategory(test.err); got != test.want {
			t.Fatalf("CallbackCategory(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}
