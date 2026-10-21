import { describe, expect, it } from 'vitest';
import {
	DASHBOARD_LIVE_KEY,
	accountAttention,
	accountHealth,
	deltaPercent,
	formatCountdown,
	healthCards,
	isKiloFree,
	isTokenPlan,
	isWindowPeriod,
	paidProviders,
	planProviders,
	readDashboardLive,
	rowTokens,
	boardOverflowLabel,
	spendKindOf,
	spendRows,
	spendSets,
	showsDayLabels,
	summaryWindow,
	topSpendRows,
	visibleBoard,
	writeDashboardLive,
	type LiveStorage,
	type SpendRow
} from './dashboard';
import type { Account, Provider, QuotaWindow, UsageRollupRow } from './types';

const now = Date.parse('2026-09-27T12:00:00Z');

function provider(id: string, kind: string): Provider {
	return {
		id,
		template_id: id,
		label: id,
		kind,
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
		retry_attempts: null,
		switch_on_4xx: true,
		switch_on_5xx: true,
		rank: 0,
		pool_strategy: 'least-loaded',
		configured: true,
		counts: {
			models: 4,
			enabled_models: 4,
			available_models: 4,
			unpriced_models: 0,
			accounts: 1,
			active_accounts: 1,
			paused_accounts: 0,
			reauth_accounts: 0
		},
		created_at_ms: 0,
		updated_at_ms: 0
	};
}

function account(overrides: Partial<Account> = {}): Account {
	return {
		id: 'a1',
		provider_id: 'openai',
		kind: 'api_key',
		label: 'work',
		status: 'active',
		priority: 0,
		...overrides
	};
}

function window(overrides: Partial<QuotaWindow> = {}): QuotaWindow {
	return {
		credential_id: 'a1',
		connection_id: 'openai',
		label: 'work',
		window: '5h',
		window_seconds: 18000,
		used_percent: 50,
		reset_at_ms: 0,
		source: 'probe',
		observed_at_ms: 0,
		stale: false,
		...overrides
	};
}

function rollup(key: string, overrides: Partial<UsageRollupRow> = {}): UsageRollupRow {
	return {
		Key: key,
		Requests: 10,
		Errors: 0,
		InputTokens: 100,
		OutputTokens: 20,
		CacheReadTokens: 5,
		CacheWriteTokens: 5,
		CostMicros: 1000,
		UnpricedRequests: 0,
		DurationMs: 0,
		Attempts: 0,
		RetriedRequests: 0,
		DurationMaxMs: 0,
		...overrides
	};
}

describe('paidProviders', () => {
	it('keeps API key and cloud connections only', () => {
		const list = [
			provider('openai', 'key'),
			provider('bedrock', 'cloud'),
			provider('claude', 'signin'),
			provider('ollama', 'local')
		];
		expect(paidProviders(list).map((p) => p.id)).toEqual(['openai', 'bedrock']);
		expect(planProviders(list).map((p) => p.id)).toEqual(['claude']);
	});

	it('leaves OpenCode out of paid spend and makes it plan usage', () => {
		const list = [
			provider('openai', 'key'),
			{ ...provider('go', 'key'), template_id: 'opencode-go' },
			{
				...provider('custom-go', 'key'),
				template_id: 'custom',
				base_url: 'https://opencode.ai/zen/go/v1'
			},
			{ ...provider('free', 'key'), template_id: 'opencode-free' },
			{
				...provider('free-custom', 'key'),
				template_id: 'custom',
				base_url: 'https://opencode.ai/zen/v1'
			}
		];
		expect(paidProviders(list).map((p) => p.id)).toEqual(['openai']);
		expect(planProviders(list).map((p) => p.id)).toEqual([
			'go',
			'custom-go',
			'free',
			'free-custom'
		]);
		const rows = spendRows(paidProviders(list), [
			rollup('go', { CostMicros: 9000 }),
			rollup('openai', { CostMicros: 100 })
		]);
		expect(rows.map((row) => row.provider.id)).toEqual(['openai']);
		expect(rows[0].costMicros).toBe(100);
	});

	it('leaves the keyless Kilo pool out of paid spend', () => {
		const list = [
			provider('openai', 'key'),
			{ ...provider('kilo', 'key'), template_id: 'kilo-free', modelsdev_provider_id: 'kilo' }
		];
		expect(paidProviders(list).map((p) => p.id)).toEqual(['openai']);
		expect(planProviders(list).map((p) => p.id)).toEqual(['kilo']);
		expect(isKiloFree(provider('kilo', 'key'))).toBe(false);
	});

	it('classifies sign-in accounts, token plans, and OpenCode as plan usage', () => {
		expect(spendKindOf(provider('openai', 'key'))).toBe('paid');
		expect(spendKindOf(provider('bedrock', 'cloud'))).toBe('paid');
		expect(spendKindOf(provider('claude', 'signin'))).toBe('plan');
		expect(spendKindOf(provider('ollama', 'local'))).toBe('local');
		expect(spendKindOf({ ...provider('x', 'key'), id: 'xiaomi-token-plan-sgp' })).toBe('plan');
		expect(
			spendKindOf({ ...provider('y', 'key'), base_url: 'https://token-plan-ams.xiaomimimo.com/v1' })
		).toBe('plan');
		expect(spendKindOf({ ...provider('z', 'key'), modelsdev_provider_id: 'opencode' })).toBe(
			'plan'
		);
		expect(isTokenPlan({ ...provider('x', 'key'), template_id: 'xiaomi-token-plan-cn' })).toBe(
			true
		);
		expect(isTokenPlan(provider('openai', 'key'))).toBe(false);
	});
});

