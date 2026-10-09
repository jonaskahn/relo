// The management API's response shapes, bound exactly as the Go handlers
// serialize them.
import type { Accent, ThemeChoice } from './theme.svelte';

/** The daemon's answer to one status read: what it is running and what it serves. */
export interface StatusResponse {
	readonly status: string;
	readonly version: string;
	readonly uptime_seconds: number;
	readonly addr: string;
	readonly secret_mode: string;
	readonly schema_version: number;
	readonly client_keys: number;
	readonly active_client_keys: number;
	// The address of every protocol a client can point at, one entry per port
	// the daemon serves.
	readonly data_plane: DataPlaneAddress[];
	readonly language: string;
}

/** One protocol endpoint the data plane listens on. */
export interface DataPlaneAddress {
	readonly protocol: string;
	readonly base_url: string;
}

/** What the console asks before it renders a page: whether the request carries a live session,
 *  and whether this daemon asks for a sign-in at all. */
export interface SessionResponse {
	readonly authenticated: boolean;
	readonly login_required: boolean;
}

/** One doctor row: what was checked, what was found, and the fix when it failed. */
export interface DoctorCheck {
	readonly name: string;
	readonly status: 'pass' | 'warn' | 'fail';
	readonly detail: string;
}

/** Every rate Relo knows for one model, in integer USD micros per million tokens.
 *  A null rate is an unknown one, which stays distinct from a rate of zero. */
export interface Prices {
	readonly input: number | null;
	readonly output: number | null;
	readonly cache_read: number | null;
	readonly cache_write: number | null;
	readonly ext_threshold: number | null;
	readonly ext_input: number | null;
	readonly ext_output: number | null;
	readonly ext_cache_read: number | null;
	readonly ext_cache_write: number | null;
}

/** What one connection holds: accounts, models, and what still needs a price. */
export interface ProviderCounts {
	readonly models: number;
	readonly enabled_models: number;
	readonly available_models: number;
	readonly unpriced_models: number;
	readonly accounts: number;
	readonly active_accounts: number;
	// paused_accounts and reauth_accounts are what tell a console why a
	// provider with accounts is still not routing through any of them.
	readonly paused_accounts: number;
	readonly reauth_accounts: number;
}

/** One connection as the console reads it. */
export interface Provider {
	readonly id: string;
	readonly template_id: string;
	readonly label: string;
	// kind is the section the provider is listed under: signin, key, cloud or
	// local. available_formats and variable_defs describe the shapes it may be
	// switched to and what its base URL still needs.
	readonly kind: 'signin' | 'key' | 'cloud' | 'local' | string;
	readonly available_formats: TemplateFormatOption[] | null;
	readonly variable_defs: TemplateVariable[] | null;
	readonly origin: 'signin' | 'custom' | 'template' | string;
	readonly auth: 'oauth' | 'api_key' | 'aws' | 'gcp' | 'none' | string;
	readonly api_format: string;
	readonly api_formats: string[];
	readonly key_header: string;
	readonly models_source: string;
	readonly models_format: string;
	// models_per_account is true when each account publishes a roster of its
	// own, which is what makes an account-level model list worth reading.
	readonly models_per_account?: boolean;
	readonly modelsdev_provider_id: string;
	readonly base_url: string;
	readonly doc_url: string;
	readonly key_env: string[];
	readonly login_flows: string[];
	readonly login_methods?: LoginMethod[];
	readonly headers: Record<string, string>;
	readonly variables: Record<string, string>;
	readonly needs_setup: string[];
	readonly routable: boolean;
	readonly unroutable_reason: string;
	// serving_accounts and active_accounts report how many accounts behind the
	// connection could take a request now and how many of them can serve this
	// model, which is what tells a model that lists from one that routes.
	readonly serving_accounts?: number;
	readonly active_accounts?: number;
	readonly enabled: boolean;
	readonly use_proxy: boolean;
	// timeout_seconds is this connection's own call wait in seconds; null
	// inherits the global value. retry_backoff is its own retry windows;
	// null inherits the global windows.
	readonly timeout_seconds: number | null;
	readonly retry_backoff: [number, number][] | null;
	// switch_on_4xx and switch_on_5xx report whether this connection moves a
	// request to another of its accounts after a status the relay does not
	// always retry. The daemon resolves the default, so both are plain.
	readonly switch_on_4xx: boolean;
	readonly switch_on_5xx: boolean;
	readonly rank: number;
	readonly pool_strategy: string;
	readonly configured: boolean;
	readonly last_refreshed_at_ms?: number;
	readonly last_refresh_error?: string;
	readonly counts: ProviderCounts;
	readonly created_at_ms: number;
	readonly updated_at_ms: number;
}

