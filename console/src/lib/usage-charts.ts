// The chart catalogue of the usage page: which series an operator may read,
// what each one counts, and how the bars are ordered. Kept pure so the page,
// the trend, and the tests all agree on one mapping.

import { formatCost, formatDuration, formatTokens } from './format';
import type { UsageRollupRow } from './types';

/** Names the quantity one chart draws. Every series a usage row can answer lives here, so a
 *  second chart never needs a second read. */
export type ChartMetric = 'requests' | 'tokens' | 'spend' | 'errors' | 'latency';

/** Lists the series in the order the switch shows them. */
export const CHART_METRICS: readonly ChartMetric[] = [
	'requests',
	'tokens',
	'spend',
	'errors',
	'latency'
];

/** Reads one series out of a usage row. Latency is the average a request of that group took,
 *  which a quiet group has none of. */
export function metricValueOf(row: UsageRollupRow, metric: ChartMetric): number {
	switch (metric) {
		case 'requests':
			return Number(row.Requests);
		case 'tokens':
			// Every token the traffic carried, cache included — the same
			// total the metric card and the dashboard report.
			return (
				Number(row.InputTokens) +
				Number(row.OutputTokens) +
				Number(row.CacheReadTokens) +
				Number(row.CacheWriteTokens)
			);
		case 'spend':
			return Number(row.CostMicros);
		case 'errors':
			return Number(row.Errors);
		case 'latency': {
			const requests = Number(row.Requests);
			return requests > 0 ? Number(row.DurationMs) / requests : 0;
		}
		default:
			return 0;
	}
}

/** Renders one value in the unit of its series, so a chart reads the same way the metric cards
 *  beside it do. */
export function formatChartValue(value: number, metric: ChartMetric): string {
	switch (metric) {
		case 'tokens':
			return formatTokens(Math.round(value));
		case 'spend':
			return formatCost(Math.round(value));
		case 'latency':
			return value > 0 ? formatDuration(Math.round(value)) : '—';
		default:
			return Math.round(value).toLocaleString();
	}
}

/** Sums one series over a trend. Latency has no sum to state, so it is the average over every
 *  request the range served instead. */
export function trendTotal(rows: readonly UsageRollupRow[], metric: ChartMetric): number {
	if (metric === 'latency') {
		const requests = rows.reduce((sum, row) => sum + Number(row.Requests), 0);
		if (requests <= 0) return 0;
		return rows.reduce((sum, row) => sum + Number(row.DurationMs), 0) / requests;
	}
	return rows.reduce((sum, row) => sum + metricValueOf(row, metric), 0);
}

/** One row of a comparison chart: the group it names, the value it holds, and the share of the
 *  longest bar it is drawn at. */
export interface ChartBar {
	key: string;
	value: number;
	share: number;
}

/** Orders the groups of one rollup by the chosen series and keeps the busiest few.
 *  A group with nothing to show is left out, and the share is floored so a small bar is still
 *  visible. */
export function barRows(
	rows: readonly UsageRollupRow[],
	metric: ChartMetric,
	limit = 10
): ChartBar[] {
	const ordered = rows
		.map((row) => ({ key: row.Key, value: metricValueOf(row, metric) }))
		.filter((entry) => entry.key !== '' && entry.value > 0)
		.sort((a, b) => b.value - a.value)
		.slice(0, Math.max(0, limit));
	const longest = ordered[0]?.value ?? 0;
	return ordered.map((entry) => ({
		...entry,
		share: longest > 0 ? Math.min(100, Math.max(2, (entry.value / longest) * 100)) : 0
	}));
}

/** How many day columns a chart draws before days share a column.
 *  Past it the 640-wide baseline gives a column no width, so wide windows read at multi-day
 *  grain instead of an empty chart. */
export const MAX_DAY_COLUMNS = 92;

/** Folds dated rows into at most maxColumns columns, oldest first.
 *  Sums add and the slowest request keeps the slowest, so every total a bucket states equals the
 *  days it holds; a bucket keeps its first day's key, which is the label the axis reads. A short
 *  series passes through untouched. */
export function bucketDays(
	rows: readonly UsageRollupRow[],
	maxColumns = MAX_DAY_COLUMNS
): UsageRollupRow[] {
	const ordered = [...rows].sort((a, b) => (a.Key < b.Key ? -1 : a.Key > b.Key ? 1 : 0));
	if (ordered.length <= maxColumns || maxColumns <= 0) return ordered;
	const size = Math.ceil(ordered.length / maxColumns);
	const buckets: UsageRollupRow[] = [];
	for (let at = 0; at < ordered.length; at += size) {
		buckets.push(mergeBucket(ordered.slice(at, at + size)));
	}
	return buckets;
}

