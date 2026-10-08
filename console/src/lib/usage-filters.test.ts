import { describe, expect, it } from 'vitest';

import type { Provider, UsageRollupRow } from './types';
import {
	activeFilterCount,
	clearRollupSpend,
	clearSummarySpend,
	DEFAULT_USAGE_FILTERS,
	hasFilters,
	isUsageOrigin,
	isUsageRange,
	isUsageTab,
	nonPaidProviderIds,
	planProviderIds,
	providerPlanMicros,
	spendScopeOf,
	splitSpend,
	statusParam,
	startOfUTCDay,
	subtractRollupSpend,
	subtractSummarySpend,
	totalsOf,
	usageProvidersQuery,
	usageQuery,
	usageSinceMs,
	type UsageSummary
} from './usage-filters';

const nowMs = 1_700_000_000_000;

describe('statusParam', () => {
	it('maps a choice to the codes the API accepts', () => {
		expect(statusParam('')).toBe('');
		expect(statusParam('ok')).toBe('2xx');
		expect(statusParam('error')).toBe('4xx,5xx');
	});
});

describe('startOfUTCDay', () => {
	it('floors one instant to its UTC midnight', () => {
		expect(startOfUTCDay(nowMs)).toBe(1_699_920_000_000);
		expect(startOfUTCDay(1_699_920_000_000)).toBe(1_699_920_000_000);
	});
});

describe('usageSinceMs', () => {
	it('is zero for all time and starts the rest on a UTC day', () => {
		expect(usageSinceMs('all', nowMs)).toBe(0);
		// A bounded range covers whole UTC days, today included: seven days
		// starts six back at midnight, the grain the archive answers at.
		expect(usageSinceMs('7', nowMs)).toBe(startOfUTCDay(nowMs) - 6 * 86_400_000);
		expect(usageSinceMs('30', nowMs)).toBe(startOfUTCDay(nowMs) - 29 * 86_400_000);
		expect(usageSinceMs('60', nowMs)).toBe(startOfUTCDay(nowMs) - 59 * 86_400_000);
		expect(usageSinceMs('90', nowMs)).toBe(startOfUTCDay(nowMs) - 89 * 86_400_000);
		// Today starts at midnight, and the rolling day starts 24 hours back.
		expect(usageSinceMs('today', nowMs)).toBe(startOfUTCDay(nowMs));
		expect(usageSinceMs('24h', nowMs)).toBe(nowMs - 86_400_000);
	});
});

describe('usageQuery', () => {
	it('renders defaults with just a group and a range', () => {
		const query = usageQuery(DEFAULT_USAGE_FILTERS, nowMs);
		const params = new URLSearchParams(query);
		expect(params.get('group_by')).toBe('day');
		// The page opens on all time, which the archive answers.
		expect(params.get('since_ms')).toBeNull();
		expect(params.get('provider')).toBeNull();
		expect(params.get('account')).toBeNull();
	});

	it('omits the group for a summary read', () => {
		const query = usageQuery({ ...DEFAULT_USAGE_FILTERS, groupBy: 'account' }, nowMs, false);
		expect(new URLSearchParams(query).get('group_by')).toBeNull();
	});

	it('carries every narrowing filter, and omits all time', () => {
		const query = usageQuery(
			{
				...DEFAULT_USAGE_FILTERS,
				range: 'all',
				groupBy: 'account',
				provider: 'openai',
				account: 'cred-1',
				model: 'gpt-4o',
				client: 'key-1',
				origin: 'internal',
				status: 'error'
			},
			nowMs
		);
		const params = new URLSearchParams(query);
		expect(params.get('group_by')).toBe('account');
		expect(params.get('since_ms')).toBeNull();
		expect(params.get('provider')).toBe('openai');
		expect(params.get('account')).toBe('cred-1');
		expect(params.get('model')).toBe('gpt-4o');
		expect(params.get('client')).toBe('key-1');
		expect(params.get('origin')).toBe('internal');
		expect(params.get('status')).toBe('4xx,5xx');
	});
});