/** One protocol a template can speak, with the host and the listing dialect that go with it. */
export interface TemplateFormatOption {
	readonly format: string;
	readonly default_base_url: string;
	readonly key_header: string;
	readonly models_format: string;
	readonly label: string;
}

/** One way into a provider account: the flow that runs it, and whether a human opens a page,
 *  types a device code, or lets Relo read the session an installed application already holds. */
export interface LoginMethod {
	readonly flow: string;
	readonly kind: 'browser' | 'device' | 'cli' | string;
}

/** One field a template needs in its base URL or headers. */
export interface TemplateVariable {
	readonly name: string;
	readonly label: string;
	readonly placeholder: string;
	readonly required: boolean;
	readonly options?: string[];
}

/** One way to add a provider. The kind decides which section of the add list the row appears in. */
export interface ProviderTemplate {
	readonly id: string;
	readonly label: string;
	readonly kind: 'signin' | 'key' | 'cloud' | 'local' | string;
	readonly origin: string;
	readonly auth: string;
	readonly key_header: string;
	readonly default_format: string;
	readonly available_formats: TemplateFormatOption[] | null;
	readonly default_base_url: string;
	readonly variables?: TemplateVariable[];
	readonly models_source: string;
	readonly models_format: string;
	readonly modelsdev_provider_id?: string;
	readonly doc_url?: string;
	readonly key_env?: string[];
	readonly login_flows?: string[];
	readonly headers?: Record<string, string>;
	readonly unsupported_reason?: string;
	readonly login_methods?: LoginMethod[];
	// modelsdev_models is how many models the template models.dev entry
	// lists, which the add list shows as a size hint.
	readonly modelsdev_models?: number;
}

/** The freshness of the models.dev catalog the daemon holds. */
export interface ModelsDevState {
	readonly source_url: string;
	readonly fetched_at_ms: number;
	readonly stale: boolean;
	readonly available: boolean;
	readonly providers_count: number;
	readonly models_count: number;
	readonly last_attempt_at_ms: number;
	readonly last_error?: string;
}

/** The template list with the state of the saved models.dev copy. */
export interface ProviderTemplatesResponse {
	readonly items: ProviderTemplate[];
	readonly modelsdev: ModelsDevState;
}

/** One line of the checklist a probe returns. */
export interface ProbeCheck {
	readonly name: string;
	readonly status: 'pass' | 'fail' | 'warn' | 'skip' | string;
	readonly detail: string;
}

/** One model a probe would save, tagged with the source its id came from so Review can say where
 *  the roster came from. */
export interface ProbeModel {
	readonly id: string;
	readonly name: string;
	readonly context_window?: number | null;
	readonly max_output?: number | null;
	readonly match: string;
	readonly modelsdev_ref: string;
	readonly source: 'listing' | 'modelsdev' | 'manual' | string;
	readonly prices: ModelPrices;
	readonly enabled: boolean;
}

/** What a sign-in probe found at one endpoint. */
export interface ProbeResult {
	readonly probe_id: string;
	readonly expires_at_ms: number;
	// target_provider_id names the provider a probe prepared a credential
	// for. It is set instead of a roster when the key is added to a provider
	// that already exists.
	readonly target_provider_id?: string;
	readonly checks: ProbeCheck[];
	readonly counts: {
		readonly listed: number;
		readonly matched: number;
		readonly priced: number;
		readonly from_listing: number;
		readonly from_manual: number;
	};
	readonly models: ProbeModel[];
	readonly modelsdev_state: ModelsDevState;
}

/** What a model says it can do. */
export interface Capabilities {
	readonly tools: boolean | null;
	readonly reasoning: boolean | null;
	readonly vision: boolean | null;
}

/** Names the layer each resolved rate came from, which is what lets a console tag a price
 *  without guessing. */
