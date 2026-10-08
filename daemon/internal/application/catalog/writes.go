// Catalog writes: toggles, context, capabilities, and patches.
package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jonaskahn/relo/internal/account"
	"github.com/jonaskahn/relo/internal/catalog"
	"github.com/jonaskahn/relo/internal/config"
)

// ToggleModels enables or disables models in bulk.
func (s *Service) ToggleModels(ctx context.Context, providerID string, modelIDs []string, enabled bool) error {
	for _, mID := range modelIDs {
		m, err := s.store.GetModel(ctx, providerID, mID)
		if err == nil {
			m.Enabled = enabled
			_ = s.store.SaveModel(ctx, m)
		}
	}
	return s.reload(ctx)
}

// SetModelsContextWindow writes one context window on every named model of a
// connection, which is how an operator sizes a whole connection at once. A nil
// value clears the override, so the provider and models.dev layers decide
// again. A model the connection does not hold is skipped, the way the bulk
// switch skips one, and the catalog is reloaded once for the whole write.
func (s *Service) SetModelsContextWindow(ctx context.Context, providerID string, modelIDs []string, value *int64) error {
	if value != nil && (*value < 1 || *value > maxContextWindow) {
		return fmt.Errorf("context_window must be between 1 and %d: %w", maxContextWindow, ErrInvalidCatalogRow)
	}
	for _, modelID := range modelIDs {
		if _, err := s.modelRow(ctx, providerID, modelID); err != nil {
			continue
		}
		if err := s.writeOverrideContextWindow(ctx, providerID, modelID, value); err != nil {
			return err
		}
	}
	return s.reload(ctx)
}

// CapabilityFlags names the tool, reasoning, and vision overrides one
// capability write applies. A null clears that override.
type CapabilityFlags struct {
	Tools     OptionalBool
	Reasoning OptionalBool
	Vision    OptionalBool
}

func (f CapabilityFlags) any() bool {
	return f.Tools.Set || f.Reasoning.Set || f.Vision.Set
}

// SetModelsCapabilities writes one capability on every named model of a
// connection. A model the connection does not hold is skipped, and the
// catalog is reloaded once for the whole write.
func (s *Service) SetModelsCapabilities(ctx context.Context, providerID string, modelIDs []string, flags CapabilityFlags) error {
	if !flags.any() {
		return nil
	}
	for _, modelID := range modelIDs {
		if _, err := s.modelRow(ctx, providerID, modelID); err != nil {
			continue
		}
		if err := s.writeOverrideCapabilities(ctx, providerID, modelID, flags); err != nil {
			return err
		}
	}
	return s.reload(ctx)
}

// ProviderPatch is one provider write.
type ProviderPatch struct {
	Enabled      *bool
	Rank         *int
	PoolStrategy *string
	BaseURL      *string
	Variables    map[string]string
	Label        *string
	APIFormat    *string
	KeyHeader    *string
	Headers      map[string]string
	UseProxy     *bool
	// TimeoutSeconds sets this connection's call wait; a present null clears
	// it back to the global value.
	TimeoutSeconds OptionalInt
	// RetryBackoff sets this connection's retry windows; a present null
	// clears them back to the global windows.
	RetryBackoff OptionalRetryBackoff
	// SwitchOn4xx and SwitchOn5xx fail this connection over on an unlisted
	// status of that class. An absent field leaves the stored choice alone.
	SwitchOn4xx *bool
	SwitchOn5xx *bool
}

// PatchProvider applies an operator's edits to one connection, so a label,
// timeout, or failover choice changes without re-entering the credential.
func (s *Service) PatchProvider(ctx context.Context, id string, patch ProviderPatch) (Provider, error) {
	row, err := s.store.GetProvider(ctx, id)
	if err != nil {
		return Provider{}, err
	}
	if err := validateProviderPatch(patch); err != nil {
		return Provider{}, err
	}
	applyProviderPatch(&row, patch)
	row.UpdatedAtMs = catalog.NowMs()

	if err := s.store.SaveProvider(ctx, row); err != nil {
		return Provider{}, err
	}
	_ = s.reload(ctx)
	return s.Provider(ctx, id)
}

func validateProviderPatch(patch ProviderPatch) error {
	if patch.TimeoutSeconds.Set && patch.TimeoutSeconds.Value != nil {
		if err := config.ValidateUpstreamTimeout(int(*patch.TimeoutSeconds.Value)); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidCatalogRow, err)
		}
	}
	if patch.RetryBackoff.Set && patch.RetryBackoff.Value != nil {
		if err := config.ValidateUpstreamRetryBackoff(*patch.RetryBackoff.Value); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidCatalogRow, err)
		}
	}
	return nil
}

func applyProviderPatch(row *catalog.ConnectionRecord, patch ProviderPatch) {
	applyProviderScalars(row, patch)
	applyProviderIdentity(row, patch)
	applyProviderRetry(row, patch)
}

