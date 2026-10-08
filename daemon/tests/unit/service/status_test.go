package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	approuting "github.com/jonaskahn/relo/internal/application/routing"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	appstatus "github.com/jonaskahn/relo/internal/application/status"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/platform"
)

func TestStatus(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	account := harness.addAccount("openai", "work", "sk-status")
	store := platform.NewQuotaStore(harness.db)
	snapshot := activity.Snapshot{
		CredentialID: account.ID, Window: "5h", UsedPercent: 40,
		// A stored window keeps its reset moment in Unix seconds, the way every
		// prober reports it, and the console reads milliseconds.
		ResetAt: 1_800_000_000, Source: "probe", UpdatedAt: harness.clock.Now(),
	}
	if err := store.UpsertSnapshots(ctx, []activity.Snapshot{snapshot}); err != nil {
		t.Fatalf("UpsertSnapshots() error = %v", err)
	}

	status, err := harness.status.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.SchemaVersion != sqlite.LatestSchemaVersion() {
		t.Fatalf("SchemaVersion = %d, want %d", status.SchemaVersion, sqlite.LatestSchemaVersion())
	}
	if status.Accounts != 1 || status.Providers == 0 || status.Models == 0 {
		t.Fatalf("Status() = %+v, want the install counts", status)
	}
	if status.LiveBytes <= 0 {
		t.Fatalf("LiveBytes = %d, want a positive size", status.LiveBytes)
	}
	if len(status.Quotas) != 1 {
		t.Fatalf("Quotas = %+v, want the stored window", status.Quotas)
	}
	window := status.Quotas[0]
	if window.CredentialID != account.ID || window.ProviderID != "openai" || window.UsedPercent != 40 {
		t.Fatalf("quota window = %+v, want the stored snapshot", window)
	}
	if window.ResetAtMs != 1_800_000_000_000 {
		t.Fatalf("ResetAtMs = %d, want the stored seconds as milliseconds", window.ResetAtMs)
	}
}

func TestQuotaStoreAdapter(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	store := platform.NewQuotaStore(harness.db)
	snapshot := activity.Snapshot{
		CredentialID: "credential-one", Window: "week", UsedPercent: 12.5,
		ResetAt: 5, Source: "probe", UpdatedAt: time.Unix(1_700_000_000, 0),
	}
	if err := store.UpsertSnapshots(ctx, []activity.Snapshot{snapshot}); err != nil {
		t.Fatalf("UpsertSnapshots() error = %v", err)
	}
	read, err := store.ListSnapshots(ctx, "credential-one")
	if err != nil {
		t.Fatalf("ListSnapshots() error = %v", err)
	}
	if len(read) != 1 || read[0].UsedPercent != 12.5 || read[0].Window != "week" {
		t.Fatalf("ListSnapshots() = %+v, want the stored window", read)
	}
	if err := store.AppendHistory(ctx, []activity.Snapshot{snapshot}); err != nil {
		t.Fatalf("AppendHistory() error = %v", err)
	}
	if err := store.TrimHistory(ctx, activity.DefaultHistoryBounds()); err != nil {
		t.Fatalf("TrimHistory() error = %v", err)
	}
}

func TestDoctor(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	checks := harness.status.Doctor(ctx)
	if len(checks) == 0 {
		t.Fatal("Doctor() returned no check")
	}
	byName := map[string]appstatus.DoctorCheck{}
	for _, check := range checks {
		if check.Name == "" || check.Status == "" {
			t.Fatalf("Doctor() returned an empty check: %+v", check)
		}
		byName[check.Name] = check
	}
	if check := byName["Database integrity"]; check.Status != appstatus.CheckPass {
		t.Fatalf("Database integrity = %+v, want a pass", check)
	}
	if check := byName["Retention"]; check.Status != appstatus.CheckPass {
		t.Fatalf("Retention = %+v, want a pass: an unset budget is the default", check)
	}
	if check := byName["Credential secrets"]; check.Status != appstatus.CheckPass {
		t.Fatalf("Credential secrets = %+v, want a pass", check)
	}
	if check := byName["Pool sanity"]; check.Status != appstatus.CheckPass {
		t.Fatalf("Pool sanity = %+v, want a pass", check)
	}
	if check := byName["Model catalog"]; check.Status != appstatus.CheckPass {
		t.Fatalf("Model catalog = %+v, want a pass", check)
	}

	t.Run("a set budget passes", func(t *testing.T) {
		if _, err := harness.settings.SaveRetention(ctx, appsettings.RetentionSettings{
			UsageDays: 30, MaxEvents: 1000,
		}); err != nil {
			t.Fatalf("SaveRetention() error = %v", err)
		}
		report := doctorByName(t, harness.status.Doctor(ctx))["Retention"]
		if report.Status != appstatus.CheckPass {
			t.Fatalf("Retention = %+v, want a pass", report)
		}
	})

	t.Run("a credential for a provider nothing ships is refused", func(t *testing.T) {
		repo := sqlite.NewCredentialRepo(harness.db)
		err := repo.Insert(ctx, sqlite.CredentialRow{
			ID: "orphan", ProviderID: "provider-from-an-older-build", Kind: "api_key", Status: "active",
		})
		if err == nil {
			t.Fatal("Insert() = nil, want the foreign key to refuse a credential nothing owns")
		}
		report := doctorByName(t, harness.status.Doctor(ctx))["Pool sanity"]
		if report.Status != appstatus.CheckPass {
			t.Fatalf("Pool sanity = %+v, want a pass", report)
		}
	})

	t.Run("a detached process warns", func(t *testing.T) {
		checks := harness.withoutCatalog().status.Doctor(ctx)
		if report := doctorByName(t, checks)["Model catalog"]; report.Status != appstatus.CheckWarn {
			t.Fatalf("Model catalog = %+v, want a warning without a catalog", report)
		}
	})
}

func TestDoctorWithoutASecretStore(t *testing.T) {
	harness := newHarness(t)
	detached := harness.withoutSecrets()
	report := doctorByName(t, detached.status.Doctor(context.Background()))["Credential secrets"]
	if report.Status != appstatus.CheckWarn {
		t.Fatalf("Credential secrets = %+v, want a warning", report)
	}
}

func doctorByName(t *testing.T, checks []appstatus.DoctorCheck) map[string]appstatus.DoctorCheck {
	t.Helper()
	byName := make(map[string]appstatus.DoctorCheck, len(checks))
	for _, check := range checks {
		byName[check.Name] = check
	}
	return byName
}

func TestStatusCountsPausedAccountsAndGroups(t *testing.T) {
	harness := newHarness(t)
	ctx := context.Background()
	harness.seedProvider("openai", "OpenAI", string(catalog.AuthAPIKey))
	account := harness.addAccount("openai", "work", "sk-paused")
	if err := harness.accounts.PauseAccount(ctx, account.ID); err != nil {
		t.Fatalf("PauseAccount() error = %v", err)
	}
	if err := harness.routes.Save(ctx, "fast", approuting.Write{
		Strategy: "priority", Enabled: true, Listed: true,
		Members: []approuting.MemberWrite{{ProviderID: "openai", ModelID: "model-1", Weight: 1, Enabled: true}},
	}); err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	status, err := harness.status.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.PausedAccounts != 1 {
		t.Fatalf("PausedAccounts = %d, want 1", status.PausedAccounts)
	}
	if status.Groups != 1 {
		t.Fatalf("Groups = %d, want 1", status.Groups)
	}
}