describe('totalsOf', () => {
	it('is empty without a summary', () => {
		expect(totalsOf(null).totalTokens).toBe(0);
	});

	it('adds every token, cache included, into a total', () => {
		const totals = totalsOf({
			requests: 3,
			errors: 1,
			input_tokens: 31,
			output_tokens: 13,
			cache_read_tokens: 5,
			cache_write_tokens: 7,
			cost_micros: 42,
			unpriced_requests: 1,
			duration_ms: 12
		});
		expect(totals.totalTokens).toBe(56);
		expect(totals.spendMicros).toBe(42);
		expect(totals.unpricedRequests).toBe(1);
		expect(totals.cacheReadTokens).toBe(5);
		expect(totals.cacheWriteTokens).toBe(7);
		expect(totals.durationMs).toBe(12);
	});
});

const summary: UsageSummary = {
	requests: 4,
	errors: 0,
	input_tokens: 10,
	output_tokens: 6,
	cache_read_tokens: 0,
	cache_write_tokens: 0,
	cost_micros: 1000,
	unpriced_requests: 0,
	duration_ms: 8
};

function rollup(key: string, cost: number, requests = 1): UsageRollupRow {
	return {
		Key: key,
		Requests: requests,
		Errors: 0,
		InputTokens: 1,
		OutputTokens: 1,
		CacheReadTokens: 0,
		CacheWriteTokens: 0,
		CostMicros: cost,
		UnpricedRequests: 0,
		DurationMs: 1,
		Attempts: 1,
		RetriedRequests: 0,
		DurationMaxMs: 1
	};
}

function provider(overrides: Partial<Provider> = {}): Provider {
	return {
		id: 'openai',
		template_id: 'openai',
		label: 'openai',
		kind: 'key',
		available_formats: null,
		variable_defs: null,
		origin: 'template',
		auth: 'api_key',
		api_format: 'openai',
		api_formats: [],
		key_header: 'Authorization',
		models_source: '',
		models_format: '',
		modelsdev_provider_id: '',
		base_url: '',
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
		rank: 0,
		pool_strategy: 'least-loaded',
		configured: true,
		counts: {
			models: 1,
			enabled_models: 1,
			available_models: 1,
			unpriced_models: 0,
			accounts: 1,
			active_accounts: 1,
			paused_accounts: 0,
			reauth_accounts: 0
		},
		created_at_ms: 0,
		updated_at_ms: 0,
		...overrides
	};
}