func applyProviderScalars(row *catalog.ConnectionRecord, patch ProviderPatch) {
	if patch.Enabled != nil {
		row.Enabled = *patch.Enabled
	}
	if patch.Rank != nil {
		row.Rank = *patch.Rank
	}
	if patch.PoolStrategy != nil {
		row.PoolStrategy = *patch.PoolStrategy
	}
	if patch.BaseURL != nil {
		row.BaseURL = *patch.BaseURL
	}
	if patch.Variables != nil {
		row.Variables = patch.Variables
	}
	if patch.Label != nil && strings.TrimSpace(*patch.Label) != "" {
		row.Label = strings.TrimSpace(*patch.Label)
	}
}

func applyProviderIdentity(row *catalog.ConnectionRecord, patch ProviderPatch) {
	if patch.APIFormat != nil {
		row.APIFormat = *patch.APIFormat
	}
	if patch.KeyHeader != nil {
		row.KeyHeader = *patch.KeyHeader
	}
	if patch.Headers != nil {
		row.Headers = patch.Headers
	}
	if patch.UseProxy != nil {
		row.UseProxy = *patch.UseProxy
	}
}

func applyProviderRetry(row *catalog.ConnectionRecord, patch ProviderPatch) {
	if patch.TimeoutSeconds.Set {
		if patch.TimeoutSeconds.Value == nil {
			row.TimeoutSeconds = nil
		} else {
			seconds := int(*patch.TimeoutSeconds.Value)
			row.TimeoutSeconds = &seconds
		}
	}
	if patch.RetryBackoff.Set {
		if patch.RetryBackoff.Value == nil {
			row.RetryBackoff = nil
		} else {
			row.RetryBackoff = *patch.RetryBackoff.Value
		}
	}
	if patch.SwitchOn4xx != nil {
		row.SwitchOn4xx = patch.SwitchOn4xx
	}
	if patch.SwitchOn5xx != nil {
		row.SwitchOn5xx = patch.SwitchOn5xx
	}
}

// DeleteProvider removes a connection the operator added, so a retired
// vendor stops appearing in setup and routing.
func (s *Service) DeleteProvider(ctx context.Context, id string) error {
	var err error
	s.accounts.WithLock(func() {
		err = s.deleteProvider(ctx, id)
	})
	return err
}

func (s *Service) deleteProvider(ctx context.Context, id string) error {
	// The reference check comes first: a group that routes to this provider's
	// models has to leave its accounts and secrets exactly where they are.
	if names := s.groupsUsingProvider(id); len(names) > 0 {
		return fmt.Errorf("%s: %w (a group routes to its models: %s)",
			id, ErrCatalogConflict, strings.Join(names, ", "))
	}
	if _, err := s.providerRow(ctx, id); err != nil {
		return err
	}
	accounts, err := s.providerAccounts(ctx, id)
	if err != nil {
		return err
	}
	if err := s.removeProviderRow(ctx, id); err != nil {
		return err
	}
	s.detachProviderAccounts(id, accounts)
	for _, entry := range accounts {
		if err := s.forgetSecret(entry.SecretRef); err != nil {
			return err
		}
	}
	return s.reload(ctx)
}

func (s *Service) providerAccounts(ctx context.Context, id string) ([]account.PoolEntry, error) {
	rows, err := s.entries.List(ctx)
	if err != nil {
		return nil, err
	}
	accounts := make([]account.PoolEntry, 0)
	for _, row := range rows {
		if row.ProviderID == id {
			accounts = append(accounts, row)
		}
	}
	return accounts, nil
}

func (s *Service) removeProviderRow(ctx context.Context, id string) error {
	if err := s.store.DeleteProvider(ctx, id); err != nil {
		if errors.Is(err, catalog.ErrConflict) {
			return fmt.Errorf("%s: %w (a group routes to its models)", id, ErrCatalogConflict)
		}
		return err
	}
	return nil
}

func (s *Service) detachProviderAccounts(id string, accounts []account.PoolEntry) {
	if s.pools == nil {
		return
	}
	pool := s.pools.GetPool(id)
	for _, entry := range accounts {
		pool.Remove(entry.ID)
	}
}

// ModelOverride is the override layer of one model. It replaces the layer
// that was stored before it, and a null field states nothing rather than
// zero, so clearing a value falls back to the provider and models.dev layers.
type ModelOverride struct {
	Name          *string
	Description   *string
	Category      *string
	ContextWindow *int64
	MaxInput      *int64
	MaxOutput     *int64
	Tools         *bool
	Reasoning     *bool
	Vision        *bool
	Prices        *Prices
}

