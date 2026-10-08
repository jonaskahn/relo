// The usage page's filter state and the query it renders, kept pure so the
// page, the summary read, and the tests all agree on one mapping.

import { spendKindOf } from './dashboard';
import type { Provider, UsageRollupRow } from './types';

/** Names what one usage table rows by. */
export type UsageGroupBy = 'day' | 'provider' | 'model' | 'account' | 'client';

/** Names the tab a group lives on. The overview reads totals and a trend, and every other tab
 *  reads one grouping in full. */
export type UsageTab = 'overview' | UsageGroupBy;

/** Lists every tab in the order the page shows them. */
export const USAGE_TABS: readonly UsageTab[] = [
	'overview',
	'day',
	'provider',
	'model',
	'account',
	'client'
];

/** Reports whether a choice the tab strip handed back is one the page reads, so an unexpected
 *  value leaves the report on the tab it was already showing. */
export function isUsageTab(value: string): value is UsageTab {
	return USAGE_TABS.some((tab) => tab === value);
}

/** Names the grouping a tab reads, and whether the tab groups at all. */
export function tabGroup(tab: UsageTab): UsageGroupBy | null {
	return tab === 'overview' ? null : tab;
}

/** One window of the usage report. */
export type UsageRange = '24h' | 'today' | '7' | '30' | '60' | '90' | 'all';

/** Reports whether a range the control handed back is one the report can be read over. */
export function isUsageRange(value: string): value is UsageRange {
	return (
		value === '24h' ||
		value === 'today' ||
		value === '7' ||
		value === '30' ||
		value === '60' ||
		value === '90' ||
		value === 'all'
	);
}

/** The status filter the page offers, mapped to the codes and classes the API accepts. An empty
 *  value matches every row. */
export type UsageStatusFilter = '' | 'ok' | 'error';

/** The whole usage query as the page holds it. */
export interface UsageFilters {
	groupBy: UsageGroupBy;
	range: UsageRange;
	provider: string;
	account: string;
	model: string;
	client: string;
	origin: '' | 'external' | 'internal';
	status: UsageStatusFilter;
}

/** Reports whether a choice a select handed back is one the report separates traffic by; an
 *  empty value is the whole report. */
export function isUsageOrigin(value: string): value is UsageFilters['origin'] {
	return value === '' || value === 'external' || value === 'internal';
}

/** The report a first visit opens. */
export const DEFAULT_USAGE_FILTERS: UsageFilters = {
	groupBy: 'day',
	// All time is what an operator opens on: the archive answers the older
	// days, so the default costs no more than a short range.
	range: 'all',
	provider: '',
	account: '',
	model: '',
	client: '',
	origin: '',
	status: ''
};

/** The grouping choices, in the order the control shows them. */
export const USAGE_GROUPS: readonly UsageGroupBy[] = [
	'day',
	'provider',
	'model',
	'account',
	'client'
];

const RANGE_DAYS: Record<'7' | '30' | '60' | '90', number> = {
	'7': 7,
	'30': 30,
	'60': 60,
	'90': 90
};

const DAY_MS = 86_400_000;

/** Names the codes a status choice matches. OK is the answers a client accepted and Error is the
 *  two classes a refusal arrives in, which is what the API reads as codes (429) and classes
 *  (4xx). */
export function statusParam(status: UsageStatusFilter): string {
	switch (status) {
		case 'ok':
			return '2xx';
		case 'error':
			return '4xx,5xx';
		default:
			return '';
	}
}

/** Floors one instant to the UTC day the ledger aggregates by.
 *  Closed days are answered at whole-day precision, so a window that starts mid-day reads a day
 *  the archive rounds; every usage window starts on one of these boundaries. */
export function startOfUTCDay(ms: number): number {
	return Math.floor(ms / DAY_MS) * DAY_MS;
}

/** Returns the oldest timestamp a range keeps, or 0 for all time.
 *  A bounded range covers whole UTC days, today included, which is the grain the archive answers
 *  at; the rolling day starts 24 hours back instead. */
export function usageSinceMs(range: UsageRange, nowMs: number): number {
	if (range === 'all') {
		return 0;
	}
	if (range === '24h') {
		return nowMs - DAY_MS;
	}
	if (range === 'today') {
		return startOfUTCDay(nowMs);
	}
	return startOfUTCDay(nowMs) - (RANGE_DAYS[range] - 1) * DAY_MS;
}

/** Renders the filter as the query string every usage read shares.
 *  The summary read asks for the same filter without a group, so a page's headline totals match
 *  its groups. */