export interface PriceSourceMap {
	readonly input?: string;
	readonly output?: string;
	readonly cache_read?: string;
	readonly cache_write?: string;
	readonly ext_threshold?: string;
	readonly ext_input?: string;
	readonly ext_output?: string;
	readonly ext_cache_read?: string;
	readonly ext_cache_write?: string;
}

/** What a model charges, in micros per million tokens. */
export interface ModelPrices {
	readonly override: Prices;
	readonly provider: Prices;
	readonly modelsdev: Prices;
	readonly effective: Prices;
	readonly effective_source: PriceSourceMap;
}

/** One layer of a model's details exactly as it is stored.
 *  A null field is a layer that is silent about a value, which is what keeps it distinct from a
 *  layer that states the value is zero. */
export interface DetailLayer {
	readonly name: string | null;
	readonly description: string | null;
	readonly family: string | null;
	readonly category: string | null;
	readonly context_window: number | null;
	readonly max_input: number | null;
	readonly max_output: number | null;
	readonly tools: boolean | null;
	readonly reasoning: boolean | null;
	readonly vision: boolean | null;
	readonly status: string | null;
	readonly release_date: string | null;
}

/** Names where each detail of a model came from. */
export interface DetailSourceMap {
	readonly name?: string;
	readonly description?: string;
	readonly family?: string;
	readonly category?: string;
	readonly context_window?: string;
	readonly max_input?: string;
	readonly max_output?: string;
	readonly tools?: string;
	readonly reasoning?: string;
	readonly vision?: string;
	readonly status?: string;
	readonly release_date?: string;
}

/** Every detail layer of one model beside the name of the layer each resolved value came from.
 *  It is present on a single model read, not on a list. */
export interface ModelDetails {
	readonly override: DetailLayer;
	readonly provider: DetailLayer;
	readonly modelsdev: DetailLayer;
	readonly effective_source: DetailSourceMap;
}

/** The context window each layer states, exactly as stored.
 *  A null layer is silent about the value rather than stating zero. */
export interface ContextLayers {
	readonly override: number | null;
	readonly provider: number | null;
	readonly modelsdev: number | null;
}

/** The override layer a console writes, which replaces the layer that was stored before it.
 *  A null field clears a value rather than stating zero. */
export interface ModelOverride {
	readonly name?: string | null;
	readonly description?: string | null;
	readonly category?: string | null;
	readonly context_window?: number | null;
	readonly max_input?: number | null;
	readonly max_output?: number | null;
	readonly tools?: boolean | null;
	readonly reasoning?: boolean | null;
	readonly vision?: boolean | null;
	readonly prices?: Prices;
}

/** One model on one connection. */
export interface Model {
	readonly provider_id: string;
	readonly model_id: string;
	// upstream_model_id is the identifier Relo sends to the provider, which a
	// clone points at a model the provider has not listed yet. cloned_from
	// names the model a clone was copied from, empty on every other row.
	readonly upstream_model_id: string;
	readonly cloned_from: string;
	readonly source: 'listing' | 'modelsdev' | 'manual' | string;
	readonly name: string;
	readonly description: string;
	readonly family: string;
	readonly category: string;
	readonly api_format: string;
	readonly base_url: string;
	readonly modelsdev_ref: string;
	readonly match: 'exact' | 'normalized' | 'vendor' | 'manual' | 'none' | string;
	readonly context_window: number | null;
	readonly max_input: number | null;
	readonly max_output: number | null;
	// context_layers is the context window each layer states, which is what
	// lets the picker offer to fall back to the provider or models.dev value.
	readonly context_layers: ContextLayers;
	// context_source names the layer the effective context window came from.
	readonly context_source: string;
	// max_output_layers is the max output each layer states, and
	// max_output_source names the layer the effective value came from.
	readonly max_output_layers: ContextLayers;
	readonly max_output_source: string;
	readonly capabilities: Capabilities;
	// capability_override is the operator's own tools, reasoning, and vision.
	// A null field is one they have not set, so the row can mark a forced value.
	readonly capability_override: Capabilities;
	readonly status: string;
	readonly release_date: string;
	readonly enabled: boolean;
	readonly available: boolean | null;
	readonly routable: boolean;
	readonly unroutable_reason: string;
	// serving_accounts and active_accounts report how many accounts behind the
	// connection could take a request now and how many of them can serve this
	// model, which is what tells a model that lists from one that routes.
	readonly serving_accounts?: number;
	readonly active_accounts?: number;
	readonly prices: ModelPrices;
	readonly group_refs: string[] | null;
	// overridden reports whether an operator changed anything about this
	// model, or picked the model it is priced as.
	readonly overridden: boolean;
	readonly details?: ModelDetails;
	readonly listed_at_ms?: number;
	readonly updated_at_ms: number;
}

