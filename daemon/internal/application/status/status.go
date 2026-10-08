// Package status reports what an install can read about itself: health,
// quota windows, and the doctor checks the console and the command line share.
package status

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jonaskahn/relo/internal/access"
	pool "github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/activity"
	appaccess "github.com/jonaskahn/relo/internal/application/access"
	appaccount "github.com/jonaskahn/relo/internal/application/account"
	appsettings "github.com/jonaskahn/relo/internal/application/settings"
	"github.com/jonaskahn/relo/internal/catalog"
)

const (
	// CheckPass is a doctor check that found nothing wrong.
	CheckPass = "pass"
	// CheckWarn is a doctor check that found something an operator should see.
	CheckWarn = "warn"
	// CheckFail is a doctor check that found something broken.
	CheckFail = "fail"
	modeNone  = "none"
)

// Status is the daemon state the management API and the dashboard report.
type Status struct {
	SchemaVersion       int
	SecretMode          string
	Providers           int
	Models              int
	RoutableProviders   int
	ConfiguredProviders int
	Accounts            int
	PausedAccounts      int
	Groups              int
	LiveBytes           int64
	Retention           appsettings.RetentionSettings
	Quotas              []QuotaWindow
}

// QuotaWindow is one quota window of one credential.
type QuotaWindow struct {
	CredentialID string
	ProviderID   string
	Label        string
	Window       string
	UsedPercent  float64
	Amount       *float64
	Currency     string
	ResetAtMs    int64
	Seconds      int64
	Source       string
	ObservedAtMs int64
}

// DoctorCheck is one diagnostic result.
type DoctorCheck struct {
	Name   string
	Status string
	Detail string
}

// Database is the state file the doctor checks.
type Database interface {
	Health(ctx context.Context) error
	SchemaVersion() (int, error)
	Path() string
}

// Retention is the usage-log budget and the size of the rows it still holds.
type Retention interface {
	Budget(ctx context.Context) (activity.RetentionBudget, bool, error)
	LiveBytes(ctx context.Context) (int64, error)
}

// Secrets answers which store holds credentials and reads one of them.
type Secrets interface {
	Mode() string
	Get(ref string) (string, error)
}

// Quotas lists the stored windows of one credential.
type Quotas interface {
	ListSnapshots(ctx context.Context, credentialID string) ([]activity.Snapshot, error)
}

// Options configure the status use cases.
type Options struct {
	DB        Database
	Retention Retention
	Secrets   Secrets
	Entries   pool.CredentialRepository
	Quotas    Quotas
	Keys      *appaccess.Keys
	Accounts  *appaccount.Service
	Catalog   *catalog.Catalog
	Home      string
	// StartupLog is the boot transcript this run writes, which marks the
	// current start in the transcript list. Empty for a run with none.
	StartupLog string
}

// Service is the status and doctor use cases.
type Service struct {
	db         Database
	retention  Retention
	secrets    Secrets
	entries    pool.CredentialRepository
	quotas     Quotas
	keys       *appaccess.Keys
	accounts   *appaccount.Service
	catalog    *catalog.Catalog
	home       string
	startupLog string
}

// New returns the status use cases.
func New(options Options) *Service {
	return &Service{
		db: options.DB, retention: options.Retention, secrets: options.Secrets,
		entries: options.Entries, quotas: options.Quotas, keys: options.Keys,
		accounts: options.Accounts, catalog: options.Catalog, home: options.Home,
		startupLog: options.StartupLog,
	}
}

// Status reports what this install can read without touching the network.
func (s *Service) Status(ctx context.Context) (Status, error) {
	budget, _, err := s.retention.Budget(ctx)
	if err != nil {
		return Status{}, err
	}
	accounts, err := s.accounts.Accounts(ctx, "")
	if err != nil {
		return Status{}, err
	}
	report := baseStatus(budget, accounts, s.liveBytes(ctx))
	if report.SchemaVersion, err = s.db.SchemaVersion(); err != nil {
		return Status{}, err
	}
	if s.secrets != nil {
		report.SecretMode = s.secrets.Mode()
	}
	if snapshot, found := s.catalogSnapshot(); found {
		fillStatusCatalog(&report, snapshot)
	}
	if report.Quotas, err = s.Quotas(ctx); err != nil {
		return Status{}, err
	}
	return report, nil
}