function mergeBucket(part: UsageRollupRow[]): UsageRollupRow {
	const first = part[0];
	const sum = (pick: (row: UsageRollupRow) => number): number =>
		part.reduce((total, row) => total + Math.max(0, Number(pick(row)) || 0), 0);
	return {
		Key: first.Key,
		Requests: sum((row) => row.Requests),
		Errors: sum((row) => row.Errors),
		InputTokens: sum((row) => row.InputTokens),
		OutputTokens: sum((row) => row.OutputTokens),
		CacheReadTokens: sum((row) => row.CacheReadTokens),
		CacheWriteTokens: sum((row) => row.CacheWriteTokens),
		CostMicros: sum((row) => row.CostMicros),
		UnpricedRequests: sum((row) => row.UnpricedRequests),
		DurationMs: sum((row) => row.DurationMs),
		Attempts: sum((row) => row.Attempts),
		RetriedRequests: sum((row) => row.RetriedRequests),
		DurationMaxMs: part.reduce(
			(slowest, row) => Math.max(slowest, Number(row.DurationMaxMs) || 0),
			0
		)
	};
}

/** One day of the token mix: every token the traffic carried, split the way the ledger stores
 *  it. */
export interface TokenMixPoint {
	key: string;
	input: number;
	output: number;
	cacheRead: number;
	cacheWrite: number;
	total: number;
}

/** Stacks one rollup per day into its four token kinds, oldest day first.
 *  A quiet day keeps its zero total rather than disappearing, so the columns line up with the
 *  trend beside them. */
export function tokenMixSeries(rows: readonly UsageRollupRow[]): TokenMixPoint[] {
	return [...rows]
		.sort((a, b) => (a.Key < b.Key ? -1 : a.Key > b.Key ? 1 : 0))
		.map((row) => {
			const input = Math.max(0, Number(row.InputTokens));
			const output = Math.max(0, Number(row.OutputTokens));
			const cacheRead = Math.max(0, Number(row.CacheReadTokens));
			const cacheWrite = Math.max(0, Number(row.CacheWriteTokens));
			return {
				key: row.Key,
				input,
				output,
				cacheRead,
				cacheWrite,
				total: input + output + cacheRead + cacheWrite
			};
		});
}

/** One day of answers and refusals: what the traffic carried, what failed, and what needed
 *  another upstream attempt. */
export interface ReliabilityPoint {
	key: string;
	requests: number;
	errors: number;
	retried: number;
	errorRate: number;
}

/** Reads one point per day, oldest first. A quiet day has no rate to claim. */
export function reliabilitySeries(rows: readonly UsageRollupRow[]): ReliabilityPoint[] {
	return [...rows]
		.sort((a, b) => (a.Key < b.Key ? -1 : a.Key > b.Key ? 1 : 0))
		.map((row) => {
			const requests = Math.max(0, Number(row.Requests));
			const errors = Math.max(0, Number(row.Errors));
			const retried = Math.max(0, Number(row.RetriedRequests));
			return {
				key: row.Key,
				requests,
				errors,
				retried,
				errorRate: requests > 0 ? errors / requests : 0
			};
		});
}

/** One day of durations: the average a request took and the slowest one of the day. A quiet day
 *  has neither. */
export interface LatencyPoint {
	key: string;
	requests: number;
	average: number;
	max: number;
}

/** Reads one point per day, oldest first. */
export function latencySeries(rows: readonly UsageRollupRow[]): LatencyPoint[] {
	return [...rows]
		.sort((a, b) => (a.Key < b.Key ? -1 : a.Key > b.Key ? 1 : 0))
		.map((row) => {
			const requests = Math.max(0, Number(row.Requests));
			return {
				key: row.Key,
				requests,
				average: requests > 0 ? Math.max(0, Number(row.DurationMs)) / requests : 0,
				max: requests > 0 ? Math.max(0, Number(row.DurationMaxMs)) : 0
			};
		});
}

/** One entry of a ranked share: what it holds and its share of the entries shown. */
export interface RankedShare {
	key: string;
	value: number;
	share: number;
}

/** Ranks entries by value and keeps the largest few.
 *  Entries with nothing to show are left out, and the shares always add to what the list shows,
 *  never to a total the list does not claim. */
export function rankedShares(
	entries: readonly { key: string; value: number }[],
	limit = 6
): RankedShare[] {
	const ranked = entries
		.filter((entry) => entry.key !== '' && Number(entry.value) > 0)
		.map((entry) => ({ key: entry.key, value: Math.max(0, Number(entry.value)) }))
		.sort((a, b) => b.value - a.value)
		.slice(0, Math.max(0, limit));
	const total = ranked.reduce((sum, entry) => sum + entry.value, 0);
	return ranked.map((entry) => ({ ...entry, share: total > 0 ? (entry.value / total) * 100 : 0 }));
}

/** One slice of a share donut: the entry it names and where its arc starts and how far it
 *  sweeps, in hundredths of a turn. */
export interface DonutSegment extends RankedShare {
	start: number;
	sweep: number;
}

