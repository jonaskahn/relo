// Desktop activity source: the last day's usage for the tray menu.
package cli

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/jonaskahn/relo/internal/activity"
	"github.com/jonaskahn/relo/internal/adapters/sqlite"
	"github.com/jonaskahn/relo/internal/adapters/templates"
	"github.com/jonaskahn/relo/internal/adapters/wire"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
	"github.com/jonaskahn/relo/internal/desktop"
)

var _ desktop.ActivitySource = desktopActivitySource{}

type desktopActivitySource struct {
	home   string
	logger *slog.Logger
}

// NewDesktopActivitySource returns the tray's usage reader over the state
// database: the last day's rollup with plan-covered spend excluded. The serve
// command injects it into the tray, so the tray never opens the database
// itself.
func NewDesktopActivitySource(home string, logger *slog.Logger) desktop.ActivitySource {
	return desktopActivitySource{home: home, logger: logger}
}

// Snapshot reads the last day of usage for the tray menu. Spend follows the
// console's own rule: only pay-as-you-go connections count, so a plan's
// traffic never reads as money.
func (s desktopActivitySource) Snapshot(ctx context.Context, sinceMs int64) (desktop.ActivitySnapshot, error) {
	db, err := sqlite.OpenReadOnly(config.DatabasePath(s.home), s.logger)
	if err != nil {
		return desktop.ActivitySnapshot{}, err
	}
	defer func() { _ = db.Close() }()

	current, err := sqlite.NewUsageQuery(db).Rollup(ctx, activity.RollupQuery{
		Filter: activity.UsageFilter{SinceMs: sinceMs}, GroupBy: activity.GroupByProvider,
	})
	if err != nil {
		return desktop.ActivitySnapshot{}, err
	}
	connections, err := sqlite.NewCatalogRepo(db).ListProviders(ctx)
	if err != nil {
		return desktop.ActivitySnapshot{}, err
	}
	skip := map[string]bool{}
	for _, connection := range connections {
		if !paidConnection(connection) {
			skip[connection.ID] = true
		}
	}
	return desktop.ActivitySnapshot{Rows: usageRows(current), UnpaidIDs: skip}, nil
}

func usageRows(rows []activity.UsageRollupRow) []desktop.UsageRow {
	result := make([]desktop.UsageRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, desktop.UsageRow{
			Key: row.Key, Requests: float64(row.Requests), Errors: float64(row.Errors),
			InputTokens: float64(row.InputTokens), OutputTokens: float64(row.OutputTokens),
			CacheReadTokens: float64(row.CacheReadTokens), CacheWriteTokens: float64(row.CacheWriteTokens),
			CostMicros: float64(row.CostMicros), DurationMs: float64(row.DurationMs),
		})
	}
	return result
}

func planConnection(row sqlite.ProviderRow) bool {
	return row.Origin == string(catalog.OriginSignIn) || tokenPlanConnection(row) ||
		openCodeConnection(row) || row.TemplateID == catalog.KiloFreeTemplate
}

func paidConnection(row sqlite.ProviderRow) bool {
	if planConnection(row) {
		return false
	}
	kind := connectionKind(row)
	return kind == catalog.KindKey || kind == catalog.KindCloud
}

func connectionKind(row sqlite.ProviderRow) catalog.Kind {
	if template, found := templates.Curated(row.TemplateID); found && template.Kind != "" {
		return template.Kind
	}
	if row.Origin == string(catalog.OriginCustom) {
		return catalog.KindLocal
	}
	return catalog.KindKey
}

func tokenPlanConnection(row sqlite.ProviderRow) bool {
	for _, id := range []string{row.ID, row.TemplateID, row.ModelsDevProviderID} {
		if strings.Contains(strings.ToLower(id), "token-plan") {
			return true
		}
	}
	parsed, err := url.Parse(strings.TrimSpace(row.BaseURL))
	return err == nil && strings.Contains(strings.ToLower(parsed.Hostname()), "token-plan")
}

func openCodeConnection(row sqlite.ProviderRow) bool {
	for _, id := range []string{row.TemplateID, row.ModelsDevProviderID} {
		if id == "opencode-go" || id == "opencode-free" || id == "opencode" {
			return true
		}
	}
	return wire.OpenCodeGo(row.BaseURL) || wire.OpenCodeFree(row.BaseURL)
}