/** One model a group may route to. */
export interface GroupMember {
	readonly provider_id: string;
	readonly model_id: string;
	// kind names how the member resolves: one connection's own model, or a
	// bare identifier the router follows across every connection serving it.
	// A member stored before routes could name a bare id has no kind, and is
	// read as one connection's model.
	readonly kind?: 'model' | 'auto' | string;
	readonly weight: number;
	readonly enabled: boolean;
	readonly eligible: boolean;
	readonly reason?: string;
	// serving_accounts and active_accounts are what tells a member that lists
	// from one that routes: the accounts that could take a request now, and
	// how many of them can serve the model.
	readonly serving_accounts?: number;
	readonly active_accounts?: number;
}

/** The name several models answer, with the rule that picks one. */
export interface Group {
	readonly id: string;
	readonly label: string;
	readonly strategy: 'priority' | 'round-robin' | 'weighted' | 'cheapest' | 'fastest' | string;
	readonly enabled: boolean;
	readonly listed: boolean;
	// switch_on_4xx and switch_on_5xx report whether this route moves a request
	// to its next member after a status the relay does not always retry.
	readonly switch_on_4xx: boolean;
	readonly switch_on_5xx: boolean;
	// client_id is the name an agent asks for: the route's identifier under
	// the namespace Relo publishes, computed by the daemon so the console
	// never reimplements the slug rule.
	readonly client_id?: string;
	readonly shadows_model: boolean;
	readonly members: GroupMember[] | null;
	// context_window and max_output are the budget the group can promise
	// whichever member answers: the smallest the enabled members state.
	readonly context_window?: number | null;
	readonly max_output?: number | null;
	// capability_warnings names each capability the group cannot promise to a
	// client reading the published model list: reasoning_off, vision_off, or
	// tools_mixed.
	readonly capability_warnings?: string[] | null;
	// supports_tools, supports_reasoning and supports_vision are what the group
	// can promise whichever member answers. True when a member states the
	// capability and none deny it, false when any member denies it, and null
	// when none state it.
	readonly supports_tools?: boolean | null;
	readonly supports_reasoning?: boolean | null;
	readonly supports_vision?: boolean | null;
	readonly created_at_ms: number;
	readonly updated_at_ms: number;
}

/** One member the daemon would try, in order. */
export interface RouteCandidate {
	readonly provider_id: string;
	readonly model_id: string;
	readonly eligible: boolean;
	readonly reason?: string;
	readonly warnings?: string[] | null;
	readonly prices?: Prices;
}

/** How a request to a group would resolve right now. */
export interface RoutePreview {
	readonly model: string;
	readonly kind: string;
	readonly group_id?: string;
	readonly candidates: RouteCandidate[] | null;
	readonly skipped?: RouteCandidate[] | null;
	readonly warnings?: RouteCandidate[] | null;
}

/** One credential a connection signs in with. */
export interface Account {
	readonly id: string;
	readonly provider_id: string;
	readonly kind: string;
	readonly label: string;
	readonly status: 'active' | 'paused' | 'needs_reauth' | string;
	readonly priority: number;
	readonly secret_mask?: string;
	// limit_state is the credential breaker's live state: ready, limited
	// while a backoff runs, or probing on the half-open trial.
	readonly limit_state?: 'ready' | 'limited' | 'probing' | string;
	readonly limited_until_ms?: number;
	// models_known is true once this account's own model list has been read,
	// and models_count is how many models it holds.
	readonly models_known?: boolean;
	readonly models_count?: number;
}

/** What one account is known to serve. */
export interface AccountModels {
	readonly credential_id: string;
	readonly provider_id: string;
	readonly known: boolean;
	readonly source?: string;
	readonly observed_at_ms?: number;
	readonly models: string[];
}