// ModelPatch is one model write. Every field is optional: enabled is the
// switch, priced_as is the models.dev reference an operator chose, and
// override replaces the whole override layer.
type ModelPatch struct {
	Enabled  *bool
	PricedAs *string
	Override *ModelOverride
	// UpstreamModelID is the identifier Relo sends to the provider, which only
	// a clone may change.
	UpstreamModelID *string
	// ContextWindow rewrites only the context window of the override layer, so
	// a console can set one value without replacing the rest. It tells an
	// absent field from one that is present and null.
	ContextWindow OptionalInt
	// MaxOutput rewrites only the max output of the override layer. A null
	// clears that field and leaves the rest of the layer alone.
	MaxOutput OptionalInt
	// Tools, Reasoning, and Vision rewrite one capability of the override
	// layer. A null clears that flag and leaves the rest of the layer alone.
	Tools     OptionalBool
	Reasoning OptionalBool
	Vision    OptionalBool
	// Prices rewrites the rates of the override layer. The rates replace the
	// ones the layer held, so a console that edits one rate sends back the
	// ones it kept, and a present object with no rate clears them all back to
	// the provider and models.dev layers.
	Prices *Prices
}

// OptionalInt tells an absent JSON field from one that is present and null. An
// absent field leaves a stored value alone and a null clears it, which is what
// a context window override needs to be able to say.
type OptionalInt struct {
	Set   bool
	Value *int64
}

// UnmarshalJSON records whether the field was present, so an absent number
// leaves the stored value alone while null clears it.
func (o *OptionalInt) UnmarshalJSON(data []byte) error {
	o.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		o.Value = nil
		return nil
	}
	var value int64
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

// OptionalBool tells an absent JSON field from one that is present and null.
// An absent field leaves a stored flag alone and a null clears it, which is
// what one capability override needs to be able to say.
type OptionalBool struct {
	Set   bool
	Value *bool
}

// UnmarshalJSON records whether the field was present, so an absent flag
// leaves the stored value alone while null clears it.
func (o *OptionalBool) UnmarshalJSON(data []byte) error {
	o.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		o.Value = nil
		return nil
	}
	var value bool
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

// OptionalRetryBackoff tells an absent JSON field from one that is present
// and null. An absent field leaves the connection's windows alone and a null
// clears them back to the global windows.
type OptionalRetryBackoff struct {
	Set   bool
	Value *[][2]int
}