/** Walks the accent ladder from the strongest tint down, so the largest slice reads first.
 *  The sixth slice is the neutral line tone, which keeps a long tail readable without inventing
 *  a palette. */
export const DONUT_FILLS: readonly string[] = [
	'var(--accent)',
	'var(--accent-600)',
	'var(--accent-400)',
	'var(--accent-800)',
	'var(--accent-200)',
	'var(--line-strong)'
];

/** Names the stroke of one slice by its rank. */
export function donutFill(index: number): string {
	return DONUT_FILLS[((index % DONUT_FILLS.length) + DONUT_FILLS.length) % DONUT_FILLS.length];
}

/** Lays ranked shares end to end around one turn.
 *  The sweeps always cover what the list shows, never a total the list does not claim. */
export function donutSegments(shares: readonly RankedShare[]): DonutSegment[] {
	const total = shares.reduce((sum, entry) => sum + entry.share, 0);
	let at = 0;
	return shares.map((entry) => {
		const sweep = total > 0 ? (entry.share / total) * 100 : 0;
		const segment = { ...entry, start: at, sweep };
		at += sweep;
		return segment;
	});
}

/** One day of the activity calendar: the day it names, what the traffic carried, and the
 *  intensity bucket it is drawn at. */
export interface HeatCell {
	key: string;
	value: number;
	level: 0 | 1 | 2 | 3 | 4;
}

/** Draws the calendar in one accent ladder: a quiet day is the sunken panel behind it, never an
 *  invented zero colour. */
export const HEAT_FILLS: readonly string[] = [
	'var(--sunken)',
	'var(--accent-100)',
	'var(--accent-200)',
	'var(--accent-400)',
	'var(--accent)'
];

/** Buckets one day against the busiest one: four steps from a whisper to full accent, with zero
 *  staying quiet. */
export function heatLevel(value: number, max: number): 0 | 1 | 2 | 3 | 4 {
	if (!(value > 0) || !(max > 0)) return 0;
	return Math.min(4, Math.max(1, Math.ceil((value / max) * 4))) as 0 | 1 | 2 | 3 | 4;
}

/** Folds dated values into Monday-first week columns.
 *  Days the series never names read as quiet, so the columns line up with the trend beside them. */
export function heatmapCells(days: readonly { key: string; value: number }[]): HeatCell[][] {
	const dated = days
		.map((entry) => ({ at: dayMs(entry.key), value: Math.max(0, entry.value) }))
		.filter((entry): entry is { at: number; value: number } => entry.at !== null)
		.sort((a, b) => a.at - b.at);
	if (dated.length === 0) return [];
	const byDay = new Map<number, number>();
	for (const entry of dated) byDay.set(entry.at, entry.value);
	const max = Math.max(0, ...dated.map((entry) => entry.value));
	const monday = startOfWeekMonday(dated[0].at);
	const sunday = monday + 6 * 86_400_000;
	let last = sunday;
	while (last < dated[dated.length - 1].at) last += 7 * 86_400_000;
	const weeks: HeatCell[][] = [];
	for (let week = monday; week <= last; week += 7 * 86_400_000) {
		const column: HeatCell[] = [];
		for (let day = 0; day < 7; day += 1) {
			const at = week + day * 86_400_000;
			const value = byDay.get(at) ?? 0;
			column.push({ key: isoDay(at), value, level: heatLevel(value, max) });
		}
		weeks.push(column);
	}
	return weeks;
}

/** The column geometry every bar chart on the usage page draws on: one 640x84
 *  baseline, the value scaled against the series maximum, and a quiet column
 *  keeping a visible 3px floor. */
export function barGeom(
	index: number,
	count: number,
	value: number,
	max: number
): { x: number; y: number; w: number; h: number } {
	const W = 640,
		H = 84,
		GAP = 6;
	const w = count > 0 ? (W - GAP * (count - 1)) / count : W;
	const h = Math.max(3, max > 0 ? (value / max) * H : 0);
	return { x: index * (w + GAP), y: H - h, w, h };
}

/** Names one bar the way a chart axis reads it, and a key that is not a day is
 *  named as it arrived. */
export function shortDay(key: string): string {
	const date = new Date(key + 'T00:00:00Z');
	if (Number.isNaN(date.getTime())) return key;
	return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });
}

function dayMs(key: string): number | null {
	const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(key);
	if (!match) return null;
	const at = Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
	const check = new Date(at);
	if (
		check.getUTCFullYear() !== Number(match[1]) ||
		check.getUTCMonth() !== Number(match[2]) - 1 ||
		check.getUTCDate() !== Number(match[3])
	) {
		return null;
	}
	return at;
}

function startOfWeekMonday(at: number): number {
	const weekday = new Date(at).getUTCDay();
	return at - ((weekday + 6) % 7) * 86_400_000;
}

function isoDay(at: number): string {
	return new Date(at).toISOString().slice(0, 10);
}