describe('spendRows', () => {
	it('joins the rollup with the paid connections the caller selected', () => {
		const providers = [provider('openai', 'key'), provider('claude', 'signin')];
		const rows = [rollup('openai', { CostMicros: 2500, UnpricedRequests: 2 })];
		const rowsOut = spendRows(paidProviders(providers), rows);
		expect(rowsOut).toHaveLength(1);
		expect(rowsOut[0].provider.id).toBe('openai');
		expect(rowsOut[0].costMicros).toBe(2500);
		expect(rowsOut[0].unpriced).toBe(2);
		expect(rowsOut[0].tokens).toBe(130);
	});

	it('keeps a plan row at zero until its cost is read', () => {
		const providers = [provider('claude', 'signin')];
		const rowsOut = spendRows(planProviders(providers), []);
		expect(rowsOut).toHaveLength(1);
		expect(rowsOut[0].costMicros).toBe(0);
	});
});

describe('topSpendRows', () => {
	function row(id: string, costMicros: number): SpendRow {
		return {
			provider: { ...provider(id, 'key'), label: id },
			requests: 10,
			errors: 0,
			costMicros,
			unpriced: 0,
			tokens: 0
		};
	}

	it('names the costliest connections first', () => {
		const rows = topSpendRows([row('openai', 100), row('anthropic', 5000), row('google', 900)]);

		expect(rows.map((entry) => entry.provider.id)).toEqual(['anthropic', 'google', 'openai']);
	});

	it('keeps the top few of them', () => {
		const rows = topSpendRows([
			row('openai', 100),
			row('anthropic', 5000),
			row('google', 900),
			row('mistral', 700)
		]);

		expect(rows).toHaveLength(3);
		expect(rows.map((entry) => entry.provider.id)).toEqual(['anthropic', 'google', 'mistral']);
	});
});

describe('spendSets', () => {
	it('splits the connections into the three cards', () => {
		const sets = spendSets([
			provider('openai', 'key'),
			provider('claude', 'signin'),
			{ ...provider('ollama', 'local') }
		]);

		expect(sets.paid.map((entry) => entry.id)).toEqual(['openai']);
		expect(sets.plan.map((entry) => entry.id)).toEqual(['claude']);
		expect(sets.free).toHaveLength(0);
	});

	it('keeps a keyless lane out of the paid and the plan cards', () => {
		const sets = spendSets([
			provider('openai', 'key'),
			{
				...provider('opencode', 'key'),
				template_id: 'opencode-free',
				modelsdev_provider_id: 'opencode',
				auth: 'none'
			}
		]);

		expect(sets.paid.map((entry) => entry.id)).toEqual(['openai']);
		expect(sets.plan).toHaveLength(0);
		expect(sets.free.map((entry) => entry.id)).toEqual(['opencode']);
	});
});

describe('rowTokens', () => {
	it('adds cache traffic to input and output', () => {
		expect(rowTokens(rollup('x'))).toBe(130);
	});
});

describe('deltaPercent', () => {
	it('compares two periods and refuses to divide by zero', () => {
		expect(deltaPercent(120, 100)).toBe(20);
		expect(deltaPercent(50, 100)).toBe(-50);
		expect(deltaPercent(10, 0)).toBeNull();
	});
});

