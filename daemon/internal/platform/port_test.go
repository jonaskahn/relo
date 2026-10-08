//go:build desktop

package platform

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/oauth"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	wireformats "github.com/jonaskahn/relo/internal/adapters/wire/formats"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/tests/testkit"
)

// TestFirstNonEmptyLabel covers the name a login gives the credential it
// produced: what the operator typed, else the account's own address, else a
// plain default. A blank value is not a label, so whitespace is skipped.
func TestFirstNonEmptyLabel(t *testing.T) {
	if got := firstNonEmptyLabel("work", "someone@example.com"); got != "work" {
		t.Errorf("firstNonEmptyLabel(typed) = %q, want the typed label", got)
	}
	if got := firstNonEmptyLabel("", "someone@example.com"); got != "someone@example.com" {
		t.Errorf("firstNonEmptyLabel(address) = %q, want the account's own address", got)
	}
	if got := firstNonEmptyLabel("", ""); got != defaultLabel {
		t.Errorf("firstNonEmptyLabel(none) = %q, want the default", got)
	}
	// Whitespace is not a label an operator typed, so it is skipped rather than
	// becoming a credential named after a blank.
	if got := firstNonEmptyLabel("   ", "someone@example.com"); got != "someone@example.com" {
		t.Errorf("firstNonEmptyLabel(blank) = %q, want the blank skipped", got)
	}
	if got := firstNonEmptyLabel("  work  ", "other"); got != "work" {
		t.Errorf("firstNonEmptyLabel(padded) = %q, want it trimmed", got)
	}
}

