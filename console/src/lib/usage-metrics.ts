// The metric catalog of the usage overview: which cards an operator may
// show, what each one is counted in, and how it is rendered from a usage
// read. The daemon validates the same identifiers in config.toml, so the two
// lists have to stay in step.

import { formatCost, formatDuration, formatTokens } from './format';
import type { UsageTotals } from './usage-filters';

/** One card: a compact value, the exact figure behind it, the unit it is counted in, and the
 *  line under it. */
export interface MetricValue {
	id: string;
	value: string;
	// exact is the precise figure the compact value rounds, which the card
	// carries as its accessible label and its tooltip.
	exact: string;
	unitKey: string;
	noteKey: string;
	noteValues: Record<string, string>;
	// danger marks a card about something going wrong, so the number itself
	// carries the emphasis rather than the accent color.
	danger: boolean;
}

/** What one row of a ranked list states: the words it shows, and the figure the bar behind them
 *  is sized by. A local engine's spend reads as a dash while its weight stays zero, so the text
 *  and the bar are not always the same number. */
export interface SpendReadout {
	text: string;
	weight: number;
}

const count = (value: number): string => Math.max(0, Math.round(value)).toLocaleString();
function rate(part: number, whole: number): string {
	if (whole <= 0) return '—';
	return ((part / whole) * 100).toFixed(1) + '%';
}

function average(total: number, parts: number, render: (value: number) => string): string {
	if (parts <= 0) return '—';
	return render(total / parts);
}

/** Renders one metric from the totals of a read.
 *  An unknown identifier renders nothing rather than a broken card. */
export function metricValue(id: string, totals: UsageTotals): MetricValue | null {
	const successes = Math.max(0, totals.requests - totals.errors);
	const priced = Math.max(0, totals.requests - totals.unpricedRequests);
	switch (id) {
		case 'requests':
			return card(id, count(totals.requests), 'unitRequests');
		case 'successes':
			return card(id, count(successes), 'unitRequests');
		case 'errors':
			return {
				...card(id, count(totals.errors), 'unitErrors'),
				danger: totals.errors > 0,
				noteKey: 'noteErrorRate',
				noteValues: { value: rate(totals.errors, totals.requests) }
			};
		case 'success_rate':
			return {
				...card(id, rate(successes, totals.requests), ''),
				exact: rate(successes, totals.requests)
			};
		case 'error_rate':
			return {
				...card(id, rate(totals.errors, totals.requests), ''),
				exact: rate(totals.errors, totals.requests),
				danger: totals.errors > 0
			};
		case 'attempts':
			return {
				...card(id, count(totals.attempts), 'unitAttempts'),
				noteKey: 'noteAttemptsPerRequest',
				noteValues: {
					value: average(totals.attempts, totals.requests, (value) => value.toFixed(2))
				}
			};
		case 'retried':
			return {
				...card(id, count(totals.retriedRequests), 'unitRequests'),
				noteKey: 'noteRetriedShare',
				noteValues: { value: rate(totals.retriedRequests, totals.requests) }
			};
		case 'tokens_input':
			return token(id, totals.inputTokens);
		case 'tokens_output':
			return token(id, totals.outputTokens);
		case 'tokens_total':
			return {
				...token(id, totals.totalTokens),
				noteKey: 'noteTokenMix',
				noteValues: {
					input: formatTokens(totals.inputTokens),
					output: formatTokens(totals.outputTokens)
				}
			};
		case 'tokens_per_request':
			return {
				...card(id, average(totals.totalTokens, totals.requests, formatTokens), 'unitPerRequest'),
				exact: average(totals.totalTokens, totals.requests, count)
			};
		case 'cache_read':
			return token(id, totals.cacheReadTokens);
		case 'cache_write':
			return token(id, totals.cacheWriteTokens);
		case 'spend':
			return {
				...card(id, formatCost(totals.spendMicros), 'unitEstimated'),
				noteKey: 'noteUnpriced',
				noteValues: { value: count(totals.unpricedRequests) }
			};
		case 'plan_usage':
			return {
				...card(id, formatCost(totals.planUsageMicros), 'unitEstimated'),
				noteKey: 'notePlanUsage'
			};
		case 'spend_per_request':
			return {
				...card(
					id,
					priced > 0 ? formatCost(Math.round(totals.spendMicros / priced)) : '—',
					'unitPricedRequest'
				),
				exact: priced > 0 ? formatCost(Math.round(totals.spendMicros / priced)) : '—'
			};
		case 'priced_requests':
			return card(id, count(priced), 'unitRequests');
		case 'unpriced_requests':
			return card(id, count(totals.unpricedRequests), 'unitRequests');
		case 'duration_avg':
			return {
				...card(
					id,
					average(totals.durationMs, totals.requests, (value) => formatDuration(Math.round(value))),
					'unitPerRequest'
				),
				noteKey: 'noteSlowest',
				noteValues: { value: totals.durationMaxMs > 0 ? formatDuration(totals.durationMaxMs) : '—' }
			};
		case 'duration_max':
			return card(id, totals.durationMaxMs > 0 ? formatDuration(totals.durationMaxMs) : '—', '');
		default:
			return null;
	}
}

/** Renders every chosen metric in the order chosen, skipping an identifier this build does not
 *  know. */
export function metricValues(ids: readonly string[], totals: UsageTotals): MetricValue[] {
	const values: MetricValue[] = [];
	for (const id of ids) {
		const value = metricValue(id, totals);
		if (value) values.push(value);
	}
	return values;
}

/** Resolves a stored choice against what the daemon offers, which keeps a card the daemon
 *  dropped out of the page and preserves the operator's order for everything else. */
export function chosenMetrics(chosen: readonly string[], available: readonly string[]): string[] {
	const known = new Set(available);
	const seen = new Set<string>();
	const resolved: string[] = [];
	for (const id of chosen) {
		if (!known.has(id) || seen.has(id)) continue;
		seen.add(id);
		resolved.push(id);
	}
	return resolved;
}

/** Names the catalogue entry of one metric. */
export function metricLabelKey(id: string): string {
	return 'ui.pages.usagePage.metric.' + id;
}

function card(id: string, value: string, unitKey: string): MetricValue {
	return { id, value, exact: value, unitKey, noteKey: '', noteValues: {}, danger: false };
}

function token(id: string, tokens: number): MetricValue {
	return { ...card(id, formatTokens(tokens), 'unitTokens'), exact: count(tokens) };
}