export function usageQuery(filters: UsageFilters, nowMs: number, includeGroup = true): string {
	const params = new URLSearchParams();
	if (includeGroup) {
		params.set('group_by', filters.groupBy);
	}
	const since = usageSinceMs(filters.range, nowMs);
	if (since > 0) {
		params.set('since_ms', String(since));
	}
	if (filters.provider) params.set('provider', filters.provider);
	if (filters.account) params.set('account', filters.account);
	if (filters.model) params.set('model', filters.model);
	if (filters.client) params.set('client', filters.client);
	if (filters.origin) params.set('origin', filters.origin);
	const status = statusParam(filters.status);
	if (status) params.set('status', status);
	return params.toString();
}

/** One read of several connections. The usage API takes each as its own provider parameter. */
export function usageProvidersQuery(
	filters: UsageFilters,
	nowMs: number,
	providerIds: readonly string[]
): string {
	const params = new URLSearchParams(usageQuery({ ...filters, provider: '' }, nowMs));
	for (const id of providerIds) {
		if (id) params.append('provider', id);
	}
	return params.toString();
}

/** Names what the cost of a filtered read answers:
 *  API spend for a pay-as-you-go connection, plan usage for a plan connection, neither for a
 *  local engine, and both when no connection is named. */
export type SpendScope = 'api' | 'plan' | 'local' | 'all';

/** Reports the scope one provider filter names.
 *  A filter that names no stored connection reads like no filter at all. */
export function spendScopeOf(filters: UsageFilters, providers: readonly Provider[]): SpendScope {
	if (filters.provider === '') return 'all';
	const provider = providers.find((entry) => entry.id === filters.provider);
	if (!provider) return 'all';
	const kind = spendKindOf(provider);
	return kind === 'paid' ? 'api' : kind;
}

/** Names the connections a plan covers. */
export function planProviderIds(providers: readonly Provider[]): string[] {
	return providers
		.filter((provider) => spendKindOf(provider) === 'plan')
		.map((provider) => provider.id);
}

/** Names every connection that is not pay-as-you-go. */
export function nonPaidProviderIds(providers: readonly Provider[]): string[] {
	return providers
		.filter((provider) => spendKindOf(provider) !== 'paid')
		.map((provider) => provider.id);
}

/** One read's cost divided by what pays for it. */
export interface SpendSplit {
	apiSpendMicros: number;
	planUsageMicros: number;
	// nonPaidMicros is what API spend leaves out: every connection that is
	// not pay-as-you-go, local engines included.
	nonPaidMicros: number;
}

/** Divides one exact summary between API spend and plan usage from the provider rollup of the
 *  read.
 *  A pay-as-you-go connection's row is API spend, a row a plan covers is plan usage, and a local
 *  engine's row is neither, so the API figure is what remains after the non-paid rows. */
export function splitSpend(
	summaryCostMicros: number,
	providerRows: readonly UsageRollupRow[],
	providers: readonly Provider[]
): SpendSplit {
	const kinds = new Map(providers.map((provider) => [provider.id, spendKindOf(provider)]));
	let nonPaidMicros = 0;
	let planMicros = 0;
	for (const row of providerRows) {
		const costMicros = Number(row.CostMicros);
		if (kinds.get(row.Key) === 'plan') planMicros += costMicros;
		if (kinds.get(row.Key) !== 'paid') nonPaidMicros += costMicros;
	}
	return {
		apiSpendMicros: Math.max(0, summaryCostMicros - nonPaidMicros),
		planUsageMicros: planMicros,
		nonPaidMicros
	};
}

/** Removes one plan total from a summary. The other counters stay, and the cost does not go
 *  below zero. */
export function subtractSummarySpend(
	summary: UsageSummary | null,
	planCost: number
): UsageSummary | null {
	if (!summary) return null;
	return { ...summary, cost_micros: Math.max(0, summary.cost_micros - planCost) };
}

/** The summary of a read that is only plan usage. */
export function clearSummarySpend(summary: UsageSummary | null): UsageSummary | null {
	if (!summary) return null;
	return { ...summary, cost_micros: 0 };
}

/** Removes the plan cost of each group key. */
export function subtractRollupSpend(
	rows: readonly UsageRollupRow[],
	plan: readonly UsageRollupRow[]
): UsageRollupRow[] {
	if (plan.length === 0) return [...rows];
	const cost = new Map(plan.map((row) => [row.Key, Number(row.CostMicros)]));
	return rows.map((row) => {
		const drop = cost.get(row.Key) ?? 0;
		if (drop <= 0) return row;
		return { ...row, CostMicros: Math.max(0, Number(row.CostMicros) - drop) };
	});
}