describe('spend split', () => {
	it('asks the usage API for each connection of a set', () => {
		const query = usageProvidersQuery(DEFAULT_USAGE_FILTERS, nowMs, ['go', 'go-2']);
		expect(new URLSearchParams(query).getAll('provider')).toEqual(['go', 'go-2']);
	});

	it('names what a filter is scoped to', () => {
		const providers = [
			provider(),
			provider({ id: 'claude', template_id: 'claude', kind: 'signin' }),
			provider({ id: 'plan', template_id: 'opencode-go' }),
			provider({ id: 'ollama', template_id: 'ollama', kind: 'local' })
		];
		const scope = (id: string) =>
			spendScopeOf({ ...DEFAULT_USAGE_FILTERS, provider: id }, providers);
		expect(scope('')).toBe('all');
		expect(scope('openai')).toBe('api');
		expect(scope('claude')).toBe('plan');
		expect(scope('plan')).toBe('plan');
		expect(scope('ollama')).toBe('local');
		// A filter that names nothing stored reads like no filter at all.
		expect(scope('deleted')).toBe('all');

		expect(planProviderIds(providers)).toEqual(['claude', 'plan']);
		expect(nonPaidProviderIds(providers)).toEqual(['claude', 'plan', 'ollama']);
	});

	it('leaves the key as API spend and the rest as plan usage', () => {
		const providers = [
			provider(),
			provider({ id: 'claude', template_id: 'claude', kind: 'signin' }),
			provider({ id: 'token', template_id: 'xiaomi-token-plan-sgp' }),
			provider({ id: 'go', template_id: 'opencode-go' }),
			provider({ id: 'free', template_id: 'opencode-free' })
		];
		const rows = [
			rollup('openai', 100),
			rollup('claude', 200),
			rollup('token', 300),
			rollup('go', 400),
			rollup('free', 500)
		];
		const split = splitSpend(1500, rows, providers);
		expect(split.apiSpendMicros).toBe(100);
		expect(split.planUsageMicros).toBe(1400);
		expect(split.nonPaidMicros).toBe(1400);
	});

	it('keeps a local engine out of both figures', () => {
		const providers = [
			provider(),
			provider({ id: 'ollama', template_id: 'ollama', kind: 'local' })
		];
		const split = splitSpend(900, [rollup('openai', 100), rollup('ollama', 800)], providers);
		expect(split.apiSpendMicros).toBe(100);
		expect(split.planUsageMicros).toBe(0);
		expect(split.nonPaidMicros).toBe(800);
		// A rollup that carries more than the summary leaves, which an archive
		// read can do, still leaves no negative API spend.
		expect(splitSpend(100, [rollup('ollama', 800)], providers).apiSpendMicros).toBe(0);
	});

	it('maps each plan connection to what its own row reports', () => {
		const providers = [
			provider(),
			provider({ id: 'claude', template_id: 'claude', kind: 'signin' }),
			provider({ id: 'go', template_id: 'opencode-go' })
		];
		const rows = [rollup('openai', 100), rollup('claude', 200), rollup('go', 300)];
		expect(providerPlanMicros(rows, providers)).toEqual({ claude: 200, go: 300 });
	});

	it('removes a plan cost from the summary and the matching rows', () => {
		const next = subtractSummarySpend(summary, 400);
		expect(next?.cost_micros).toBe(600);
		expect(next?.requests).toBe(4);
		expect(subtractSummarySpend(summary, 5000)?.cost_micros).toBe(0);

		const rows = [rollup('openai', 700, 2), rollup('go', 300, 2), rollup('gpt-4o', 250)];
		const plan = [rollup('go', 300, 2), rollup('gpt-4o', 250)];
		const subtracted = subtractRollupSpend(rows, plan);
		expect(subtracted.map((row) => row.CostMicros)).toEqual([700, 0, 0]);
		expect(subtracted[1].Requests).toBe(2);
	});

	it('clears spend when the filter names one plan or local connection', () => {
		expect(clearSummarySpend(summary)?.cost_micros).toBe(0);
		expect(clearSummarySpend(summary)?.requests).toBe(4);
		expect(clearRollupSpend([rollup('go', 900)]).map((row) => row.CostMicros)).toEqual([0]);
	});

	it('carries plan usage beside API spend in the headline', () => {
		expect(totalsOf(summary, 400)).toMatchObject({ spendMicros: 1000, planUsageMicros: 400 });
		expect(totalsOf(summary)).toMatchObject({ spendMicros: 1000, planUsageMicros: 0 });
		expect(totalsOf(null, 400).planUsageMicros).toBe(0);
	});
});

describe('hasFilters', () => {
	it('ignores group and range', () => {
		expect(hasFilters(DEFAULT_USAGE_FILTERS)).toBe(false);
		expect(hasFilters({ ...DEFAULT_USAGE_FILTERS, model: 'gpt-4o' })).toBe(true);
	});
});

describe('activeFilterCount', () => {
	it('counts what narrows a read, and nothing else', () => {
		expect(activeFilterCount(DEFAULT_USAGE_FILTERS)).toBe(0);
		expect(activeFilterCount({ ...DEFAULT_USAGE_FILTERS, range: '7', groupBy: 'model' })).toBe(0);
		expect(
			activeFilterCount({
				...DEFAULT_USAGE_FILTERS,
				provider: 'openai',
				account: 'cred-1',
				model: 'gpt-4o',
				client: 'key-1',
				origin: 'internal',
				status: 'error'
			})
		).toBe(6);
	});
});

describe('the guards over a usage filter choice', () => {
	it('accepts only the tabs the report reads', () => {
		expect(isUsageTab('overview')).toBe(true);
		expect(isUsageTab('client')).toBe(true);
		expect(isUsageTab('chart')).toBe(false);
	});

	it('accepts only the ranges and origins the query carries', () => {
		expect(isUsageRange('7')).toBe(true);
		expect(isUsageRange('14')).toBe(false);
		expect(isUsageOrigin('')).toBe(true);
		expect(isUsageOrigin('internal')).toBe(true);
		expect(isUsageOrigin('third-party')).toBe(false);
	});
});
