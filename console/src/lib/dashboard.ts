import type { Account, Provider, QuotaWindow, UsageRollupRow } from './types';
import { isMoneyReading } from './provider-quota';

/** Orders connections the way the dashboard's routing universe reads them: the busiest 24 hours
 *  first, then the connection with more accounts to serve a request. */
export function providersByTraffic(providers: Provider[], rows: UsageRollupRow[]): Provider[] {
	const byTraffic = new Map(rows.map((row) => [row.Key, Number(row.Requests)]));
	return [...providers].sort((a, b) => {
		const ta = byTraffic.get(a.id) ?? 0;
		const tb = byTraffic.get(b.id) ?? 0;
		if (tb !== ta) return tb - ta;
		return b.counts.accounts - a.counts.accounts;
	});
}

// The dashboard reads several daemon answers and combines them into the
// health-first board. These helpers hold the pure decisions: how a
// connection's cost counts, how healthy an account is, and what a number
// changed by.

/** Names how a connection's estimated cost counts: pay-as-you-go API spend, plan usage, or
 *  neither. */
export type SpendKind = 'paid' | 'plan' | 'local';

/** Lists the connections whose usage is API spend: an API key or a cloud credential the operator
 *  tops up. */
export function paidProviders(providers: Provider[]): Provider[] {
	return providers.filter((provider) => spendKindOf(provider) === 'paid');
}

/** Lists the connections a plan covers: sign-in accounts, token-plan APIs, and the keyless free
 *  lanes. */
export function planProviders(providers: Provider[]): Provider[] {
	return providers.filter((provider) => spendKindOf(provider) === 'plan');
}

/** Reports how one connection's cost counts. A plan covers sign-in accounts, token-plan APIs,
 *  the keyless free lanes; a local engine has no per-token price at all, and everything else is
 *  pay-as-you-go. */
export function spendKindOf(
	provider: Pick<Provider, 'kind' | 'id' | 'template_id' | 'modelsdev_provider_id' | 'base_url'>
): SpendKind {
	if (
		provider.kind === 'signin' ||
		isTokenPlan(provider) ||
		isOpenCode(provider) ||
		isKiloFree(provider)
	) {
		return 'plan';
	}
	if (provider.kind === 'key' || provider.kind === 'cloud') return 'paid';
	return 'local';
}

/** Reports a connection billed by the token rather than the request: its identifier or its base
 *  URL host names the plan. */
export function isTokenPlan(
	provider: Pick<Provider, 'id' | 'template_id' | 'modelsdev_provider_id' | 'base_url'>
): boolean {
	for (const id of [provider.id, provider.template_id, provider.modelsdev_provider_id]) {
		if (id.toLowerCase().includes('token-plan')) return true;
	}
	try {
		return new URL(provider.base_url).hostname.toLowerCase().includes('token-plan');
	} catch {
		return false;
	}
}

/** Reports a connection served by OpenCode, Go or Free.
 *  A custom base URL that still points at that gateway counts the same way. */
export function isOpenCode(
	provider: Pick<Provider, 'template_id' | 'modelsdev_provider_id' | 'base_url'>
): boolean {
	for (const id of [provider.template_id, provider.modelsdev_provider_id]) {
		if (id === 'opencode-go' || id === 'opencode-free' || id === 'opencode') return true;
	}
	return openCodeURL(provider.base_url);
}

function openCodeURL(baseURL: string): boolean {
	try {
		return new URL(baseURL).hostname.toLowerCase() === 'opencode.ai';
	} catch {
		return false;
	}
}

/** Reports a connection served by the keyless Kilo pool, which bills no token
 *  because it accepts no account. */
export function isKiloFree(
	provider: Pick<Provider, 'template_id' | 'modelsdev_provider_id'>
): boolean {
	return provider.template_id === 'kilo-free' || provider.modelsdev_provider_id === 'kilo-free';
}

/** One connection on the spend board. */
export interface SpendRow {
	provider: Provider;
	requests: number;
	errors: number;
	costMicros: number;
	unpriced: number;
	tokens: number;
}

/** Joins one provider rollup with the connection it belongs to, so a spend board has one row per
 *  connection the caller selected. */
export function spendRows(providers: Provider[], rows: UsageRollupRow[]): SpendRow[] {
	const byKey = new Map(rows.map((row) => [row.Key, row]));
	return providers.map((provider) => {
		const row = byKey.get(provider.id);
		return {
			provider,
			requests: Number(row?.Requests ?? 0),
			errors: Number(row?.Errors ?? 0),
			costMicros: Number(row?.CostMicros ?? 0),
			unpriced: Number(row?.UnpricedRequests ?? 0),
			tokens:
				Number(row?.InputTokens ?? 0) +
				Number(row?.OutputTokens ?? 0) +
				Number(row?.CacheReadTokens ?? 0) +
				Number(row?.CacheWriteTokens ?? 0)
		};
	});
}