// UnmarshalJSON records whether the field was present, so absent windows
// leave the connection's retries alone while null restores the global ones.
func (o *OptionalRetryBackoff) UnmarshalJSON(data []byte) error {
	o.Set = true
	if strings.TrimSpace(string(data)) == "null" {
		o.Value = nil
		return nil
	}
	var value [][2]int
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

const maxContextWindow = 100_000_000

// PatchModel applies an operator's edits to one model, so its facts,
// capabilities, or prices change without re-reading the provider.
func (s *Service) PatchModel(ctx context.Context, providerID, modelID string, patch ModelPatch) (Model, error) {
	// A single-field write and a whole-override write say the same thing, so
	// asking for both at once is refused rather than resolved in an order the
	// operator cannot see.
	if patch.Override != nil && (patch.ContextWindow.Set || patch.MaxOutput.Set || patch.Tools.Set || patch.Reasoning.Set || patch.Vision.Set || patch.Prices != nil) {
		return Model{}, fmt.Errorf("override cannot be set together with a single field: %w", ErrInvalidCatalogRow)
	}

	mRow, err := s.modelRow(ctx, providerID, modelID)
	if err != nil {
		return Model{}, err
	}

	if err := s.applyModelPatch(ctx, providerID, modelID, &mRow, patch); err != nil {
		return Model{}, err
	}
	mRow.UpdatedAtMs = catalog.NowMs()
	if err := s.store.SaveModel(ctx, mRow); err != nil {
		return Model{}, err
	}

	if err := s.writeModelOverrides(ctx, providerID, modelID, patch); err != nil {
		return Model{}, err
	}

	_ = s.reload(ctx)
	return s.Model(ctx, providerID, modelID)
}

func (s *Service) applyModelPatch(ctx context.Context, providerID, modelID string, mRow *catalog.ModelRecord, patch ModelPatch) error {
	if patch.Enabled != nil {
		mRow.Enabled = *patch.Enabled
	}
	if err := s.applyModelUpstreamID(providerID, modelID, mRow, patch.UpstreamModelID); err != nil {
		return err
	}
	return s.applyModelPricedAs(mRow, patch.PricedAs)
}

func (s *Service) applyModelUpstreamID(providerID, modelID string, mRow *catalog.ModelRecord, upstreamID *string) error {
	if upstreamID == nil {
		return nil
	}
	if mRow.ClonedFrom == "" {
		return fmt.Errorf("%s/%s: %w (only a cloned model has an upstream id)",
			providerID, modelID, ErrNotCustom)
	}
	upstream := strings.TrimSpace(*upstreamID)
	if upstream == "" {
		return fmt.Errorf("a clone needs an upstream id: %w", ErrInvalidCatalogRow)
	}
	mRow.UpstreamModelID = upstream
	return nil
}

func (s *Service) applyModelPricedAs(mRow *catalog.ModelRecord, pricedAs *string) error {
	if pricedAs == nil {
		return nil
	}
	ref := strings.TrimSpace(*pricedAs)
	if ref == "" {
		// An empty reference returns the model to automatic matching.
		mRow.ModelsDevRef = ""
		mRow.Match = string(catalog.ModelsDevMatchNone)
		return nil
	}
	if _, _, _, found := s.priceModel(ref); !found {
		return fmt.Errorf("%s: %w", ref, ErrUnknownModelsDevRef)
	}
	mRow.ModelsDevRef = ref
	mRow.Match = string(catalog.ModelsDevMatchManual)
	return nil
}

func (s *Service) writeModelOverrides(ctx context.Context, providerID, modelID string, patch ModelPatch) error {
	if patch.ContextWindow.Set {
		if err := s.writeOverrideContextWindow(ctx, providerID, modelID, patch.ContextWindow.Value); err != nil {
			return err
		}
	} else if err := s.writeOverride(ctx, providerID, modelID, patch.Override); err != nil {
		return err
	}
	if patch.MaxOutput.Set {
		if err := s.writeOverrideMaxOutput(ctx, providerID, modelID, patch.MaxOutput.Value); err != nil {
			return err
		}
	}
	if err := s.writeOverrideCapabilities(ctx, providerID, modelID, CapabilityFlags{Tools: patch.Tools, Reasoning: patch.Reasoning, Vision: patch.Vision}); err != nil {
		return err
	}
	if patch.Prices != nil {
		if err := s.writeOverridePrices(ctx, providerID, modelID, patch.Prices); err != nil {
			return err
		}
	}
	if patch.PricedAs == nil {
		return nil
	}
	// A reference an operator chose reads from the copy Relo holds, so the
	// prices the console reads next are the ones just chosen.
	return s.rematchModel(ctx, providerID, modelID)
}

func (s *Service) writeOverride(ctx context.Context, providerID, modelID string, override *ModelOverride) error {
	if override == nil {
		return nil
	}
	roundOverrideCounts(override)
	if err := validateOverride(*override); err != nil {
		return err
	}
	row := catalog.FactsRecord{
		ProviderID: providerID, ModelID: modelID, Layer: "override",
		Name: override.Name, Description: override.Description, Category: override.Category,
		ContextWindow: override.ContextWindow, MaxInput: override.MaxInput,
		MaxOutput: override.MaxOutput, SupportsTools: override.Tools,
		SupportsReasoning: override.Reasoning, SupportsVision: override.Vision,
	}
	if override.Prices != nil {
		row.Prices = override.Prices.row()
	}
	if row.Empty() {
		return s.store.DeleteModelFactsLayer(ctx, providerID, modelID, "override")
	}
	return s.store.SaveModelFacts(ctx, row)
}

func (s *Service) writeOverrideContextWindow(ctx context.Context, providerID, modelID string, value *int64) error {
	if value != nil && (*value < 1 || *value > maxContextWindow) {
		return fmt.Errorf("context_window must be between 1 and %d: %w", maxContextWindow, ErrInvalidCatalogRow)
	}
	value = catalog.RoundContextWindow(value)
	row, err := s.overrideLayer(ctx, providerID, modelID)
	if err != nil {
		return err
	}
	row.ProviderID, row.ModelID, row.Layer = providerID, modelID, "override"
	row.ContextWindow = value
	if row.Empty() {
		return s.store.DeleteModelFactsLayer(ctx, providerID, modelID, "override")
	}
	return s.store.SaveModelFacts(ctx, row)
}

func (s *Service) writeOverrideMaxOutput(ctx context.Context, providerID, modelID string, value *int64) error {
	if value != nil && (*value < 1 || *value > maxContextWindow) {
		return fmt.Errorf("max_output must be between 1 and %d: %w", maxContextWindow, ErrInvalidCatalogRow)
	}
	value = catalog.RoundContextWindow(value)
	row, err := s.overrideLayer(ctx, providerID, modelID)
	if err != nil {
		return err
	}
	row.ProviderID, row.ModelID, row.Layer = providerID, modelID, "override"
	row.MaxOutput = value
	if row.Empty() {
		return s.store.DeleteModelFactsLayer(ctx, providerID, modelID, "override")
	}
	return s.store.SaveModelFacts(ctx, row)
}

func (s *Service) writeOverrideCapabilities(ctx context.Context, providerID, modelID string, flags CapabilityFlags) error {
	if !flags.any() {
		return nil
	}
	row, err := s.overrideLayer(ctx, providerID, modelID)
	if err != nil {
		return err
	}
	row.ProviderID, row.ModelID, row.Layer = providerID, modelID, "override"
	if flags.Tools.Set {
		row.SupportsTools = flags.Tools.Value
	}
	if flags.Reasoning.Set {
		row.SupportsReasoning = flags.Reasoning.Value
	}
	if flags.Vision.Set {
		row.SupportsVision = flags.Vision.Value
	}
	if row.Empty() {
		return s.store.DeleteModelFactsLayer(ctx, providerID, modelID, "override")
	}
	return s.store.SaveModelFacts(ctx, row)
}

func (s *Service) writeOverridePrices(ctx context.Context, providerID, modelID string, prices *Prices) error {
	if err := validateOverride(ModelOverride{Prices: prices}); err != nil {
		return err
	}
	row, err := s.overrideLayer(ctx, providerID, modelID)
	if err != nil {
		return err
	}
	row.ProviderID, row.ModelID, row.Layer = providerID, modelID, "override"
	row.Prices = prices.row()
	if row.Empty() {
		return s.store.DeleteModelFactsLayer(ctx, providerID, modelID, "override")
	}
	return s.store.SaveModelFacts(ctx, row)
}

func (s *Service) overrideLayer(ctx context.Context, providerID, modelID string) (catalog.FactsRecord, error) {
	rows, err := s.store.ListModelFacts(ctx, providerID, modelID)
	if err != nil {
		return catalog.FactsRecord{}, err
	}
	for _, row := range rows {
		if row.Layer == "override" {
			return row, nil
		}
	}
	return catalog.FactsRecord{}, nil
}

func (s *Service) applyOverrideFields(ctx context.Context, providerID, modelID string, override *ModelOverride) error {
	if override == nil {
		return nil
	}
	roundOverrideCounts(override)
	if err := validateOverride(*override); err != nil {
		return err
	}
	row, err := s.overrideLayer(ctx, providerID, modelID)
	if err != nil {
		return err
	}
	row.ProviderID, row.ModelID, row.Layer = providerID, modelID, "override"
	fillOverrideFacts(&row, override)
	if row.Empty() {
		return s.store.DeleteModelFactsLayer(ctx, providerID, modelID, "override")
	}
	return s.store.SaveModelFacts(ctx, row)
}

func fillOverrideFacts(row *catalog.FactsRecord, override *ModelOverride) {
	if override.Name != nil {
		row.Name = override.Name
	}
	if override.Description != nil {
		row.Description = override.Description
	}
	if override.Category != nil {
		row.Category = override.Category
	}
	fillOverrideWindows(row, override)
	fillOverrideCapabilities(row, override)
	if override.Prices != nil {
		row.Prices = mergePriceRow(row.Prices, override.Prices.row())
	}
}

func fillOverrideWindows(row *catalog.FactsRecord, override *ModelOverride) {
	if override.ContextWindow != nil {
		row.ContextWindow = override.ContextWindow
	}
	if override.MaxInput != nil {
		row.MaxInput = override.MaxInput
	}
	if override.MaxOutput != nil {
		row.MaxOutput = override.MaxOutput
	}
}

func fillOverrideCapabilities(row *catalog.FactsRecord, override *ModelOverride) {
	if override.Tools != nil {
		row.SupportsTools = override.Tools
	}
	if override.Reasoning != nil {
		row.SupportsReasoning = override.Reasoning
	}
	if override.Vision != nil {
		row.SupportsVision = override.Vision
	}
}

func mergePriceRow(base, overlay catalog.PriceRecord) catalog.PriceRecord {
	if overlay.Input != nil {
		base.Input = overlay.Input
	}
	if overlay.Output != nil {
		base.Output = overlay.Output
	}
	if overlay.CacheRead != nil {
		base.CacheRead = overlay.CacheRead
	}
	if overlay.CacheWrite != nil {
		base.CacheWrite = overlay.CacheWrite
	}
	if overlay.ExtThreshold != nil {
		base.ExtThreshold = overlay.ExtThreshold
	}
	if overlay.ExtInput != nil {
		base.ExtInput = overlay.ExtInput
	}
	if overlay.ExtOutput != nil {
		base.ExtOutput = overlay.ExtOutput
	}
	if overlay.ExtCacheRead != nil {
		base.ExtCacheRead = overlay.ExtCacheRead
	}
	if overlay.ExtCacheWrite != nil {
		base.ExtCacheWrite = overlay.ExtCacheWrite
	}
	return base
}

func roundOverrideCounts(override *ModelOverride) {
	override.ContextWindow = catalog.RoundContextWindow(override.ContextWindow)
	override.MaxInput = catalog.RoundContextWindow(override.MaxInput)
	override.MaxOutput = catalog.RoundContextWindow(override.MaxOutput)
}

func validateOverride(override ModelOverride) error {
	if override.Category != nil && !knownCategory(*override.Category) {
		return fmt.Errorf("%s: %w (unknown model category)", *override.Category, ErrInvalidCatalogRow)
	}
	numbers := map[string]*int64{
		"context_window": override.ContextWindow,
		"max_input":      override.MaxInput,
		"max_output":     override.MaxOutput,
	}
	if override.Prices != nil {
		numbers["prices.input"] = override.Prices.Input
		numbers["prices.output"] = override.Prices.Output
		numbers["prices.cache_read"] = override.Prices.CacheRead
		numbers["prices.cache_write"] = override.Prices.CacheWrite
		numbers["prices.ext_threshold"] = override.Prices.ExtThreshold
		numbers["prices.ext_input"] = override.Prices.ExtInput
		numbers["prices.ext_output"] = override.Prices.ExtOutput
		numbers["prices.ext_cache_read"] = override.Prices.ExtCacheRead
		numbers["prices.ext_cache_write"] = override.Prices.ExtCacheWrite
	}
	for name, value := range numbers {
		if value != nil && *value < 0 {
			return fmt.Errorf("%s must not be negative: %w", name, ErrInvalidCatalogRow)
		}
	}
	return nil
}

func knownCategory(value string) bool {
	switch catalog.Category(strings.TrimSpace(value)) {
	case catalog.CategoryChat, catalog.CategoryReasoning, catalog.CategoryVision,
		catalog.CategoryImage, catalog.CategoryAudio, catalog.CategoryVideo,
		catalog.CategoryEmbedding:
		return true
	default:
		return false
	}
}

func (s *Service) rematchModel(ctx context.Context, providerID, modelID string) error {
	model, err := s.modelRow(ctx, providerID, modelID)
	if err != nil {
		return err
	}
	host, err := s.providerRow(ctx, providerID)
	if err != nil {
		return err
	}
	idx, _ := s.modelsDev.Cached()
	fact, _, found := s.layerForRef(
		host.ID, modelsDevProviderIDOf(host.ModelsDevProviderID, host.ID),
		model.ModelID, model.Match, model.ModelsDevRef, idx,
	)
	if !found {
		return s.store.DeleteModelFactsLayer(ctx, providerID, modelID, "modelsdev")
	}
	return s.store.SaveModelFacts(ctx, fact)
}

// AddModel adds a model by hand, which is how an operator routes to an id of a
// connection that publishes no list of its own.
func requireHandListed(modelsFormat string, providerID string) error {
	// A connection that publishes its own model list is the authority on which
	// models it serves, so an id typed here would name a model nothing proved
	// reachable. Azure, Vertex and Kiro publish no list and are the ones that
	// take an id by hand.
	if modelsFormat != string(catalog.ModelsNone) {
		return fmt.Errorf("%s: %w", providerID, ErrListAuthoritative)
	}
	return nil
}

// AddModel adds a model by hand, which is how an operator routes to an id of a
// connection that publishes no list of its own.
func (s *Service) AddModel(ctx context.Context, providerID, modelID, pricedAs string) (Model, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return Model{}, fmt.Errorf("a model needs an id: %w", ErrInvalidCatalogRow)
	}
	host, err := s.providerRow(ctx, providerID)
	if err != nil {
		return Model{}, err
	}
	if err := requireHandListed(host.ModelsFormat, providerID); err != nil {
		return Model{}, err
	}
	if err := s.ensureModelAbsent(ctx, providerID, modelID); err != nil {
		return Model{}, err
	}
	if err := s.checkPricedAsRef(pricedAs); err != nil {
		return Model{}, err
	}
	row := newManualModelRow(providerID, modelID, pricedAs)
	if err := s.store.SaveModel(ctx, row); err != nil {
		return Model{}, err
	}
	if err := s.rematchModel(ctx, providerID, modelID); err != nil {
		return Model{}, err
	}
	_ = s.reload(ctx)
	return s.Model(ctx, providerID, modelID)
}