func baseStatus(budget activity.RetentionBudget, accounts []appaccount.Account, liveBytes int64) Status {
	return Status{
		Retention: appsettings.RetentionSettings{
			UsageDays: budget.UsageDays, MaxEvents: budget.MaxEvents, MaxBytes: budget.MaxBytes,
		},
		Accounts:       len(accounts),
		PausedAccounts: pausedCount(accounts),
		SecretMode:     modeNone,
		LiveBytes:      liveBytes,
	}
}

func fillStatusCatalog(report *Status, snapshot *catalog.Snapshot) {
	providers, models, routable, configured := snapshot.Counts()
	report.Providers = providers
	report.Models = models
	report.RoutableProviders = routable
	report.ConfiguredProviders = configured
	report.Groups = len(snapshot.Groups)
}

// Quotas returns every stored quota window of every credential.
func (s *Service) Quotas(ctx context.Context) ([]QuotaWindow, error) {
	accounts, err := s.accounts.Accounts(ctx, "")
	if err != nil {
		return nil, err
	}
	windows := make([]QuotaWindow, 0, len(accounts))
	for _, item := range accounts {
		rows, err := s.quotas.ListSnapshots(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		windows = append(windows, toQuotaWindows(item, rows)...)
	}
	return windows, nil
}

// Doctor runs the data checks the dashboard and the command line report.
func (s *Service) Doctor(ctx context.Context) []DoctorCheck {
	return []DoctorCheck{
		s.checkDatabase(ctx),
		s.checkSecrets(ctx),
		s.checkPool(ctx),
		s.checkCatalog(ctx),
		s.checkAccessKeys(ctx),
		s.checkRetention(ctx),
	}
}

func (s *Service) checkAccessKeys(ctx context.Context) DoctorCheck {
	const name = "Client keys"
	keys, err := s.keys.AccessKeys(ctx)
	if err != nil {
		return DoctorCheck{Name: name, Status: CheckFail, Detail: err.Error()}
	}
	live := 0
	for _, key := range keys {
		if key.Status == access.StatusActive {
			live++
		}
	}
	note := s.legacyTokenNote()
	if live == 0 {
		return DoctorCheck{
			Name: name, Status: CheckWarn,
			Detail: "no active client key: inference is refused until one is created" + note,
		}
	}
	return DoctorCheck{
		Name: name, Status: CheckPass,
		Detail: fmt.Sprintf("%d of %d keys authenticate inference%s", live, len(keys), note),
	}
}

func (s *Service) legacyTokenNote() string {
	if s.home == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(s.home, access.LegacyTokenFile)); err != nil {
		return ""
	}
	return "; the legacy " + access.LegacyTokenFile + " file is ignored"
}

func (s *Service) checkDatabase(ctx context.Context) DoctorCheck {
	const name = "Database integrity"
	if err := s.db.Health(ctx); err != nil {
		return DoctorCheck{Name: name, Status: CheckFail, Detail: err.Error()}
	}
	return DoctorCheck{
		Name: name, Status: CheckPass,
		Detail: fmt.Sprintf("quick_check ok, %s live, %s in WAL", bytesOf(s.liveBytes(ctx)), bytesOf(s.walBytes())),
	}
}

func (s *Service) checkSecrets(ctx context.Context) DoctorCheck {
	const name = "Credential secrets"
	if s.secrets == nil {
		return DoctorCheck{Name: name, Status: CheckWarn, Detail: "no secret store is configured"}
	}
	rows, err := s.entries.List(ctx)
	if err != nil {
		return DoctorCheck{Name: name, Status: CheckFail, Detail: err.Error()}
	}
	missing := 0
	for _, row := range rows {
		if row.SecretRef == "" {
			continue
		}
		if _, err := s.secrets.Get(row.SecretRef); err != nil {
			missing++
		}
	}
	if missing > 0 {
		return DoctorCheck{
			Name: name, Status: CheckWarn,
			Detail: fmt.Sprintf("%d of %d credentials have no readable secret", missing, len(rows)),
		}
	}
	return DoctorCheck{
		Name: name, Status: CheckPass,
		Detail: fmt.Sprintf("%d secrets resolve in %s mode", len(rows), s.secrets.Mode()),
	}
}