describe('accountHealth', () => {
	it('puts a sign-in that must be repeated first', () => {
		const health = accountHealth(account({ status: 'needs_reauth' }), [], now);
		expect(health.state).toBe('needs_signin');
	});

	it('reports the live rate limit with its resume time', () => {
		const until = now + 4 * 60_000;
		const health = accountHealth(
			account({ limit_state: 'limited', limited_until_ms: until }),
			[],
			now
		);
		expect(health.state).toBe('limited');
		expect(health.limitedUntilMs).toBe(until);
	});

	it('ranks full windows exhausted and near-full windows near', () => {
		expect(accountHealth(account(), [window({ used_percent: 100 })], now).state).toBe('exhausted');
		expect(accountHealth(account(), [window({ used_percent: 85 })], now).state).toBe('near');
		expect(accountHealth(account(), [window({ used_percent: 10 })], now).state).toBe('ready');
		expect(
			accountHealth(account(), [window({ used_percent: 100, amount: 0, currency: 'USD' })], now)
				.state
		).toBe('ready');
	});

	it('keeps the soonest reset that still matters', () => {
		const health = accountHealth(
			account(),
			[
				window({ reset_at_ms: now + 3_600_000 }),
				window({ window: '7d', reset_at_ms: now + 60_000 })
			],
			now
		);
		expect(health.nextResetMs).toBe(now + 60_000);
	});
});

describe('accountAttention', () => {
	it('raises the accounts whose window is full or nearly full', () => {
		const items = accountAttention(
			[
				account({ id: 'full', label: 'hi@claude.ai' }),
				account({ id: 'near', label: 'Gecko Brisk' }),
				account({ id: 'calm', label: 'calm' })
			],
			[
				window({ credential_id: 'full', used_percent: 100 }),
				window({ credential_id: 'near', used_percent: 96 }),
				window({ credential_id: 'calm', used_percent: 40 })
			],
			now
		);

		expect(items.map((item) => item.account.id)).toEqual(['full', 'near']);
		expect(items[0].reason).toBe('reached');
		expect(items[0].tone).toBe('danger');
		expect(items[1].reason).toBe('near');
		expect(items[1].tone).toBe('warn');
	});

	it('raises a sign-in that expired before a window that is nearly full', () => {
		const items = accountAttention(
			[
				account({ id: 'near', label: 'near' }),
				account({ id: 'gated', label: 'gated', status: 'needs_reauth' })
			],
			[window({ credential_id: 'near', used_percent: 95 })],
			now
		);

		expect(items.map((item) => item.account.id)).toEqual(['gated', 'near']);
		expect(items[0].reason).toBe('needs_signin');
		expect(items[0].tone).toBe('danger');
	});

	it('raises a paused and a rate limited account', () => {
		const items = accountAttention(
			[
				account({ id: 'paused', status: 'paused' }),
				account({ id: 'backoff', limit_state: 'limited', limited_until_ms: now })
			],
			[],
			now
		);

		expect(items.map((item) => item.reason)).toEqual(['paused', 'limited']);
	});

	it('reads a balance as a fact and not as a limit', () => {
		const items = accountAttention(
			[account()],
			[window({ used_percent: 100, amount: 7.48, currency: 'USD' })],
			now
		);

		expect(items).toEqual([]);
	});

	it('opens the connection the account belongs to', () => {
		const items = accountAttention(
			[account({ provider_id: 'anthropic' })],
			[window({ used_percent: 100 })],
			now
		);

		expect(items[0].providerId).toBe('anthropic');
	});
});

describe('healthCards', () => {
	it('joins each account with its connection and its quota reading', () => {
		const cards = healthCards(
			[account(), account({ id: 'a2', provider_id: 'other' })],
			[provider('openai', 'key')],
			[window({ used_percent: 85 })],
			now
		);
		// The account whose connection is not configured has nothing to show and
		// is left off the board.
		expect(cards).toHaveLength(1);
		expect(cards[0].account.id).toBe('a1');
		expect(cards[0].provider?.id).toBe('openai');
		expect(cards[0].health.state).toBe('near');
	});

	it('puts the accounts that need attention first', () => {
		const cards = healthCards(
			[
				account({ id: 'calm' }),
				account({ id: 'gated', status: 'needs_reauth' }),
				account({ id: 'busy' })
			],
			[provider('openai', 'key')],
			[window({ credential_id: 'busy', used_percent: 100 })],
			now
		);
		expect(cards.map((card) => card.account.id)).toEqual(['gated', 'busy', 'calm']);
	});
});

describe('formatCountdown', () => {
	it('renders the coarsest units that still tick', () => {
		expect(formatCountdown(0)).toBe('');
		expect(formatCountdown(30_000)).toBe('1m');
		expect(formatCountdown(4 * 60_000)).toBe('4m');
		expect(formatCountdown((2 * 3600 + 14 * 60) * 1000)).toBe('2h 14m');
		expect(formatCountdown(3 * 1440 * 60_000)).toBe('3d 0h');
	});
});

function memoryStorage(seed: Record<string, string> = {}): LiveStorage {
	const held = new Map(Object.entries(seed));
	return {
		getItem: (key) => held.get(key) ?? null,
		setItem: (key, value) => void held.set(key, value)
	};
}