func (s *Service) ensureModelAbsent(ctx context.Context, providerID, modelID string) error {
	if _, err := s.store.GetModel(ctx, providerID, modelID); err == nil {
		return fmt.Errorf("%s/%s: %w (the provider already has this model)",
			providerID, modelID, ErrCatalogConflict)
	} else if !errors.Is(err, catalog.ErrModelNotFound) {
		return err
	}
	return nil
}

func newManualModelRow(providerID, modelID, pricedAs string) catalog.ModelRecord {
	available := true
	row := catalog.ModelRecord{
		ProviderID: providerID, ModelID: modelID, Source: SourceManual,
		Enabled: true, Available: &available,
		Match: string(catalog.ModelsDevMatchNone), UpdatedAtMs: catalog.NowMs(),
	}
	row.ModelsDevRef, row.Match = pricedAsRow(pricedAs)
	return row
}

func pricedAsRow(pricedAs string) (string, string) {
	ref := strings.TrimSpace(pricedAs)
	if ref == "" {
		return "", string(catalog.ModelsDevMatchNone)
	}
	return ref, string(catalog.ModelsDevMatchManual)
}

func (s *Service) checkPricedAsRef(pricedAs string) error {
	ref := strings.TrimSpace(pricedAs)
	if ref == "" {
		return nil
	}
	if _, _, _, found := s.priceModel(ref); !found {
		return fmt.Errorf("%s: %w", ref, ErrUnknownModelsDevRef)
	}
	return nil
}