/** Every token one rollup row accounts for. */
export function rowTokens(row: UsageRollupRow): number {
	return (
		Number(row.InputTokens ?? 0) +
		Number(row.OutputTokens ?? 0) +
		Number(row.CacheReadTokens ?? 0) +
		Number(row.CacheWriteTokens ?? 0)
	);
}

/** Reports the change between two totals as a percentage, or null when there is no previous
 *  period to compare with.
 *  A half rounds away from zero, the same rule the daemon's own delta follows. */
export function deltaPercent(current: number, previous: number): number | null {
	if (previous <= 0) return null;
	const change = ((current - previous) / previous) * 100;
	return Math.sign(change) * Math.round(Math.abs(change));
}

/** Where one account stands against its quota. */
export type AccountHealthState =
	'needs_signin' | 'paused' | 'limited' | 'exhausted' | 'near' | 'ready';

/** An account as the health board reads it. */
export interface AccountHealth {
	state: AccountHealthState;
	// nextResetMs is the soonest window reset that matters, and 0 when no
	// window reports one.
	nextResetMs: number;
	limitedUntilMs: number;
}

/** Ranks a health state, so the board shows the accounts that need attention first. */
export const HEALTH_SEVERITY: Record<AccountHealthState, number> = {
	needs_signin: 0,
	paused: 1,
	limited: 2,
	exhausted: 3,
	near: 4,
	ready: 5
};

/** One account as the health board draws it: the account, the connection it belongs to, and
 *  where it stands against its quota. */
export interface HealthCard {
	account: Account;
	provider: Provider | undefined;
	health: AccountHealth;
}

/** Joins the three reads the health board paints from — the accounts, the connections they
 *  belong to, and the quota each one reports — and orders them with the accounts that need
 *  attention first. An account whose connection is not configured is left out. */
export function healthCards(
	accounts: readonly Account[],
	providers: readonly Provider[],
	windows: readonly QuotaWindow[],
	now: number = Date.now()
): HealthCard[] {
	const windowsByAccount = new Map<string, QuotaWindow[]>();
	for (const window of windows) {
		const list = windowsByAccount.get(window.credential_id) ?? [];
		list.push(window);
		windowsByAccount.set(window.credential_id, list);
	}
	return accounts
		.filter((account) => providers.some((provider) => provider.id === account.provider_id))
		.map((account) => ({
			account,
			provider: providers.find((entry) => entry.id === account.provider_id),
			health: accountHealth(account, windowsByAccount.get(account.id) ?? [], now)
		}))
		.sort((a, b) => HEALTH_SEVERITY[a.health.state] - HEALTH_SEVERITY[b.health.state]);
}

/** Names the one state a dashboard card shows. A sign-in that expired or a paused account
 *  outranks a quota window, and the live rate-limit backoff outranks how full the windows are. */
export function accountHealth(
	account: Account,
	windows: readonly QuotaWindow[],
	now: number = Date.now()
): AccountHealth {
	if (account.status === 'needs_reauth')
		return { state: 'needs_signin', nextResetMs: 0, limitedUntilMs: 0 };
	if (account.status === 'paused') return { state: 'paused', nextResetMs: 0, limitedUntilMs: 0 };
	if (account.limit_state === 'limited') {
		return { state: 'limited', nextResetMs: 0, limitedUntilMs: account.limited_until_ms ?? 0 };
	}
	const used = windows.filter(
		(window) => window.credential_id === account.id && !isMoneyReading(window)
	);
	let worst = 0;
	let nextResetMs = 0;
	for (const window of used) {
		if (window.used_percent > worst) worst = window.used_percent;
		if (window.reset_at_ms > now && (nextResetMs === 0 || window.reset_at_ms < nextResetMs)) {
			nextResetMs = window.reset_at_ms;
		}
	}
	const state: AccountHealthState = worst >= 100 ? 'exhausted' : worst >= 80 ? 'near' : 'ready';
	return { state, nextResetMs, limitedUntilMs: 0 };
}

/** Renders a remaining time the way a dashboard reads it: coarse, with the smallest unit first
 *  and nothing below a minute. */
export function formatCountdown(remainingMs: number): string {
	if (remainingMs <= 0) return '';
	const totalMinutes = Math.ceil(remainingMs / 60_000);
	const days = Math.floor(totalMinutes / 1440);
	const hours = Math.floor((totalMinutes % 1440) / 60);
	const minutes = totalMinutes % 60;
	if (days > 0) return days + 'd ' + hours + 'h';
	if (hours > 0) return hours + 'h ' + minutes + 'm';
	return minutes + 'm';
}

/** How many rows a dashboard board shows before the rest are named only as an overflow mark on
 *  the section description. */
