import type {
	Account,
	Model,
	ModelPrices,
	Prices,
	Provider,
	ProviderTemplate,
	QuotaWindow
} from './types';

/** An empty rate set, which is what a model with no known price carries. */
export function emptyPrices(): Prices {
	return {
		input: null,
		output: null,
		cache_read: null,
		cache_write: null,
		ext_threshold: null,
		ext_input: null,
		ext_output: null,
		ext_cache_read: null,
		ext_cache_write: null
	};
}

/** Fills the rates a test cares about and leaves the rest unknown. */
export function pricesOf(rates: Partial<Prices>): Prices {
	return { ...emptyPrices(), ...rates };
}

/** Every rate a Prices carries, so a loop can walk a layer by name without pretending
 *  Object.keys hands back more than strings. A name that is not a rate fails the build here. */
const PRICE_KEYS = [
	'input',
	'output',
	'cache_read',
	'cache_write',
	'ext_threshold',
	'ext_input',
	'ext_output',
	'ext_cache_read',
	'ext_cache_write'
] as const satisfies readonly (keyof Prices)[];

/** Layers one set of rates as the effective one, which is what a list row reads. */
export function modelPrices(override: Prices, provider: Prices, modelsdev: Prices): ModelPrices {
	const effective = { ...emptyPrices() };
	const source: Record<string, string> = {};
	for (const key of PRICE_KEYS) {
		if (override[key] !== null) {
			effective[key] = override[key];
			source[key] = 'override';
		} else if (provider[key] !== null) {
			effective[key] = provider[key];
			source[key] = 'provider';
		} else if (modelsdev[key] !== null) {
			effective[key] = modelsdev[key];
			source[key] = 'modelsdev';
		}
	}
	return { override, provider, modelsdev, effective, effective_source: source };
}

/** Builds a connection for a test, with the fields a case does not name filled in. */
export function providerOf(patch: Partial<Provider> = {}): Provider {
	return {
		id: 'openai',
		template_id: 'openai',
		label: 'OpenAI',
		kind: 'key',
		available_formats: null,
		variable_defs: null,
		origin: 'template',
		auth: 'api_key',
		api_format: 'openai-chat',
		api_formats: ['openai-chat'],
		key_header: 'bearer',
		models_source: 'listing',
		models_format: 'openai',
		modelsdev_provider_id: 'openai',
		base_url: 'https://api.openai.com/v1',
		doc_url: '',
		key_env: [],
		login_flows: [],
		headers: {},
		variables: {},
		needs_setup: [],
		routable: true,
		unroutable_reason: '',
		enabled: true,
		use_proxy: false,
		timeout_seconds: null,
		retry_backoff: null,
		switch_on_4xx: true,
		switch_on_5xx: true,
		rank: 100,
		pool_strategy: 'least-loaded',
		configured: true,
		counts: {
			models: 3,
			enabled_models: 3,
			available_models: 3,
			unpriced_models: 0,
			accounts: 1,
			active_accounts: 1,
			paused_accounts: 0,
			reauth_accounts: 0
		},
		created_at_ms: 1,
		updated_at_ms: 1,
		...patch
	};
}

/** Builds a provider template for a test. */
export function templateOf(patch: Partial<ProviderTemplate> = {}): ProviderTemplate {
	return {
		id: 'openai',
		label: 'OpenAI',
		kind: 'key',
		origin: 'template',
		auth: 'api_key',
		key_header: 'bearer',
		default_format: 'openai-chat',
		available_formats: [
			{
				format: 'openai-chat',
				default_base_url: 'https://api.openai.com/v1',
				key_header: 'bearer',
				models_format: 'openai',
				label: 'OpenAI Chat Completions'
			}
		],
		default_base_url: 'https://api.openai.com/v1',
		models_source: 'listing',
		models_format: 'openai',
		modelsdev_models: 42,
		...patch
	};
}

/** Builds the template a sign-in flow test needs. */
export function signInTemplate(patch: Partial<ProviderTemplate> = {}): ProviderTemplate {
	return templateOf({
		id: 'claude',
		label: 'Claude',
		kind: 'signin',
		auth: 'oauth',
		login_methods: [{ flow: 'claude', kind: 'browser' }],
		login_flows: ['claude'],
		...patch
	});
}

/** Builds a model row for a test. */
export function modelOf(patch: Partial<Model> = {}): Model {
	return {
		provider_id: 'openai',
		model_id: 'gpt-5',
		upstream_model_id: 'gpt-5',
		cloned_from: '',
		source: 'listing',
		name: 'GPT-5',
		description: '',
		family: 'gpt',
		category: 'chat',
		api_format: 'openai-chat',
		base_url: '',
		modelsdev_ref: 'openai/gpt-5',
		match: 'exact',
		context_window: 400000,
		max_input: null,
		max_output: 128000,
		context_layers: { override: null, provider: 400000, modelsdev: null },
		context_source: 'provider',
		max_output_layers: { override: null, provider: 128000, modelsdev: null },
		max_output_source: 'provider',
		capabilities: { tools: true, reasoning: null, vision: null },
		capability_override: { tools: null, reasoning: null, vision: null },
		status: 'active',
		release_date: '',
		enabled: true,
		available: true,
		routable: true,
		unroutable_reason: '',
		prices: modelPrices(
			pricesOf({ input: 1_250_000 }),
			emptyPrices(),
			pricesOf({ output: 10_000_000 })
		),
		group_refs: null,
		overridden: false,
		updated_at_ms: 1,
		...patch
	};
}

/** Builds an account for a test. */
export function accountOf(patch: Partial<Account> = {}): Account {
	return {
		id: 'acc-1',
		provider_id: 'openai',
		kind: 'api_key',
		label: 'default',
		status: 'active',
		priority: 0,
		secret_mask: 'sk-…abcd',
		...patch
	};
}

/** One limit an account reported, with the short burst window a probe usually answers with. */
export function quotaWindowOf(patch: Partial<QuotaWindow> = {}): QuotaWindow {
	return {
		credential_id: 'acc-1',
		connection_id: 'openai',
		label: 'default',
		window: '5h',
		window_seconds: 18_000,
		used_percent: 50,
		reset_at_ms: 0,
		source: 'probe',
		observed_at_ms: 0,
		stale: false,
		...patch
	};
}
