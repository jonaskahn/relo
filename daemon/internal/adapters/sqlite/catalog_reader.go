// Catalog reader: the connection and model rows routing resolves.
package sqlite

import (
	"context"

	"github.com/jonaskahn/relo/internal/catalog"
)

// CatalogReader adapts the catalog tables to the catalog's read port, so the
// catalog package never sees a row of this package's own types.
type CatalogReader struct {
	repo *CatalogRepo
}

// NewCatalogReader returns a catalog reader over the given database.
func NewCatalogReader(db *DB) *CatalogReader {
	return &CatalogReader{repo: NewCatalogRepo(db)}
}

var _ catalog.Reader = (*CatalogReader)(nil)

// ListConnections returns every stored connection.
func (r *CatalogReader) ListConnections(ctx context.Context) ([]catalog.ConnectionRecord, error) {
	rows, err := r.repo.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]catalog.ConnectionRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, connectionRecord(row))
	}
	return records, nil
}

// ListModels returns every stored model.
func (r *CatalogReader) ListModels(ctx context.Context) ([]catalog.ModelRecord, error) {
	rows, err := r.repo.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]catalog.ModelRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, modelRecord(row))
	}
	return records, nil
}

// ListModelFacts returns every stored layer of model details.
func (r *CatalogReader) ListModelFacts(ctx context.Context) ([]catalog.FactsRecord, error) {
	rows, err := r.repo.ListAllModelFacts(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]catalog.FactsRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, factsRecord(row))
	}
	return records, nil
}

// ListRoutes returns every stored route with its ordered members.
func (r *CatalogReader) ListRoutes(ctx context.Context) ([]catalog.RouteRecord, error) {
	rows, err := r.repo.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]catalog.RouteRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, routeRecord(row))
	}
	return records, nil
}

func connectionRecord(row ProviderRow) catalog.ConnectionRecord {
	return catalog.ConnectionRecord(row)
}

func modelRecord(row ModelRow) catalog.ModelRecord {
	return catalog.ModelRecord(row)
}

func factsRecord(row ModelFactsRow) catalog.FactsRecord {
	return catalog.FactsRecord{
		ProviderID: row.ProviderID, ModelID: row.ModelID, Layer: row.Layer,
		Name: row.Name, Description: row.Description, Family: row.Family, Category: row.Category,
		ContextWindow: row.ContextWindow, MaxInput: row.MaxInput, MaxOutput: row.MaxOutput,
		SupportsTools: row.SupportsTools, SupportsReasoning: row.SupportsReasoning,
		SupportsVision: row.SupportsVision, ReasoningEfforts: row.ReasoningEfforts,
		ReasoningToggle: row.ReasoningToggle, ReasoningBudget: row.ReasoningBudget,
		ReasoningBudgetMin: row.ReasoningBudgetMin, ReasoningBudgetMax: row.ReasoningBudgetMax,
		Status: row.Status, ReleaseDate: row.ReleaseDate,
		Prices: priceRecord(row.Prices),
	}
}

func priceRecord(row PriceRow) catalog.PriceRecord {
	return catalog.PriceRecord(row)
}

func routeRecord(row GroupRow) catalog.RouteRecord {
	members := make([]catalog.RouteMemberRecord, 0, len(row.Members))
	for _, member := range row.Members {
		members = append(members, catalog.RouteMemberRecord{
			Position: member.Position, ProviderID: member.ProviderID, ModelID: member.ModelID,
			Kind: member.Kind, Weight: member.Weight, Enabled: member.Enabled,
		})
	}
	return catalog.RouteRecord{
		ID: row.ID, Label: row.Label, Strategy: row.Strategy, Enabled: row.Enabled,
		Listed: row.Listed, Members: members, CreatedAtMs: row.CreatedAtMs, UpdatedAtMs: row.UpdatedAtMs,
		SwitchOn4xx: row.SwitchOn4xx, SwitchOn5xx: row.SwitchOn5xx,
	}
}

func groupRow(route catalog.RouteRecord) GroupRow {
	members := make([]GroupMemberRow, 0, len(route.Members))
	for _, member := range route.Members {
		members = append(members, GroupMemberRow{
			Position: member.Position, ProviderID: member.ProviderID, ModelID: member.ModelID,
			Kind: member.Kind, Weight: member.Weight, Enabled: member.Enabled,
		})
	}
	return GroupRow{
		ID: route.ID, Label: route.Label, Strategy: route.Strategy, Enabled: route.Enabled,
		Listed: route.Listed, Members: members, CreatedAtMs: route.CreatedAtMs, UpdatedAtMs: route.UpdatedAtMs,
		SwitchOn4xx: route.SwitchOn4xx, SwitchOn5xx: route.SwitchOn5xx,
	}
}

// SaveRoute writes one route from the catalog record shape.
func (r *CatalogRepo) SaveRoute(ctx context.Context, route catalog.RouteRecord) error {
	return r.SaveGroup(ctx, groupRow(route))
}

// DeleteRoute removes one route.
func (r *CatalogRepo) DeleteRoute(ctx context.Context, id string) error {
	return r.DeleteGroup(ctx, id)
}