export const boardLimit = 6;

/** The paid-spend board is the one board that can grow past the standard cap: an operator with
 *  many API connections still needs to see them all. */
export const spendBoardLimit = 10;

/** The rows a board paints. Anything past the limit stays in the totals and is named by
 *  boardOverflowLabel. */
export function visibleBoard<T>(items: readonly T[], limit: number = boardLimit): T[] {
	return items.slice(0, limit);
}

/** "n+" once a board has more rows than it shows, and empty while every row still fits. */
export function boardOverflowLabel(count: number, limit: number = boardLimit): string {
	return count > limit ? limit + '+' : '';
}

const DAY_MS = 86_400_000;

function utcDayStart(ms: number): number {
	return Math.floor(ms / DAY_MS) * DAY_MS;
}

/** Names the range a summary can read: the rolling day, the current UTC day, a run of whole UTC
 *  days, or everything the ledger holds. */
export type WindowPeriod = '24h' | 'today' | '7d' | '30d' | '60d' | '90d' | 'all';

/** Reports whether a window a control handed back is one the summary can read, so an unexpected
 *  choice leaves the page on the window it was already showing. */
export function isWindowPeriod(value: string): value is WindowPeriod {
	return (
		value === '24h' ||
		value === 'today' ||
		value === '7d' ||
		value === '30d' ||
		value === '60d' ||
		value === '90d' ||
		value === 'all'
	);
}

/** The whole-day windows, in the order the control offers them. */
export const WINDOW_PERIODS: readonly Exclude<WindowPeriod, 'all'>[] = [
	'24h',
	'today',
	'7d',
	'30d',
	'60d',
	'90d'
];

// WINDOW_DAYS is how many UTC days each whole-day window covers, today
// included.
const WINDOW_DAYS: Record<'7d' | '30d' | '60d' | '90d', number> = {
	'7d': 7,
	'30d': 30,
	'60d': 60,
	'90d': 90
};

/** One window of the summary series: its label, its rollups and its totals. */
export interface SummaryWindow {
	// A zero until means "now": the read runs to the moment it is asked.
	start: number;
	until: number;
	// prevStart/prevUntil bound the window of equal length before this one,
	// which is what a delta compares against. All time has none.
	prevStart: number;
	prevUntil: number;
}

/** Begins one window: the last day is rolling like the KPI strip's, today floors to the current
 *  UTC day, and longer windows floor to the UTC day the ledger aggregates by, so a window that
 *  reaches into the archive reads whole days. */
export function windowStart(period: WindowPeriod, now: number): number {
	if (period === '24h') return now - DAY_MS;
	if (period === 'all') return 0;
	if (period === 'today') return utcDayStart(now);
	const days = WINDOW_DAYS[period];
	return utcDayStart(now) - (days - 1) * DAY_MS;
}

/** Bounds the selected summary period and the window before it, so one number means the same
 *  thing at every window.
 *  Today compares against the same span of yesterday rather than a full day, so a part-day today
 *  is not read against a whole day behind it. */
export function summaryWindow(period: WindowPeriod, now: number): SummaryWindow {
	if (period === 'all') return { start: 0, until: 0, prevStart: 0, prevUntil: 0 };
	if (period === '24h') {
		return { start: now - DAY_MS, until: 0, prevStart: now - 2 * DAY_MS, prevUntil: now - DAY_MS };
	}
	if (period === 'today') {
		const start = utcDayStart(now);
		return { start, until: 0, prevStart: start - DAY_MS, prevUntil: now - DAY_MS };
	}
	const days = WINDOW_DAYS[period];
	const start = utcDayStart(now) - (days - 1) * DAY_MS;
	return { start, until: 0, prevStart: start - days * DAY_MS, prevUntil: start };
}

/** The dashboard's recent-request tail remembers whether it is live in this browser.
 *  A missing value is live, which is the same default the logs page opens on. */
export const DASHBOARD_LIVE_KEY = 'relo.dashboard.recentLive';

/** The slice of localStorage the dashboard's live preference uses. */
export interface LiveStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
}

/** Reports whether the dashboard's tail was left live. A browser that refuses storage reads
 *  live. */
export function readDashboardLive(storage: LiveStorage | undefined): boolean {
	if (!storage) return true;
	try {
		const raw = storage.getItem(DASHBOARD_LIVE_KEY);
		if (raw === null) return true;
		return raw === 'on';
	} catch {
		return true;
	}
}

/** Remembers the preference. A refused write is not worth reporting. */
export function writeDashboardLive(storage: LiveStorage | undefined, live: boolean): void {
	if (!storage) return;
	try {
		storage.setItem(DASHBOARD_LIVE_KEY, live ? 'on' : 'off');
	} catch {
		// A refused write still lets this visit keep the chosen live state.
	}
}