// CloneRequest is one model clone: the model to copy, the identifier the clone
// is stored under, and the upstream id Relo sends. A clone reaches a model the
// provider has not listed yet by keeping its own id while sending another
// upstream.
type CloneRequest struct {
	SourceModelID   string
	ModelID         string
	UpstreamModelID *string
	Override        *ModelOverride
}

const modelIDMaxLength = 256

// CloneModel copies one model under a new identifier. The clone starts from
// the source's format, endpoint, pricing reference, provider facts and
// overrides, then takes on the fields the request states. It keeps its own id
// while Relo sends the upstream id to the provider.
func (s *Service) CloneModel(ctx context.Context, providerID string, req CloneRequest) (Model, error) {
	sourceID := strings.TrimSpace(req.SourceModelID)
	if sourceID == "" {
		return Model{}, fmt.Errorf("a clone needs a source model: %w", ErrInvalidCatalogRow)
	}
	newID, err := validateModelID(req.ModelID)
	if err != nil {
		return Model{}, err
	}
	source, err := s.modelRow(ctx, providerID, sourceID)
	if err != nil {
		return Model{}, err
	}
	if err := s.ensureModelAbsent(ctx, providerID, newID); err != nil {
		return Model{}, err
	}
	return s.saveClone(ctx, providerID, newID, source, req)
}

