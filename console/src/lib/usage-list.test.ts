import { describe, expect, it } from 'vitest';

import type { UsageRollupRow } from './types';
import {
	SHARE_OTHER_KEY,
	averageDurationOf,
	averageText,
	defaultSort,
	errorRateOf,
	errorRateText,
	parseSort,
	searchUsageRows,
	shareSegments,
	slowestText,
	sortParam,
	sortUsageRows,
	spendOfRow,
	tokenTexts,
	totalTokensOf,
	usageInsights,
	type UsageListRow
} from './usage-list';

function row(overrides: Partial<UsageRollupRow> = {}): UsageRollupRow {
	return {
		Key: 'grok',
		Requests: 100,
		Errors: 5,
		InputTokens: 700,
		OutputTokens: 200,
		CacheReadTokens: 90,
		CacheWriteTokens: 10,
		CostMicros: 2_000_000,
		UnpricedRequests: 0,
		DurationMs: 20_000,
		Attempts: 110,
		RetriedRequests: 4,
		DurationMaxMs: 9_000,
		...overrides
	};
}

function entry(
	key: string,
	overrides: Partial<UsageRollupRow> = {},
	extra: Partial<UsageListRow> = {}
): UsageListRow {
	const data = row({ Key: key, ...overrides });
	return {
		key,
		row: data,
		label: key,
		id: key,
		spend: { text: String(data.CostMicros), weight: data.CostMicros },
		...extra
	};
}

describe('usage row readings', () => {
	it('counts every token kind and keeps quiet rows without a rate or average', () => {
		expect(tokenTexts(row())).toEqual({ compact: '1.0k', exact: '1,000' });
		expect(averageDurationOf(row())).toBe(200);
		expect(errorRateOf(row())).toBeCloseTo(0.05);
		expect(averageText(row())).toBe('200 ms');
		expect(errorRateText(row())).toBe('5.0%');

		const quiet = row({ Requests: 0, Errors: 0, DurationMs: 12_000, DurationMaxMs: 0 });
		expect(averageDurationOf(quiet)).toBeNull();
		expect(errorRateOf(quiet)).toBeNull();
		expect(averageText(quiet)).toBe('—');
		expect(errorRateText(quiet)).toBe('—');
	});

	it('states a local connection without a price and a plan connection as plan usage', () => {
		expect(spendOfRow(row(), true, 'local', 0)).toEqual({ text: '—', weight: 0 });
		expect(spendOfRow(row(), true, 'plan', 4_000_000)).toEqual({
			text: '$4.00',
			weight: 4_000_000
		});
		expect(spendOfRow(row(), true, 'paid', 4_000_000).weight).toBe(2_000_000);
		expect(spendOfRow(row(), false, 'plan', 4_000_000).weight).toBe(2_000_000);
	});
});

describe('usage row readings, unstated', () => {
	it('renders a slowest request only when one was recorded', () => {
		expect(slowestText(row())).toBe('9.0 s');
		expect(slowestText(row({ DurationMaxMs: 0 }))).toBe('—');
		expect(slowestText(row({ DurationMaxMs: -1 }))).toBe('—');
	});

	it('never counts a negative figure against a group', () => {
		expect(
			totalTokensOf(
				row({
					InputTokens: -10,
					OutputTokens: -10,
					CacheReadTokens: -10,
					CacheWriteTokens: -10
				})
			)
		).toBe(0);
		expect(averageDurationOf(row({ Requests: 10, DurationMs: -20 }))).toBe(0);
		expect(errorRateOf(row({ Requests: 10, Errors: -1 }))).toBe(0);
	});
});

describe('usage search', () => {
	const rows = [
		entry('alpha', {}, { label: 'Claude.ai', id: 'provider-1' }),
		entry('beta', {}, { label: 'Work account', id: 'acct-2', searchText: 'ChatGPT' }),
		entry('gamma', {}, { label: 'Unattributed', id: 'raw-key' })
	];

	it('matches labels, identifiers, keys, and related text case-insensitively', () => {
		expect(searchUsageRows(rows, ' claude ').map((item) => item.key)).toEqual(['alpha']);
		expect(searchUsageRows(rows, 'ACCT-2').map((item) => item.key)).toEqual(['beta']);
		expect(searchUsageRows(rows, 'gamma').map((item) => item.key)).toEqual(['gamma']);
		expect(searchUsageRows(rows, 'chatgpt').map((item) => item.key)).toEqual(['beta']);
	});

	it('keeps every row for an empty query and returns none for a miss', () => {
		expect(searchUsageRows(rows, '   ')).toHaveLength(3);
		expect(searchUsageRows(rows, 'nothing')).toEqual([]);
	});
});