const deniedStorage: LiveStorage = {
	getItem() {
		throw new Error('denied');
	},
	setItem() {
		throw new Error('denied');
	}
};

describe('board cap', () => {
	it('keeps six and marks a seventh as 6+', () => {
		const items = [1, 2, 3, 4, 5, 6, 7];
		expect(visibleBoard(items)).toEqual([1, 2, 3, 4, 5, 6]);
		expect(boardOverflowLabel(items.length)).toBe('6+');
		expect(boardOverflowLabel(6)).toBe('');
	});
});

describe('summaryWindow', () => {
	it('reads the last 24 hours as a rolling window with the day before it', () => {
		const window = summaryWindow('24h', now);
		expect(window.start).toBe(now - 86_400_000);
		expect(window.until).toBe(0);
		expect(window.prevStart).toBe(now - 2 * 86_400_000);
		expect(window.prevUntil).toBe(now - 86_400_000);
	});

	it('floors longer windows to UTC days and compares the equal window before', () => {
		const dayStart = Date.parse('2026-09-27T00:00:00Z');
		const week = summaryWindow('7d', now);
		expect(week.start).toBe(dayStart - 6 * 86_400_000);
		expect(week.until).toBe(0);
		expect(week.prevStart).toBe(dayStart - 13 * 86_400_000);
		expect(week.prevUntil).toBe(dayStart - 6 * 86_400_000);

		const month = summaryWindow('30d', now);
		expect(month.start).toBe(dayStart - 29 * 86_400_000);
		expect(month.prevStart).toBe(dayStart - 59 * 86_400_000);
		expect(month.prevUntil).toBe(dayStart - 29 * 86_400_000);

		const sixty = summaryWindow('60d', now);
		expect(sixty.start).toBe(dayStart - 59 * 86_400_000);
		expect(sixty.prevStart).toBe(dayStart - 119 * 86_400_000);

		const ninety = summaryWindow('90d', now);
		expect(ninety.start).toBe(dayStart - 89 * 86_400_000);
		expect(ninety.prevStart).toBe(dayStart - 179 * 86_400_000);
	});

	it('reads today from midnight against the same span of yesterday', () => {
		const dayStart = Date.parse('2026-09-27T00:00:00Z');
		const today = summaryWindow('today', now);
		expect(today.start).toBe(dayStart);
		expect(today.until).toBe(0);
		expect(today.prevStart).toBe(dayStart - 86_400_000);
		expect(today.prevUntil).toBe(now - 86_400_000);
	});

	it('leaves all time without a previous window', () => {
		const window = summaryWindow('all', now);
		expect(window.start).toBe(0);
		expect(window.until).toBe(0);
		expect(window.prevUntil).toBe(0);
	});
});

describe('showsDayLabels', () => {
	it('names the days of a window of a week or less', () => {
		expect(showsDayLabels('24h')).toBe(true);
		expect(showsDayLabels('today')).toBe(true);
		expect(showsDayLabels('7d')).toBe(true);
	});

	it('leaves the days unnamed once the window runs past a week', () => {
		expect(showsDayLabels('30d')).toBe(false);
		expect(showsDayLabels('60d')).toBe(false);
		expect(showsDayLabels('90d')).toBe(false);
		expect(showsDayLabels('all')).toBe(false);
	});
});

describe('dashboard live state', () => {
	it('is live without a store or without a settled choice', () => {
		expect(readDashboardLive(undefined)).toBe(true);
		expect(readDashboardLive(memoryStorage())).toBe(true);
	});

	it('round-trips the chosen state', () => {
		const storage = memoryStorage();
		writeDashboardLive(storage, false);
		expect(storage.getItem(DASHBOARD_LIVE_KEY)).toBe('off');
		expect(readDashboardLive(storage)).toBe(false);
		writeDashboardLive(storage, true);
		expect(readDashboardLive(storage)).toBe(true);
	});

	it('stays live when the store refuses a read or a write', () => {
		expect(readDashboardLive(deniedStorage)).toBe(true);
		expect(() => writeDashboardLive(deniedStorage, false)).not.toThrow();
		expect(() => writeDashboardLive(undefined, false)).not.toThrow();
	});
});

describe('the guard over the summary window', () => {
	it('accepts every window the control offers and nothing else', () => {
		for (const period of ['24h', 'today', '7d', '30d', '60d', '90d', 'all']) {
			expect(isWindowPeriod(period)).toBe(true);
		}
		expect(isWindowPeriod('14d')).toBe(false);
		expect(isWindowPeriod('')).toBe(false);
	});
});