func (s *Service) saveClone(ctx context.Context, providerID, newID string, source catalog.ModelRecord, req CloneRequest) (Model, error) {
	upstream, err := cloneUpstreamID(source, req.UpstreamModelID)
	if err != nil {
		return Model{}, err
	}
	if err := s.store.SaveModel(ctx, cloneRow(providerID, newID, upstream, source)); err != nil {
		return Model{}, err
	}
	if err := s.copyFacts(ctx, providerID, source.ModelID, newID); err != nil {
		return Model{}, err
	}
	if err := s.applyOverrideFields(ctx, providerID, newID, req.Override); err != nil {
		return Model{}, err
	}
	if err := s.rematchModel(ctx, providerID, newID); err != nil {
		return Model{}, err
	}
	_ = s.reload(ctx)
	return s.Model(ctx, providerID, newID)
}

func cloneUpstreamID(source catalog.ModelRecord, override *string) (string, error) {
	// Cloning a clone keeps the real upstream id rather than pointing at the
	// clone, so a second copy still reaches the same provider model.
	upstream := strings.TrimSpace(source.UpstreamModelID)
	if upstream == "" {
		upstream = source.ModelID
	}
	if override != nil {
		upstream = strings.TrimSpace(*override)
		if upstream == "" {
			return "", fmt.Errorf("a clone needs an upstream id: %w", ErrInvalidCatalogRow)
		}
	}
	return upstream, nil
}

func cloneRow(providerID, newID, upstream string, source catalog.ModelRecord) catalog.ModelRecord {
	available := true
	row := catalog.ModelRecord{
		ProviderID:      providerID,
		ModelID:         newID,
		Source:          SourceManual,
		APIFormat:       source.APIFormat,
		BaseURL:         source.BaseURL,
		ModelsDevRef:    source.ModelsDevRef,
		Match:           source.Match,
		Enabled:         source.Enabled,
		Available:       &available,
		UpstreamModelID: upstream,
		ClonedFrom:      source.ModelID,
		UpdatedAtMs:     catalog.NowMs(),
	}
	if row.Match == "" {
		row.Match = string(catalog.ModelsDevMatchNone)
	}
	return row
}

func (s *Service) copyFacts(ctx context.Context, providerID, sourceID, targetID string) error {
	rows, err := s.store.ListModelFacts(ctx, providerID, sourceID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Layer != "provider" && row.Layer != "override" {
			continue
		}
		row.ProviderID = providerID
		row.ModelID = targetID
		if err := s.store.SaveModelFacts(ctx, row); err != nil {
			return err
		}
	}
	return nil
}

func validateModelID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", fmt.Errorf("a model needs an id: %w", ErrInvalidCatalogRow)
	}
	if len(id) > modelIDMaxLength {
		return "", fmt.Errorf("a model id is at most %d characters: %w", modelIDMaxLength, ErrInvalidCatalogRow)
	}
	if strings.ContainsAny(id, " \t\r\n") {
		return "", fmt.Errorf("a model id cannot contain whitespace: %w", ErrInvalidCatalogRow)
	}
	return id, nil
}