/** One model as one account reads it: the window that account is served with, and whether the
 *  account is known to serve it. */
export interface AccountContextModel {
	readonly model_id: string;
	readonly context_window: number | null;
	readonly override: number | null;
	readonly inherited: number | null;
	readonly max_input: number | null;
	readonly listed: boolean;
}

/** One account's effective context windows. */
export interface AccountContext {
	readonly credential_id: string;
	readonly provider_id: string;
	readonly models: AccountContextModel[];
}

/** One coding agent Relo can wire to itself. */
export interface IntegrationView {
	readonly id: string;
	readonly client: string;
	readonly summary: string;
	readonly protocol: 'openai' | 'anthropic' | string;
	readonly base_url?: string;
	readonly manages_files: boolean;
	readonly enabled: boolean;
	readonly state: 'off' | 'on' | 'drifted' | 'error' | string;
	readonly last_error?: string;
	readonly key?: IntegrationKey;
	readonly files: IntegrationFile[];
	readonly preview?: IntegrationPlan[];
	// steps are what an operator does by hand for a client Relo does not
	// configure itself, already filled in with this machine's address, the
	// hint of the key the integration owns, and a model Relo publishes.
	readonly steps?: string[];
	readonly updated_at_ms?: number;
	// models_stale is true when Relo's published model list changed after
	// setup, so the card offers Repair even when the client's own files
	// still match.
	readonly models_stale?: boolean;
	// restart_needed is true when Codex's app-server started before Relo
	// last wrote the catalog, so the picker still holds the old list.
	readonly restart_needed?: boolean;
	// env_key is the variable the client's own file reads, when that file
	// names a reference rather than the secret.
	readonly env_key?: string;
	// context_1m is the Codex card's 1M button. Highlighted means setup
	// writes the 1M window. Other agents omit it.
	readonly context_1m?: boolean;
}

/** The client key an agent was given, and the hint that identifies it. */
export interface IntegrationKey {
	readonly id: string;
	readonly name: string;
	readonly token_hint: string;
	readonly status: 'active' | 'expired' | 'revoked' | string;
	readonly created_at_ms: number;
	readonly last_used_at_ms?: number;
}

/** One file Relo writes for an agent, and whether it is still what Relo would write. */
export interface IntegrationFile {
	readonly kind: string;
	readonly path: string;
	readonly present: boolean;
	readonly drifted: boolean;
	readonly snapshot_path?: string;
}

/** One file a set-up would write, with the fragment it would put there, so the console can show
 *  it before anything changes. */
export interface IntegrationPlan {
	readonly kind: string;
	readonly path: string;
	readonly fragment?: string;
	readonly refused?: boolean;
	readonly reason?: string;
	readonly code?: string;
}

/** What one setup, repair or rotation did. */
export interface IntegrationResult {
	readonly integration: IntegrationView;
	readonly token?: string;
}

/** One line of an agent's verification. */
export interface IntegrationVerifyCheck {
	readonly name: 'key' | 'env' | 'settings' | 'config' | string;
	readonly ok: boolean;
	readonly detail?: string;
	// fixed is true when verify wrote the check's subject back itself: a
	// missing settings file or env block, never a changed file.
	readonly fixed?: boolean;
}

/** The wiring check for one agent. */
export interface IntegrationVerify {
	readonly ok: boolean;
	readonly url: string;
	readonly status: number;
	readonly models: number;
	readonly message: string;
	// config is true when verify also confirmed the provider block in the
	// agent's own file.
	readonly config?: boolean;
	// checks are the wiring behind the key, one entry per check verify ran.
	readonly checks?: IntegrationVerifyCheck[];
}

/** One model the data plane offers the agent, in the shape the agent's own protocol reports it. */
export interface IntegrationModel {
	readonly id: string;
	readonly name?: string;
	// context_window is the window the data plane advertises for this model,
	// absent when the listing stated none.
	readonly context_window?: number | null;
	// provider_id and source_model_id are the connection and the provider's own
	// identifier the listing reported beside the public name, so a surface
	// attributes an entry without decoding the name it was published under.
	readonly provider_id?: string;
	readonly source_model_id?: string;
}