func (s *Service) checkPool(ctx context.Context) DoctorCheck {
	const name = "Pool sanity"
	rows, err := s.entries.List(ctx)
	if err != nil {
		return DoctorCheck{Name: name, Status: CheckFail, Detail: err.Error()}
	}
	orphans := 0
	for _, row := range rows {
		if _, found := s.catalogProvider(row.ProviderID); !found {
			orphans++
		}
	}
	if orphans > 0 {
		return DoctorCheck{
			Name: name, Status: CheckWarn,
			Detail: fmt.Sprintf("%d credentials belong to providers this binary does not ship", orphans),
		}
	}
	return DoctorCheck{
		Name: name, Status: CheckPass,
		Detail: fmt.Sprintf("%d credentials across %d providers", len(rows), s.providerCount()),
	}
}

func (s *Service) checkCatalog(ctx context.Context) DoctorCheck {
	const name = "Model catalog"
	snapshot, found := s.catalogSnapshot()
	if !found {
		return DoctorCheck{Name: name, Status: CheckWarn, Detail: "no catalog is attached to this process"}
	}
	providers, models, routable, configured := snapshot.Counts()
	if providers == 0 {
		return DoctorCheck{Name: name, Status: CheckWarn, Detail: "no provider has been added yet"}
	}
	if models == 0 {
		return DoctorCheck{Name: name, Status: CheckFail, Detail: "the catalog holds no model"}
	}
	return DoctorCheck{
		Name: name, Status: CheckPass,
		Detail: fmt.Sprintf("%d models across %d providers (%d routable, %d configured)",
			models, providers, routable, configured),
	}
}

func (s *Service) checkRetention(ctx context.Context) DoctorCheck {
	const name = "Retention"
	config, stored, err := s.retention.Budget(ctx)
	if err != nil {
		return DoctorCheck{Name: name, Status: CheckFail, Detail: err.Error()}
	}
	if !stored {
		return DoctorCheck{
			Name: name, Status: CheckPass,
			Detail: "no budget is set: the usage log is kept in full",
		}
	}
	return DoctorCheck{
		Name: name, Status: CheckPass,
		Detail: fmt.Sprintf("%d days, %d events, %s", config.UsageDays, config.MaxEvents, bytesOf(config.MaxBytes)),
	}
}

func (s *Service) liveBytes(ctx context.Context) int64 {
	size, err := s.retention.LiveBytes(ctx)
	if err != nil {
		return 0
	}
	return size
}

func (s *Service) walBytes() int64 {
	if s.db == nil {
		return 0
	}
	info, err := os.Stat(s.db.Path() + "-wal")
	if err != nil {
		return 0
	}
	return info.Size()
}

func toQuotaWindows(item appaccount.Account, rows []activity.Snapshot) []QuotaWindow {
	windows := make([]QuotaWindow, 0, len(rows))
	for _, row := range rows {
		windows = append(windows, QuotaWindow{
			CredentialID: item.ID, ProviderID: item.ProviderID, Label: item.Label,
			Window: row.Window, UsedPercent: row.UsedPercent, ResetAtMs: row.ResetAt * 1000,
			Amount: row.Amount, Currency: row.Currency, Seconds: row.Seconds,
			Source: row.Source, ObservedAtMs: row.UpdatedAt.UnixMilli(),
		})
	}
	return windows
}

func pausedCount(accounts []appaccount.Account) int {
	paused := 0
	for _, item := range accounts {
		if item.Status != pool.StatusActive {
			paused++
		}
	}
	return paused
}

func bytesOf(count int64) string {
	if count < 1024 {
		return fmt.Sprintf("%d B", count)
	}
	units := []string{"KiB", "MiB", "GiB"}
	value := float64(count)
	unit := ""
	for _, next := range units {
		value /= 1024
		unit = next
		if value < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

func (s *Service) catalogSnapshot() (*catalog.Snapshot, bool) {
	if s.catalog == nil {
		return nil, false
	}
	return s.catalog.Snapshot()
}

func (s *Service) catalogProvider(id string) (catalog.Provider, bool) {
	snapshot, found := s.catalogSnapshot()
	if !found {
		return catalog.Provider{}, false
	}
	return snapshot.Provider(id)
}

func (s *Service) providerCount() int {
	snapshot, found := s.catalogSnapshot()
	if !found {
		return 0
	}
	return len(snapshot.Providers)
}
