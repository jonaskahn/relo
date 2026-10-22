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
	sortParam,
	sortUsageRows,
	spendOfRow,
	tokenTexts,
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
});

describe('usage sort address', () => {
	it('round-trips a valid choice and falls back from a bad one', () => {
		const fallback = { key: 'spend' as const, descending: true };
		expect(parseSort('name:asc', fallback)).toEqual({ key: 'name', descending: false });
		expect(parseSort('tokens:desc', fallback)).toEqual({ key: 'tokens', descending: true });
		expect(parseSort('bogus:desc', fallback)).toBe(fallback);
		expect(parseSort('name:sideways', fallback)).toBe(fallback);
		expect(parseSort(null, fallback)).toBe(fallback);
		expect(sortParam({ key: 'name', descending: false })).toBe('name:asc');
	});

	it('defaults days to newest first and other groups to highest value', () => {
		expect(defaultSort('day')).toEqual({ key: 'name', descending: true });
		expect(defaultSort('provider')).toEqual({ key: 'spend', descending: true });
	});
});