describe('usage sorting', () => {
	const rows = [
		entry('b', { Requests: 20, Errors: 5, CostMicros: 1_000_000 }, { label: 'Beta' }),
		entry('a', { Requests: 10, Errors: 1, CostMicros: 3_000_000 }, { label: 'Alpha' }),
		entry(
			'q',
			{ Requests: 0, Errors: 0, CostMicros: 0, DurationMs: 0, DurationMaxMs: 0 },
			{ label: 'Quiet', spend: { text: '—', weight: 0 } }
		)
	];

	it('orders names and numbers in both directions', () => {
		expect(sortUsageRows(rows, { key: 'name', descending: false }).map((item) => item.key)).toEqual(
			['a', 'b', 'q']
		);
		expect(sortUsageRows(rows, { key: 'spend', descending: true }).map((item) => item.key)).toEqual(
			['a', 'b', 'q']
		);
		expect(
			sortUsageRows(rows, { key: 'spend', descending: false }).map((item) => item.key)
		).toEqual(['q', 'b', 'a']);
	});

	it('keeps missing rates last in both directions and breaks ties by key', () => {
		const tied = [entry('z', { Requests: 10, Errors: 1 }), entry('y', { Requests: 10, Errors: 1 })];
		expect(
			sortUsageRows([...rows, ...tied], { key: 'errors', descending: true }).slice(-1)[0].key
		).toBe('q');
		expect(
			sortUsageRows(tied, { key: 'errors', descending: false }).map((item) => item.key)
		).toEqual(['y', 'z']);
	});

	it('orders every other field the list offers', () => {
		expect(sortUsageRows(rows, { key: 'requests', descending: true }).map((i) => i.key)).toEqual([
			'b',
			'a',
			'q'
		]);
		// Both trained rows carry the same token count, so the stable group key decides.
		expect(sortUsageRows(rows, { key: 'tokens', descending: true }).map((i) => i.key)).toEqual([
			'a',
			'b',
			'q'
		]);
		// The quieter row served fewer requests over the same total time, so its average is higher.
		expect(sortUsageRows(rows, { key: 'latency', descending: true }).map((i) => i.key)).toEqual([
			'a',
			'b',
			'q'
		]);
		expect(sortUsageRows(rows, { key: 'name', descending: true }).map((i) => i.key)).toEqual([
			'q',
			'b',
			'a'
		]);
	});

	it('breaks an equal value by the stable group key in both directions', () => {
		const equal = [entry('y', { Requests: 10 }), entry('x', { Requests: 10 })];
		expect(sortUsageRows(equal, { key: 'requests', descending: true }).map((i) => i.key)).toEqual([
			'x',
			'y'
		]);
		expect(sortUsageRows(equal, { key: 'requests', descending: false }).map((i) => i.key)).toEqual([
			'x',
			'y'
		]);
	});
});

describe('usage insights', () => {
	const rows = [
		entry(
			'small',
			{ Requests: 49, Errors: 10, DurationMs: 90_000 },
			{ spend: { text: '$1.00', weight: 1_000_000 } }
		),
		entry(
			'busy',
			{ Requests: 100, Errors: 20, DurationMs: 10_000 },
			{ spend: { text: '$5.00', weight: 5_000_000 } }
		),
		entry(
			'fast',
			{ Requests: 60, Errors: 1, DurationMs: 1_000 },
			{ spend: { text: '$2.00', weight: 2_000_000 } }
		)
	];

	it('picks the most valuable group and only samples rate findings from 50 requests', () => {
		const insights = usageInsights(rows, 'spend');
		expect(insights[0]).toMatchObject({ kind: 'highest', share: 0.625 });
		expect(insights[1]).toMatchObject({ kind: 'error_rate', rate: 0.2 });
		expect(insights[2]).toMatchObject({ kind: 'latency' });
	});

	it('falls back to the busiest group when no group has enough rate traffic', () => {
		const insights = usageInsights([entry('only', { Requests: 20, Errors: 10 })], 'requests');
		expect(insights.map((insight) => insight.kind)).toEqual(['highest', 'requests']);
	});

	it('does not headline an unnamed group', () => {
		const insights = usageInsights(
			[entry('', { Requests: 500 }), entry('named', { Requests: 20 })],
			'requests'
		);
		expect(insights[0]).toMatchObject({ kind: 'highest', entry: { key: 'named' } });
	});

	it('headlines nothing when every group is empty or unnamed', () => {
		expect(usageInsights([], 'spend')).toEqual([]);
		expect(usageInsights([entry('', { Requests: 0 })], 'requests')).toEqual([]);
		// A group whose only traffic is unpriced still has no spend to share.
		expect(
			usageInsights([entry('free', { Requests: 10 }, { spend: { text: '—', weight: 0 } })], 'spend')
		).toEqual([{ kind: 'requests', entry: expect.objectContaining({ key: 'free' }) }]);
	});

	it('breaks an equal insight by the stable group key and reads latency from the sample', () => {
		const tied = [
			entry('z', { Requests: 100, Errors: 10, DurationMs: 10_000 }),
			entry('y', { Requests: 100, Errors: 10, DurationMs: 10_000 })
		];
		const insights = usageInsights(tied, 'requests');
		expect(insights[1]).toMatchObject({ kind: 'error_rate', entry: { key: 'y' } });
		expect(insights[2]).toMatchObject({ kind: 'latency', entry: { key: 'y' }, averageMs: 100 });
	});
});