/** Zeroes every row's cost. */
export function clearRollupSpend(rows: UsageRollupRow[]): UsageRollupRow[] {
	return rows.map((row) => (row.CostMicros === 0 ? row : { ...row, CostMicros: 0 }));
}

/** Maps each plan connection to the plan usage its own provider rollup row reports. */
export function providerPlanMicros(
	providerRows: readonly UsageRollupRow[],
	providers: readonly Provider[]
): Record<string, number> {
	const kinds = new Map(providers.map((provider) => [provider.id, spendKindOf(provider)]));
	const plan: Record<string, number> = {};
	for (const row of providerRows) {
		if (kinds.get(row.Key) === 'plan') plan[row.Key] = Number(row.CostMicros);
	}
	return plan;
}

/** The headline the page shows. TotalTokens is the exact sum the summary read reports, which is
 *  not the sum of a limited group list. */
export interface UsageTotals {
	requests: number;
	errors: number;
	inputTokens: number;
	outputTokens: number;
	totalTokens: number;
	spendMicros: number;
	// planUsageMicros is what a plan covered, which is catalog value rather
	// than an invoice. The summary carries no such split, so the page reads it
	// from the plan rollup and hands it here.
	planUsageMicros: number;
	unpricedRequests: number;
	cacheReadTokens: number;
	cacheWriteTokens: number;
	durationMs: number;
	// attempts is how many upstream sends the requests made, and
	// retriedRequests how many of them needed more than one.
	attempts: number;
	retriedRequests: number;
	// durationMaxMs is the slowest single request, which the average hides.
	durationMaxMs: number;
}

/** A report with nothing in it. */
export const EMPTY_TOTALS: UsageTotals = {
	requests: 0,
	errors: 0,
	inputTokens: 0,
	outputTokens: 0,
	totalTokens: 0,
	spendMicros: 0,
	planUsageMicros: 0,
	unpricedRequests: 0,
	cacheReadTokens: 0,
	cacheWriteTokens: 0,
	durationMs: 0,
	attempts: 0,
	retriedRequests: 0,
	durationMaxMs: 0
};

/** Mirrors the summary endpoint's body. */
export interface UsageSummary {
	requests: number;
	errors: number;
	input_tokens: number;
	output_tokens: number;
	cache_read_tokens: number;
	cache_write_tokens: number;
	cost_micros: number;
	unpriced_requests: number;
	duration_ms: number;
	// The counters a read reports when the daemon knows them. A stored body
	// an older daemon wrote reads as zero.
	duration_max_ms?: number;
	attempts?: number;
	retried_requests?: number;
	// archive reports that the totals include whole days read from their
	// aggregate rather than from the request log.
	archive?: boolean;
	archive_from_ms?: number;
}

/** Folds the summary body into the page's headline.
 *  The summary knows no plan, so planUsageMicros comes from the plan rollup the page also reads. */
export function totalsOf(summary: UsageSummary | null, planUsageMicros = 0): UsageTotals {
	if (!summary) {
		return EMPTY_TOTALS;
	}
	return {
		requests: summary.requests,
		errors: summary.errors,
		inputTokens: summary.input_tokens,
		outputTokens: summary.output_tokens,
		// The total is every token the traffic carried, cache included,
		// which is the sum the dashboard and the desktop window report.
		totalTokens:
			summary.input_tokens +
			summary.output_tokens +
			summary.cache_read_tokens +
			summary.cache_write_tokens,
		spendMicros: summary.cost_micros,
		planUsageMicros,
		unpricedRequests: summary.unpriced_requests,
		cacheReadTokens: summary.cache_read_tokens,
		cacheWriteTokens: summary.cache_write_tokens,
		durationMs: summary.duration_ms,
		attempts: summary.attempts ?? 0,
		retriedRequests: summary.retried_requests ?? 0,
		durationMaxMs: summary.duration_max_ms ?? 0
	};
}

/** Reports whether anything narrows the read beyond the group and range, so the page can offer a
 *  clear action only when there is one. */
export function hasFilters(filters: UsageFilters): boolean {
	return activeFilterCount(filters) > 0;
}

/** Counts the filters that narrow a read, which is what a collapsed filter panel reports on its
 *  toggle. */
export function activeFilterCount(filters: UsageFilters): number {
	return [
		filters.provider,
		filters.account,
		filters.model,
		filters.client,
		filters.origin,
		filters.status
	].filter((value) => value !== '').length;
}
