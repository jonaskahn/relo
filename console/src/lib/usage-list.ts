// The grouped usage list: how rows are named, searched, ordered, summarised,
// and folded into shares. Kept pure so the page and its tests agree on one
// reading of the same rollup.

import { formatCost, formatDuration, formatTokens } from './format';
import type { SpendReadout } from './usage-metrics';
import type { UsageRollupRow } from './types';

/** Names what a grouped usage row sorts by. */
export type UsageSortKey = 'name' | 'requests' | 'errors' | 'tokens' | 'latency' | 'spend';

/** One sort choice: the field and whether the largest value comes first. */
export interface UsageSort {
	key: UsageSortKey;
	descending: boolean;
}

/** One group as the list and its insights read it. */
export interface UsageListRow {
	key: string;
	row: UsageRollupRow;
	label: string;
	id: string;
	spend: SpendReadout;
	// searchText carries the connection a grouped account belongs to, so a
	// search for either name finds the same account.
	searchText?: string;
}

/** One finding above the list. The page renders the words and units; the
 *  module only decides which group holds which figure. */
export type UsageInsight =
	| { kind: 'highest'; entry: UsageListRow; share: number }
	| { kind: 'error_rate'; entry: UsageListRow; rate: number }
	| { kind: 'requests'; entry: UsageListRow }
	| { kind: 'latency'; entry: UsageListRow; averageMs: number };

/** One segment of the share strip: a group or the combined tail. */
export interface UsageShareSegment {
	key: string;
	label: string;
	value: number;
	share: number;
	// count is present only on the combined tail.
	count?: number;
}

/** Names the tail segment so the page can give it a localized label. */
export const SHARE_OTHER_KEY = '__other__';

const ERROR_RATE_SAMPLE = 50;
const SHARE_LIMIT = 5;

/** Reads one row's total tokens, cache included. */
export function totalTokensOf(row: UsageRollupRow): number {
	return (
		Math.max(0, Number(row.InputTokens)) +
		Math.max(0, Number(row.OutputTokens)) +
		Math.max(0, Number(row.CacheReadTokens)) +
		Math.max(0, Number(row.CacheWriteTokens))
	);
}

/** Reads a row's average duration, or null when it served no requests. */
export function averageDurationOf(row: UsageRollupRow): number | null {
	const requests = Number(row.Requests);
	return requests > 0 ? Math.max(0, Number(row.DurationMs)) / requests : null;
}

/** Reads a row's error rate, or null when it served no requests. */
export function errorRateOf(row: UsageRollupRow): number | null {
	const requests = Number(row.Requests);
	return requests > 0 ? Math.max(0, Number(row.Errors)) / requests : null;
}

/** Keeps the rows whose label, identifier, key, or related search text contains the query. */
export function searchUsageRows(rows: readonly UsageListRow[], query: string): UsageListRow[] {
	const needle = query.trim().toLocaleLowerCase();
	if (needle === '') return [...rows];
	return rows.filter((entry) =>
		[entry.label, entry.id, entry.key, entry.searchText ?? ''].some((text) =>
			text.toLocaleLowerCase().includes(needle)
		)
	);
}

function sortValue(entry: UsageListRow, key: UsageSortKey): string | number | null {
	switch (key) {
		case 'name':
			return entry.label;
		case 'requests':
			return Number(entry.row.Requests);
		case 'errors':
			return errorRateOf(entry.row);
		case 'tokens':
			return totalTokensOf(entry.row);
		case 'latency':
			return averageDurationOf(entry.row);
		case 'spend':
			return entry.spend.weight;
	}
}

/** Orders rows by one field. A missing value sorts last in both directions; ties fall back to
 *  the stable group key so a refresh never reshuffles equal rows. */
export function sortUsageRows(rows: readonly UsageListRow[], sort: UsageSort): UsageListRow[] {
	return [...rows].sort((a, b) => {
		const left = sortValue(a, sort.key);
		const right = sortValue(b, sort.key);
		if (left === null || right === null) {
			if (left === right) return a.key.localeCompare(b.key);
			return left === null ? 1 : -1;
		}
		const order =
			typeof left === 'string' || typeof right === 'string'
				? String(left).localeCompare(String(right))
				: left - right;
		return order !== 0 ? (sort.descending ? -order : order) : a.key.localeCompare(b.key);
	});
}

function highestUsageRow(
	rows: readonly UsageListRow[],
	metric: 'spend' | 'requests'
): UsageListRow | null {
	return (
		[...rows]
			.filter((entry) => entry.key !== '')
			.sort(
				(a, b) =>
					(metric === 'spend'
						? b.spend.weight - a.spend.weight
						: Number(b.row.Requests) - Number(a.row.Requests)) || a.key.localeCompare(b.key)
			)[0] ?? null
	);
}

/** Finds the most valuable group, the highest error rate and slowest average among groups with
 *  enough traffic to make a rate meaningful, and falls back to the busiest group when none has
 *  enough traffic. Empty and unnamed groups never headline a finding. */