describe('usage shares', () => {
	it('keeps the top five and folds the rest into one tail', () => {
		const rows = Array.from({ length: 7 }, (_, index) =>
			entry('g' + index, { Requests: 7 - index })
		);
		const shares = shareSegments(rows, 'requests');
		expect(shares).toHaveLength(6);
		expect(shares[5]).toMatchObject({ key: SHARE_OTHER_KEY, count: 2, value: 3 });
		expect(shares.reduce((sum, segment) => sum + segment.share, 0)).toBeCloseTo(100);
	});

	it('uses the row weights for a connection share and skips an all-zero metric', () => {
		const rows = [
			entry('local', { Requests: 100 }, { spend: { text: '—', weight: 0 } }),
			entry('paid', { Requests: 100 }, { spend: { text: '$2.00', weight: 2_000_000 } })
		];
		expect(shareSegments(rows, 'spend').map((segment) => segment.key)).toEqual(['paid']);
		expect(
			shareSegments([entry('one', { Errors: 1 }), entry('two', { Errors: 0 })], 'errors')
		).toHaveLength(1);
		expect(shareSegments([entry('quiet', { Requests: 0, Errors: 0 })], 'requests')).toEqual([]);
	});

	it('shares out tokens and breaks an equal value by the stable group key', () => {
		const rows = [entry('b', { Requests: 10 }), entry('a', { Requests: 10 })];
		expect(shareSegments(rows, 'tokens').map((segment) => segment.key)).toEqual(['a', 'b']);
		expect(shareSegments(rows, 'tokens').every((segment) => segment.share === 50)).toBe(true);
	});

	it('never counts a negative figure as a share', () => {
		expect(shareSegments([entry('negative', { Errors: -5 })], 'errors')).toEqual([]);
	});
});

describe('usage sort address', () => {
	it('round-trips a valid choice and falls back from a bad one', () => {
		const fallback = { key: 'spend' as const, descending: true };
		expect(parseSort('name:asc', fallback)).toEqual({ key: 'name', descending: false });
		expect(parseSort('tokens:desc', fallback)).toEqual({ key: 'tokens', descending: true });
		expect(parseSort('requests:desc', fallback)).toEqual({ key: 'requests', descending: true });
		expect(parseSort('errors:asc', fallback)).toEqual({ key: 'errors', descending: false });
		expect(parseSort('latency:desc', fallback)).toEqual({ key: 'latency', descending: true });
		expect(parseSort('spend:asc', fallback)).toEqual({ key: 'spend', descending: false });
		expect(parseSort('bogus:desc', fallback)).toBe(fallback);
		expect(parseSort('name:sideways', fallback)).toBe(fallback);
		expect(parseSort('name', fallback)).toBe(fallback);
		expect(parseSort(null, fallback)).toBe(fallback);
		expect(parseSort('', fallback)).toBe(fallback);
		expect(sortParam({ key: 'name', descending: false })).toBe('name:asc');
		expect(sortParam({ key: 'spend', descending: true })).toBe('spend:desc');
	});

	it('defaults days to newest first and other groups to highest value', () => {
		expect(defaultSort('day')).toEqual({ key: 'name', descending: true });
		expect(defaultSort('provider')).toEqual({ key: 'spend', descending: true });
		expect(defaultSort('model')).toEqual({ key: 'spend', descending: true });
		expect(defaultSort('account')).toEqual({ key: 'spend', descending: true });
		expect(defaultSort('client')).toEqual({ key: 'spend', descending: true });
	});
});