/** What one integration's key reaches: the address that answered, its status, and the models
 *  that client shape listed. */
export interface IntegrationModels {
	readonly ok: boolean;
	readonly url: string;
	readonly status: number;
	readonly models: IntegrationModel[];
	readonly message: string;
}

/** One test turn sent through an integration's own key. */
export interface IntegrationChat {
	readonly ok: boolean;
	readonly status: number;
	readonly model: string;
	readonly text: string;
	readonly error?: string;
	readonly duration_ms: number;
	// warnings are what Relo doubted about the model it sent to, which the
	// tester shows beside the answer.
	readonly warnings?: string[];
}

/** One turn the Integrations chat tester sends.
 *  The mode picks the target: one connection's model, or a client shape that may follow a route
 *  instead. */
export interface ChatTesterRequest {
	readonly mode: 'direct' | 'format' | string;
	readonly provider_id?: string;
	readonly model_id?: string;
	readonly route_id?: string;
	readonly format?: string;
	readonly system?: string;
	readonly max_tokens?: number;
	// stream asks the daemon to answer frame by frame, which is what lets the
	// console show a reply as it is written.
	readonly stream?: boolean;
	readonly messages: ChatTesterMessage[];
}

/** One message of a test turn. */
export interface ChatTesterMessage {
	readonly role: 'user' | 'assistant' | string;
	readonly content: string;
}

/** One tester turn's result: the text that came back, the model that answered, and the
 *  identifiers that link it to the request log. */
export interface ChatTesterAnswer {
	readonly request_id: string;
	readonly ok: boolean;
	readonly text: string;
	readonly provider?: string;
	readonly model?: string;
	readonly route_reason?: string;
	readonly status: number;
	readonly input_tokens: number;
	readonly output_tokens: number;
	readonly duration_ms: number;
	readonly error?: string;
	readonly error_code?: string;
}

/** One client key as the list reads it. */
export interface AccessKey {
	readonly id: string;
	readonly name: string;
	readonly kind: string;
	readonly client?: string;
	// owner names the integration this key belongs to, and is empty on a key
	// an operator minted for themselves.
	readonly owner?: string;
	readonly token_hint: string;
	readonly generation: number;
	readonly status: 'active' | 'expired' | 'revoked' | string;
	readonly expires_at_ms?: number;
	readonly created_at_ms: number;
	readonly updated_at_ms?: number;
	readonly last_used_at_ms?: number;
}

/** A key the moment it is minted, with the token shown once. */
export interface IssuedAccessKey extends AccessKey {
	readonly token: string;
}

/** Rows are serialized by Go's default struct marshaling, so the field names are the Go names,
 *  capitalized. */
export interface UsageLogRow {
	readonly ID: number;
	readonly RequestID: string;
	readonly TimestampMs: number;
	readonly Provider: string;
	readonly Model: string;
	readonly RequestedModel: string;
	readonly GroupID: string | null;
	readonly CredentialLabel: string;
	readonly Surface: string;
	readonly Status: number;
	readonly DurationMs: number;
	readonly InputTokens: number;
	readonly OutputTokens: number;
	readonly CacheReadTokens: number;
	readonly CacheWriteTokens: number;
	readonly EstimatedCostMicros: number | null;
	readonly RouteProvider: string;
	readonly RouteReason: string;
	readonly ClientKeyID: string;
	readonly ClientKeyName: string;
	// Origin names where a request came from: internal traffic the console's
	// chat tester ran, or external traffic a client sent.
	readonly Origin: string;
}

/** One bucket of the usage report. */
export interface UsageRollupRow {
	readonly Key: string;
	readonly Requests: number;
	readonly Errors: number;
	readonly InputTokens: number;
	readonly OutputTokens: number;
	readonly CacheReadTokens: number;
	readonly CacheWriteTokens: number;
	readonly CostMicros: number;
	readonly UnpricedRequests: number;
	readonly DurationMs: number;
	readonly Attempts: number;
	readonly RetriedRequests: number;
	readonly DurationMaxMs: number;
}