export function usageInsights(
	rows: readonly UsageListRow[],
	valueMetric: 'spend' | 'requests'
): UsageInsight[] {
	const insights: UsageInsight[] = [];
	const highest = highestUsageRow(rows, valueMetric);
	if (highest) {
		const total = rows.reduce(
			(sum, entry) => sum + (valueMetric === 'spend' ? entry.spend.weight : entry.row.Requests),
			0
		);
		const value = valueMetric === 'spend' ? highest.spend.weight : highest.row.Requests;
		if (total > 0 && value > 0) {
			insights.push({ kind: 'highest', entry: highest, share: value / total });
		}
	}

	const sampled = rows.filter((entry) => Number(entry.row.Requests) >= ERROR_RATE_SAMPLE);
	const errorRow = [...sampled].sort(
		(a, b) => (errorRateOf(b.row) ?? 0) - (errorRateOf(a.row) ?? 0) || a.key.localeCompare(b.key)
	)[0];
	if (errorRow) {
		insights.push({ kind: 'error_rate', entry: errorRow, rate: errorRateOf(errorRow.row) ?? 0 });
	} else {
		const busiest = highestUsageRow(rows, 'requests');
		if (busiest) insights.push({ kind: 'requests', entry: busiest });
	}

	const slowest = [...sampled]
		.filter((entry) => averageDurationOf(entry.row) !== null)
		.sort(
			(a, b) =>
				(averageDurationOf(b.row) ?? 0) - (averageDurationOf(a.row) ?? 0) ||
				a.key.localeCompare(b.key)
		)[0];
	if (slowest) {
		insights.push({
			kind: 'latency',
			entry: slowest,
			averageMs: averageDurationOf(slowest.row) ?? 0
		});
	}
	return insights;
}

/** Folds the ranked rows into a share strip: the largest five, then one combined tail. Shares
 *  are taken against every positive group, including the tail. */
export function shareSegments(
	rows: readonly UsageListRow[],
	metric: 'requests' | 'tokens' | 'spend' | 'errors'
): UsageShareSegment[] {
	const values = rows
		.map((entry) => ({
			key: entry.key,
			label: entry.label,
			value:
				metric === 'spend'
					? entry.spend.weight
					: metric === 'tokens'
						? totalTokensOf(entry.row)
						: metric === 'errors'
							? Math.max(0, Number(entry.row.Errors))
							: Math.max(0, Number(entry.row.Requests))
		}))
		.filter((entry) => entry.value > 0)
		.sort((a, b) => b.value - a.value || a.key.localeCompare(b.key));
	const total = values.reduce((sum, entry) => sum + entry.value, 0);
	if (total <= 0) return [];
	const head = values.slice(0, SHARE_LIMIT);
	const tail = values.slice(SHARE_LIMIT);
	const segments: UsageShareSegment[] = head.map((entry) => ({
		...entry,
		share: (entry.value / total) * 100
	}));
	if (tail.length > 0) {
		const value = tail.reduce((sum, entry) => sum + entry.value, 0);
		segments.push({
			key: SHARE_OTHER_KEY,
			label: '',
			value,
			share: (value / total) * 100,
			count: tail.length
		});
	}
	return segments;
}

/** States a group's spend the way its row reads it: a local engine has no price, a plan-covered
 *  connection has plan usage, and every other group has API spend. */
export function spendOfRow(
	row: UsageRollupRow,
	providerRow: boolean,
	kind: 'paid' | 'plan' | 'local' | undefined,
	planMicros: number
): SpendReadout {
	if (providerRow && kind === 'local') return { text: '—', weight: 0 };
	const weight = providerRow && kind === 'plan' ? planMicros : Number(row.CostMicros);
	return { text: formatCost(weight), weight };
}

/** Renders the compact token figure and its exact value for a row. */
export function tokenTexts(row: UsageRollupRow): { compact: string; exact: string } {
	const total = totalTokensOf(row);
	return { compact: formatTokens(total), exact: total.toLocaleString() };
}

/** Renders a row's average duration, with a dash when it served no requests. */
export function averageText(row: UsageRollupRow): string {
	const average = averageDurationOf(row);
	return average === null ? '—' : formatDuration(Math.round(average));
}

/** Renders a row's slowest request, with a dash when none was recorded. */
export function slowestText(row: UsageRollupRow): string {
	const slowest = Math.max(0, Number(row.DurationMaxMs));
	return slowest > 0 ? formatDuration(slowest) : '—';
}

/** Renders a row's error rate, with a dash when it served no requests. */
export function errorRateText(row: UsageRollupRow): string {
	const rate = errorRateOf(row);
	return rate === null ? '—' : (rate * 100).toFixed(1) + '%';
}

/** Names the default ordering of one grouped tab. */
export function defaultSort(group: 'day' | 'provider' | 'model' | 'account' | 'client'): UsageSort {
	return group === 'day' ? { key: 'name', descending: true } : { key: 'spend', descending: true };
}

/** Reads a sort choice from the address, falling back to the tab's default. */
export function parseSort(value: string | null, fallback: UsageSort): UsageSort {
	if (!value) return fallback;
	const [key, direction] = value.split(':');
	if (
		key !== 'name' &&
		key !== 'requests' &&
		key !== 'errors' &&
		key !== 'tokens' &&
		key !== 'latency' &&
		key !== 'spend'
	) {
		return fallback;
	}
	if (direction !== 'asc' && direction !== 'desc') return fallback;
	return { key, descending: direction === 'desc' };
}

/** Renders a sort choice as the address parameter it round-trips through. */
export function sortParam(sort: UsageSort): string {
	return sort.key + ':' + (sort.descending ? 'desc' : 'asc');
}