// TestQuotaProbeBaseTrimsTrailingSlashes covers the host a quota probe is asked
// at. A stored base URL with a trailing slash would otherwise produce a path
// with a doubled separator, which no vendor answers on.
func TestQuotaProbeBaseTrimsTrailingSlashes(t *testing.T) {
	for input, want := range map[string]string{
		"https://api.example.com":       "https://api.example.com",
		"https://api.example.com/":      "https://api.example.com",
		"https://api.example.com//":     "https://api.example.com/",
		"  https://api.example.com/v1 ": "https://api.example.com/v1",
		"":                              "",
	} {
		if got := quotaProbeBase(input); got != want {
			t.Errorf("quotaProbeBase(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestMeteredKeyHost covers which connections the background prober asks about
// a quota at all. The vendor hosts bill by key and publish a balance, so they
// are the ones a probe can read; an operator's own gateway is not.
func TestMeteredKeyHost(t *testing.T) {
	for _, baseURL := range []string{
		"https://openrouter.ai/api/v1",
		"https://api.z.ai/api/paas/v4",
		"https://api.deepseek.com",
		"https://api.moonshot.cn/v1",
		"https://api.siliconflow.cn/v1",
	} {
		if !meteredKeyHost(baseURL) {
			t.Errorf("meteredKeyHost(%q) = false, want the vendor host recognised", baseURL)
		}
	}

	for _, baseURL := range []string{
		"",
		"http://127.0.0.1:10101",
		"https://api.openai.com/v1",
		"https://generativelanguage.googleapis.com",
		"not a url",
	} {
		if meteredKeyHost(baseURL) {
			t.Errorf("meteredKeyHost(%q) = true, want a host that publishes no balance", baseURL)
		}
	}
}

// TestCredentialExtrasCarriesOnlyWhatWasAskedFor covers the extras a sign-in
// hands a probe beside the token. A provider that asked for nothing gets an
// empty map rather than keys with empty values, so a probe can tell the
// difference between "not asked" and "asked, and empty".
func TestCredentialExtrasCarriesOnlyWhatWasAskedFor(t *testing.T) {
	empty := credentialExtras(oauth.Credential{})
	if len(empty) != 0 {
		t.Errorf("credentialExtras(empty) = %v, want nothing", empty)
	}

	account := credentialExtras(oauth.Credential{
		Value: oauth.OAuthCredential{AccountID: "acct-1"},
	})
	if account["account_id"] != "acct-1" {
		t.Errorf("account_id = %q, want the account the token belongs to", account["account_id"])
	}
	if len(account) != 1 {
		t.Errorf("credentialExtras(account only) = %v, want only the account", account)
	}

	full := credentialExtras(oauth.Credential{
		Value: oauth.OAuthCredential{
			AccountID: "acct-1",
			Extra: map[string]string{
				oauth.ExtraProjectID:  "project-1",
				oauth.ExtraAPIBaseURL: "https://cloudcode-pa.googleapis.com",
				"something_else":      "ignored",
			},
		},
	})
	if full["projectId"] != "project-1" {
		t.Errorf("projectId = %q, want the project the quota bills", full["projectId"])
	}
	if full["api_server_url"] != "https://cloudcode-pa.googleapis.com" {
		t.Errorf("api_server_url = %q, want the base the probe reads", full["api_server_url"])
	}
	// An extra nobody asked for is not forwarded to a probe that would not
	// know what to do with it.
	if _, present := full["something_else"]; present {
		t.Errorf("credentialExtras() forwarded an extra it does not name: %v", full)
	}
}

// TestConnectionBaseURL covers the endpoint a quota probe reuses rather than
// assuming the vendor's default host. A connection the catalog does not have,
// or a catalog with no snapshot yet, has no endpoint to probe.
func TestConnectionBaseURL(t *testing.T) {
	// A catalog that has loaded nothing has no connection to probe, so there
	// is no endpoint to read and the probe is skipped rather than guessed at.
	empty := catalog.New(sqlite.NewCatalogReader(testkit.OpenTestDB(t)), nil, wireformats.New())
	if got := connectionBaseURL(empty, "openai"); got != "" {
		t.Errorf("connectionBaseURL(empty catalog) = %q, want empty", got)
	}
	// Nor is there one for a connection the catalog does not hold.
	if got := connectionBaseURL(empty, "a-connection-that-does-not-exist"); got != "" {
		t.Errorf("connectionBaseURL(unknown) = %q, want empty", got)
	}
}

// TestCatalogGlueNamesItsOwnCatalogue covers the registry lookups the
// composition root forwards to the template package. A retired template name
// is reported under the name that replaced it, so a stored connection still
// resolves after an upgrade.
func TestCatalogGlueNamesItsOwnCatalogue(t *testing.T) {
	registry := TemplateRegistry{}

	templates := registry.All(nil)
	if len(templates) == 0 {
		t.Fatal("All() returned no templates, want the registry")
	}

	curated, found := registry.Get(templates[0].ID, nil)
	if !found || curated.ID != templates[0].ID {
		t.Errorf("Get(%q) = %+v, found = %v, want the template itself", templates[0].ID, curated, found)
	}
	if _, found := registry.Get("a-template-that-does-not-exist", nil); found {
		t.Error("Get() found a template the registry does not hold")
	}

	handwritten, found := registry.Curated(templates[0].ID)
	if !found || handwritten.ID != templates[0].ID {
		t.Errorf("Curated(%q) = %+v, found = %v, want the curated template", templates[0].ID, handwritten, found)
	}

	// A label is only rewritten when it still matches a retired default, so
	// one an operator typed keeps whatever they typed.
	label, renamed := registry.RewrittenLabel("a-template-that-does-not-exist", "legacy")
	if renamed || label != "" {
		t.Errorf("RewrittenLabel(unknown) = %q, %v, want no rewrite", label, renamed)
	}
}

// TestAccountEdgesForwardWithoutAnAdapter covers the glue that hands one
// feature's port to another feature's adapter, and the pure decision it makes
// on its own: whether a wire format reads one roster per account.
func TestAccountEdgesForwardWithoutAnAdapter(t *testing.T) {
	edges := &AccountEdges{}

	// PerAccount is the one decision the edges make without reaching an
	// adapter, so it is answered with nothing wired at all.
	// Codex and Antigravity publish a roster that differs per account, so
	// their model list is read once for each rather than once per connection.
	if !edges.PerAccount(catalog.ModelsCodex) {
		t.Error("PerAccount(codex) = false, want a roster read per account")
	}
	if !edges.PerAccount(catalog.ModelsAntigravity) {
		t.Error("PerAccount(antigravity) = false, want a roster read per account")
	}
	// The common formats publish one roster for the connection.
	if edges.PerAccount(catalog.ModelsOpenAI) {
		t.Error("PerAccount(openai) = true, want one roster for the connection")
	}
	if edges.PerAccount(catalog.ModelsNone) {
		t.Error("PerAccount(none) = true for a format that publishes no roster")
	}
}

// TestQuotaStoreRoundTripsTheWindows covers the store the background prober
// writes what it read through: every field of a window survives the trip into
// SQLite and back, because the history ring and the current read both compare
// against what was stored.
func TestQuotaStoreRoundTripsTheWindows(t *testing.T) {
	ctx := context.Background()
	db := testkit.OpenTestDB(t)
	store := NewQuotaStore(db)

	gemDollars, claudeEuros := 12.5, 3.25
	snapshots := []activity.Snapshot{
		{
			CredentialID: "cred-1", Window: activity.WindowFiveHours, UsedPercent: 20,
			ResetAt: 1_700_000_000_000, Amount: &gemDollars, Currency: "USD",
			Seconds: 21_600, Source: "probe",
			UpdatedAt: time.UnixMilli(1_700_000_000_001),
		},
		{
			CredentialID: "cred-1", Window: activity.WindowSevenDays, UsedPercent: 75,
			ResetAt: 1_700_000_100_000, Amount: &claudeEuros, Currency: "EUR",
			Seconds: 604_800, Source: "probe",
			UpdatedAt: time.UnixMilli(1_700_000_000_002),
		},
		// A window with no amount is one a vendor reported a percentage for.
		{
			CredentialID: "cred-2", Window: activity.WindowFiveHours, UsedPercent: 0,
			Source: "probe", UpdatedAt: time.UnixMilli(1_700_000_000_003),
		},
	}
	if err := store.UpsertSnapshots(ctx, snapshots); err != nil {
		t.Fatalf("UpsertSnapshots() error = %v", err)
	}

	stored, err := store.ListSnapshots(ctx, "cred-1")
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("ListSnapshots() = %+v, want both windows of the credential", stored)
	}
	byWindow := map[string]activity.Snapshot{}
	for _, snapshot := range stored {
		byWindow[snapshot.Window] = snapshot
	}
	gem := byWindow[activity.WindowFiveHours]
	if gem.UsedPercent != 20 || gem.Amount == nil || *gem.Amount != gemDollars || gem.Currency != "USD" ||
		gem.ResetAt != 1_700_000_000_000 || gem.Seconds != 21_600 || gem.Source != "probe" {
		t.Errorf("five-hour window = %+v, want every field round-tripped", gem)
	}

	t.Run("a window with no amount stores none rather than zero", func(t *testing.T) {
		stored, err := store.ListSnapshots(ctx, "cred-2")
		if err != nil || len(stored) != 1 {
			t.Fatalf("ListSnapshots() = %+v, %v", stored, err)
		}
		if stored[0].Amount != nil || stored[0].Currency != "" {
			t.Fatalf("window = %+v, want no amount", stored[0])
		}
		if stored[0].UsedPercent != 0 || stored[0].Source != "probe" {
			t.Fatalf("window = %+v, want the share and source kept", stored[0])
		}
	})

	t.Run("a credential with no windows lists nothing", func(t *testing.T) {
		stored, err := store.ListSnapshots(ctx, "cred-none")
		if err != nil || len(stored) != 0 {
			t.Fatalf("ListSnapshots(unknown) = %+v, %v, want nothing", stored, err)
		}
	})

	t.Run("keeping and trimming the history ring", func(t *testing.T) {
		if err := store.AppendHistory(ctx, snapshots); err != nil {
			t.Fatalf("AppendHistory() error = %v", err)
		}
		bounds := activity.HistoryBounds{
			PerCredential: 2, Credentials: 4, Total: 100,
			MaxAge: 30 * 24 * time.Hour,
		}
		if err := store.TrimHistory(ctx, bounds); err != nil {
			t.Fatalf("TrimHistory() error = %v", err)
		}
	})
}