/** One try at one account within a request. */
export interface UsageAttemptRow {
	readonly Ordinal: number;
	readonly Provider: string;
	readonly Model: string;
	readonly CredentialLabel: string;
	readonly CredentialID: string;
	readonly Status: number;
	readonly ErrorCode: string;
	readonly DurationMs: number;
	readonly InputTokens: number;
	readonly OutputTokens: number;
}

/** The envelope every usage read answers with. Archive reports that the range reached past the
 *  request rows retention keeps, so the older part of the range is whole UTC days read from
 *  their aggregate. */
export interface UsageRead {
	readonly items: UsageRollupRow[];
	readonly archive: boolean;
	readonly archive_from_ms?: number;
}

/** Names what one captured message is: the request an agent sent, the request Relo sent to a
 *  provider account, or what it answered. */
export type UsageCaptureKind =
	'agent_request' | 'agent_response' | 'provider_request' | 'provider_response';

/** One stored message without its body: a body is only read when an operator asks for that one
 *  message. */
export interface UsageCapture {
	readonly id: number;
	readonly kind: UsageCaptureKind;
	readonly ordinal?: number;
	readonly method: string;
	readonly url: string;
	readonly status: number;
	readonly headers: Record<string, string[]>;
	readonly body_bytes: number;
	readonly truncated: boolean;
}

/** The metric choice the daemon stored and the metrics it may name. */
export interface UsageMetricsChoice {
	readonly metrics: string[];
	readonly available: string[];
	readonly min: number;
	readonly max: number;
}

/** What the daemon keeps, and for how long. */
export interface RetentionSettings {
	readonly usage_days: number;
	readonly max_events: number;
	readonly max_bytes: number;
}

/** What one maintenance sweep removed: the days it closed, the request rows the budget took, the
 *  captured bodies it purged, and the space the database held before and after compaction. */
export interface SweepReport {
	readonly finalized_days: number;
	readonly rows_deleted: number;
	readonly captures_deleted: number;
	readonly live_bytes_before: number;
	readonly live_bytes_after: number;
	readonly vacuumed: boolean;
}

/** Where the maintenance sweep the console started is: the phase it is in, its report once it is
 *  through, and its error when it failed. */
export interface SweepStatus {
	readonly state: 'idle' | 'working' | 'done' | 'failed';
	readonly started_at_ms?: number;
	readonly ended_at_ms?: number;
	readonly report?: SweepReport;
	readonly error?: string;
}

/** How a quota chart reads: the share an account spent, or the share it has left.
 *  The daemon stores the choice in config.toml, so every browser draws the same one. */
export type QuotaDisplay = 'used' | 'remaining';

/** The whole management configuration the console edits. */
export interface Settings {
	readonly retention: RetentionSettings;
	readonly language: string;
	readonly appearance: { theme: ThemeChoice; accent: Accent; quota_display: QuotaDisplay };
	readonly system: SystemSettings;
	readonly server: ServerSettings;
	readonly providers: ProvidersSettings;
	readonly access: AccessSettings;
	readonly secrets: SecretsSettings;
	readonly restart_pending: boolean;
}

/** When the daemon runs, how it logs, and how it learns about newer builds.
 *  The log level and the update feed apply at the next start; the start-at-login choice applies
 *  at once. */
export interface SystemSettings {
	readonly autostart: boolean;
	readonly log_level: string;
	readonly updates_url: string;
	readonly updates_download: string;
}

/** Where the management listener and the data plane answer. */
export interface ServerSettings {
	readonly bind: string;
	readonly port: number;
	readonly openai_port: number;
	readonly anthropic_port: number;
	readonly gemini_port: number;
}

/** How the daemon reaches providers and their model directory: the outbound proxy, the
 *  models.dev document, and the call, retry, and failover waits. */
export interface ProvidersSettings {
	readonly catalog_url: string;
	readonly proxy_url: string;
	readonly timeout_seconds: number;
	readonly retry_backoff: [number, number][];
	readonly failover_cooldown_seconds: number;
}

/** Who may open the console. The sign-in choice is read-only here: it changes the sign-in model
 *  of the whole console and stays in config.toml. */
export interface AccessSettings {
	readonly allow_external: boolean;
	readonly login: boolean;
}

/** Where provider credentials live. Read-only: editing the vault from the console could strand
 *  every stored credential. */
export interface SecretsSettings {
	readonly keychain: boolean;
	readonly key_file: string;
}