// DeleteModel removes a model an operator added by hand. A model a listing
// publishes stays, because the next refresh would only add it back.
func (s *Service) DeleteModel(ctx context.Context, providerID, modelID string) error {
	row, err := s.modelRow(ctx, providerID, modelID)
	if err != nil {
		return err
	}
	if row.Source != SourceManual {
		return fmt.Errorf("%s/%s: %w (only a model added by hand can be removed)",
			providerID, modelID, ErrNotCustom)
	}
	if names := s.groupsUsing(providerID, modelID); len(names) > 0 {
		return fmt.Errorf("%s/%s: %w (a group routes to this model: %s)",
			providerID, modelID, ErrCatalogConflict, strings.Join(names, ", "))
	}
	if err := s.store.DeleteModelRow(ctx, providerID, modelID); err != nil {
		return err
	}
	return s.reload(ctx)
}

func (s *Service) groupsUsing(providerID, modelID string) []string {
	snap, ok := s.catalog.Snapshot()
	if !ok {
		return nil
	}
	var names []string
	for _, group := range snap.Groups {
		for _, member := range group.Members {
			if member.ProviderID == providerID && member.ModelID == modelID {
				names = append(names, groupLabel(group))
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

func (s *Service) groupsUsingProvider(providerID string) []string {
	if s.catalog == nil {
		return nil
	}
	snap, ok := s.catalog.Snapshot()
	if !ok {
		return nil
	}
	var names []string
	for _, group := range snap.Groups {
		for _, member := range group.Members {
			if member.ProviderID == providerID {
				names = append(names, groupLabel(group))
				break
			}
		}
	}
	sort.Strings(names)
	return names
}

func groupLabel(group catalog.Group) string {
	if strings.TrimSpace(group.Label) != "" {
		return group.Label
	}
	return group.ID
}

func (s *Service) modelRow(ctx context.Context, providerID, modelID string) (catalog.ModelRecord, error) {
	row, err := s.store.GetModel(ctx, providerID, modelID)
	if errors.Is(err, catalog.ErrModelNotFound) {
		return catalog.ModelRecord{}, fmt.Errorf("%s/%s: %w", providerID, modelID, ErrModelNotFound)
	}
	return row, err
}

func (s *Service) providerRow(ctx context.Context, id string) (catalog.ConnectionRecord, error) {
	row, err := s.store.GetProvider(ctx, id)
	if errors.Is(err, catalog.ErrProviderNotFound) {
		return catalog.ConnectionRecord{}, fmt.Errorf("%s: %w", id, ErrProviderNotFound)
	}
	return row, err
}

func (s *Service) uniqueProviderID(ctx context.Context, baseID string) string {
	candidate := baseID
	counter := 1
	for {
		_, err := s.store.GetProvider(ctx, candidate)
		if err != nil {
			return candidate
		}
		counter++
		candidate = fmt.Sprintf("%s-%d", baseID, counter)
	}
}

func (p Prices) row() catalog.PriceRecord {
	return catalog.PriceRecord{
		Input:         p.Input,
		Output:        p.Output,
		CacheRead:     p.CacheRead,
		CacheWrite:    p.CacheWrite,
		ExtThreshold:  p.ExtThreshold,
		ExtInput:      p.ExtInput,
		ExtOutput:     p.ExtOutput,
		ExtCacheRead:  p.ExtCacheRead,
		ExtCacheWrite: p.ExtCacheWrite,
	}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func stringPtr(s string) *string {
	val := s
	return &val
}

func stringPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return stringPtr(s)
}

func resolvePrices(ov, pr, md catalog.PriceRecord) (catalog.Prices, PriceSourceMap) {
	var eff catalog.Prices
	var src PriceSourceMap

	eff.Input, src.Input = pickPriceField(ov.Input, pr.Input, md.Input)
	eff.Output, src.Output = pickPriceField(ov.Output, pr.Output, md.Output)
	eff.CacheRead, src.CacheRead = pickPriceField(ov.CacheRead, pr.CacheRead, md.CacheRead)
	eff.CacheWrite, src.CacheWrite = pickPriceField(ov.CacheWrite, pr.CacheWrite, md.CacheWrite)
	eff.ExtThreshold, src.ExtThreshold = pickPriceField(ov.ExtThreshold, pr.ExtThreshold, md.ExtThreshold)
	eff.ExtInput, src.ExtInput = pickPriceField(ov.ExtInput, pr.ExtInput, md.ExtInput)
	eff.ExtOutput, src.ExtOutput = pickPriceField(ov.ExtOutput, pr.ExtOutput, md.ExtOutput)
	eff.ExtCacheRead, src.ExtCacheRead = pickPriceField(ov.ExtCacheRead, pr.ExtCacheRead, md.ExtCacheRead)
	eff.ExtCacheWrite, src.ExtCacheWrite = pickPriceField(ov.ExtCacheWrite, pr.ExtCacheWrite, md.ExtCacheWrite)

	return eff, src
}

func pickPriceField(ov, pr, md *int64) (*int64, string) {
	if ov != nil {
		return ov, "override"
	}
	if pr != nil {
		return pr, "provider"
	}
	if md != nil {
		return md, "modelsdev"
	}
	return nil, ""
}