/** The sign-in choices stored for one sign-in connection: token renewal for Claude.ai and
 *  ChatGPT. Whether a model is published twice, at 200K and at 1M, follows from the model
 *  itself, so no stored choice governs it. */
export interface TemplateSettings {
	readonly auto_refresh: boolean;
}

/** What a change to the retention window would delete. */
export interface RetentionPreview {
	readonly Config: RetentionSettings & { UpdatedAtMs?: number };
	readonly AgeCutoffMs: number;
	readonly RowsBefore: number;
	readonly RowsDeleted: number;
	readonly LiveBytesBefore: number;
	readonly LiveBytesAfter: number;
	readonly EstimatedBytesFreed: number;
}

/** A sign-in the daemon is running on an operator's behalf. */
export interface OAuthOperation {
	readonly operation_id: string;
	readonly state: 'running' | 'done' | 'failed' | string;
	readonly provider_id: string;
	readonly url?: string;
	readonly device_code?: string;
	readonly instructions?: string;
	readonly account_id?: string;
	readonly error?: string;
}

/** How far one provider login got, as the callback page polling it is told.
 *  `phase` is the outcome; `error` names the refusal when the login did not finish. */
export interface CallbackStatus {
	readonly provider: string;
	readonly phase: 'connected' | 'failed' | 'denied' | 'state' | 'timeout' | string;
	readonly account?: string;
	readonly error?: string;
}

/** The state of the loopback port one sign-in flow binds.
 *  A provider that registered its redirect address with its vendor can only receive the browser
 *  on that exact port, so a process already listening on it has to be ended before the sign-in
 *  starts. */
export interface OAuthPortStatus {
	readonly busy: boolean;
	readonly port?: number;
	readonly process?: string;
	readonly pid?: number;
}

/** What a model refresh did. */
export interface RefreshResult {
	readonly status: 'updated' | 'skipped' | 'failed' | string;
	readonly listed: number;
	readonly added: number;
	readonly unavailable: number;
	readonly matched: number;
	readonly priced: number;
	readonly modelsdev: ModelsDevState;
}

/** What a download of the models.dev catalog did. */
export interface PricesReport {
	readonly fetched_at_ms: number;
	readonly providers: number;
	readonly models_matched: number;
	readonly prices_changed: number;
	// models_unmatched is how many models the downloaded catalog does not
	// describe. They keep the facts already saved, so the count is what says
	// that some of what a page shows is older than the fetch.
	readonly models_unmatched: number;
	readonly modelsdev: ModelsDevState;
}

/** How one connection fared in an update of every connected provider. */
export interface CatalogRefreshOutcome {
	readonly provider_id: string;
	readonly label: string;
	readonly status: 'updated' | 'skipped' | 'failed' | string;
	readonly detail?: string;
	readonly listed: number;
	readonly added: number;
	readonly unavailable: number;
	readonly matched: number;
	readonly priced: number;
}

/** What the console's one update action did: the model catalog it downloaded and how each
 *  connection fared. */
export interface CatalogRefreshResult {
	readonly metadata: PricesReport;
	readonly metadata_error?: string;
	readonly providers: CatalogRefreshOutcome[];
	readonly updated: number;
	readonly skipped: number;
	readonly failed: number;
}

/** One model in the saved models.dev copy, in the shape a Priced as picker shows it. */
export interface ModelsDevSearchHit {
	readonly ref: string;
	readonly provider_id: string;
	readonly provider_name: string;
	readonly model_id: string;
	readonly name: string;
	readonly context_window: number | null;
	readonly prices: Prices;
}

/** One hit of a models.dev search. */
export interface ModelsDevSearchResult {
	readonly items: ModelsDevSearchHit[];
	readonly modelsdev: ModelsDevState;
}

/** One usage-limit window an account reported, either from a probe or from the headers of a live
 *  response. */
export interface QuotaWindow {
	readonly credential_id: string;
	readonly connection_id: string;
	readonly label: string;
	readonly window: string;
	readonly window_seconds: number;
	readonly used_percent: number;
	readonly amount?: number;
	readonly currency?: string;
	readonly reset_at_ms: number;
	readonly source: string;
	readonly observed_at_ms: number;
	readonly stale: boolean;
}
