import { describe, expect, it } from 'vitest';

import {
	CHART_METRICS,
	barGeom,
	barRows,
	bucketDays,
	donutFill,
	donutSegments,
	formatChartValue,
	heatLevel,
	heatmapCells,
	latencySeries,
	metricValueOf,
	rankedShares,
	reliabilitySeries,
	shortDay,
	tokenMixSeries,
	trendTotal
} from './usage-charts';
import type { UsageRollupRow } from './types';

function row(overrides: Partial<UsageRollupRow> = {}): UsageRollupRow {
	return {
		Key: 'grok',
		Requests: 4,
		Errors: 1,
		InputTokens: 300,
		OutputTokens: 100,
		CacheReadTokens: 0,
		CacheWriteTokens: 0,
		CostMicros: 2_000_000,
		UnpricedRequests: 0,
		DurationMs: 8_000,
		Attempts: 4,
		RetriedRequests: 0,
		DurationMaxMs: 3_000,
		...overrides
	};
}

describe('metricValueOf', () => {
	it('reads every series off one row', () => {
		const sample = row();
		expect(metricValueOf(sample, 'requests')).toBe(4);
		expect(metricValueOf(sample, 'tokens')).toBe(400);
		expect(metricValueOf(sample, 'spend')).toBe(2_000_000);
		expect(metricValueOf(sample, 'errors')).toBe(1);
		expect(metricValueOf(sample, 'latency')).toBe(2_000);
	});

	it('has no latency to claim for a group that served nothing', () => {
		expect(metricValueOf(row({ Requests: 0, DurationMs: 5_000 }), 'latency')).toBe(0);
	});
});

describe('formatChartValue', () => {
	it('renders each series in its own unit', () => {
		expect(formatChartValue(1_500, 'requests')).toBe('1,500');
		expect(formatChartValue(1_500, 'tokens')).toBe('1.5k');
		expect(formatChartValue(2_000_000, 'spend')).toBe('$2.00');
		expect(formatChartValue(1_500, 'latency')).toBe('1.5 s');
		expect(formatChartValue(0, 'latency')).toBe('—');
	});
});

describe('trendTotal', () => {
	it('sums what can be summed', () => {
		const rows = [row(), row({ Key: 'other', CostMicros: 1_000_000 })];
		expect(trendTotal(rows, 'spend')).toBe(3_000_000);
		expect(trendTotal(rows, 'requests')).toBe(8);
	});

	it('averages latency over the requests of the range', () => {
		const rows = [
			row({ Requests: 2, DurationMs: 2_000 }),
			row({ Requests: 6, DurationMs: 12_000 })
		];
		expect(trendTotal(rows, 'latency')).toBe(1_750);
		expect(trendTotal([], 'latency')).toBe(0);
	});
});

describe('barRows', () => {
	it('orders by the chosen series and keeps the busiest rows', () => {
		const rows = [
			row({ Key: 'a', CostMicros: 1 }),
			row({ Key: 'b', CostMicros: 100 }),
			row({ Key: 'c', CostMicros: 50 })
		];
		expect(barRows(rows, 'spend').map((bar) => bar.key)).toEqual(['b', 'c', 'a']);
		expect(barRows(rows, 'spend', 2).map((bar) => bar.key)).toEqual(['b', 'c']);
	});

	it('draws the longest bar full and floors a small one', () => {
		const rows = [row({ Key: 'a', Requests: 100 }), row({ Key: 'b', Requests: 1 })];
		const bars = barRows(rows, 'requests');
		expect(bars[0].share).toBe(100);
		expect(bars[1].share).toBe(2);
	});

	it('leaves out a group with no name and one with nothing to show', () => {
		const rows = [
			row({ Key: '', Requests: 5 }),
			row({ Key: 'quiet', Requests: 0 }),
			row({ Key: 'busy' })
		];
		expect(barRows(rows, 'requests').map((bar) => bar.key)).toEqual(['busy']);
	});

	it('accepts every series the switch offers', () => {
		for (const metric of CHART_METRICS) {
			expect(() => barRows([row()], metric)).not.toThrow();
		}
	});
});

describe('tokenMixSeries', () => {
	it('stacks one day into the four kinds the ledger stores', () => {
		const [point] = tokenMixSeries([
			row({
				Key: '2026-09-01',
				InputTokens: 300,
				OutputTokens: 100,
				CacheReadTokens: 50,
				CacheWriteTokens: 25
			})
		]);
		expect(point).toEqual({
			key: '2026-09-01',
			input: 300,
			output: 100,
			cacheRead: 50,
			cacheWrite: 25,
			total: 475
		});
	});

	it('orders oldest day first and keeps a quiet day in place', () => {
		const points = tokenMixSeries([
			row({ Key: '2026-09-02', Requests: 2 }),
			row({ Key: '2026-09-01', Requests: 0, InputTokens: 0, OutputTokens: 0 })
		]);
		expect(points.map((point) => point.key)).toEqual(['2026-09-01', '2026-09-02']);
		expect(points[0].total).toBe(0);
	});
});

describe('reliabilitySeries', () => {
	it('reads errors, retries and the rate off one day', () => {
		const [point] = reliabilitySeries([
			row({ Key: '2026-09-01', Requests: 4, Errors: 1, RetriedRequests: 2 })
		]);
		expect(point).toEqual({
			key: '2026-09-01',
			requests: 4,
			errors: 1,
			retried: 2,
			errorRate: 0.25
		});
	});

	it('claims no rate for a quiet day', () => {
		const [point] = reliabilitySeries([row({ Key: '2026-09-01', Requests: 0, Errors: 0 })]);
		expect(point.errorRate).toBe(0);
	});
});

describe('latencySeries', () => {
	it('reads the average and the slowest request of one day', () => {
		const [point] = latencySeries([
			row({ Key: '2026-09-01', Requests: 4, DurationMs: 8_000, DurationMaxMs: 3_000 })
		]);
		expect(point).toEqual({ key: '2026-09-01', requests: 4, average: 2_000, max: 3_000 });
	});

	it('claims neither for a quiet day', () => {
		const [point] = latencySeries([
			row({ Key: '2026-09-01', Requests: 0, DurationMs: 5_000, DurationMaxMs: 3_000 })
		]);
		expect(point.average).toBe(0);
		expect(point.max).toBe(0);
	});
});

describe('rankedShares', () => {
	it('ranks by value and shares what the list shows', () => {
		const shares = rankedShares([
			{ key: 'a', value: 1_000_000 },
			{ key: 'b', value: 3_000_000 },
			{ key: 'c', value: 6_000_000 }
		]);
		expect(shares.map((share) => share.key)).toEqual(['c', 'b', 'a']);
		expect(shares[0].share).toBe(60);
		expect(shares[1].share).toBe(30);
		expect(shares[2].share).toBe(10);
	});

	it('leaves out an unnamed entry and one with nothing to show', () => {
		const shares = rankedShares([
			{ key: '', value: 9_000_000 },
			{ key: 'quiet', value: 0 },
			{ key: 'busy', value: 2_000_000 }
		]);
		expect(shares.map((share) => share.key)).toEqual(['busy']);
		expect(shares[0].share).toBe(100);
	});

	it('keeps the largest few of a long list', () => {
		const entries = Array.from({ length: 8 }, (_, index) => ({
			key: 'k' + index,
			value: (index + 1) * 1_000_000
		}));
		const shares = rankedShares(entries, 6);
		expect(shares).toHaveLength(6);
		expect(shares[0].key).toBe('k7');
	});
});

describe('spendShareRows', () => {
	it('ranks by cost and shares what the list shows', () => {
		const shares = rankedShares([
			{ key: 'a', value: 1_000_000 },
			{ key: 'b', value: 3_000_000 },
			{ key: 'c', value: 6_000_000 }
		]);
		expect(shares.map((share) => share.key)).toEqual(['c', 'b', 'a']);
		expect(shares[0].share).toBe(60);
		expect(shares[1].share).toBe(30);
		expect(shares[2].share).toBe(10);
	});
});

describe('donutSegments', () => {
	it('lays the shares end to end around one turn', () => {
		const segments = donutSegments(
			rankedShares([
				{ key: 'a', value: 1_000_000 },
				{ key: 'b', value: 3_000_000 },
				{ key: 'c', value: 6_000_000 }
			])
		);
		expect(segments.map((segment) => segment.key)).toEqual(['c', 'b', 'a']);
		expect(segments[0]).toMatchObject({ start: 0, sweep: 60 });
		expect(segments[1]).toMatchObject({ start: 60, sweep: 30 });
		expect(segments[2]).toMatchObject({ start: 90, sweep: 10 });
		const covered = segments.reduce((sum, segment) => sum + segment.sweep, 0);
		expect(covered).toBeCloseTo(100, 6);
	});

	it('draws nothing from an empty list', () => {
		expect(donutSegments([])).toEqual([]);
	});

	it('walks the accent ladder by rank', () => {
		expect(donutFill(0)).toBe('var(--accent)');
		expect(donutFill(5)).toBe('var(--line-strong)');
		expect(donutFill(6)).toBe(donutFill(0));
	});
});

describe('heatmapCells', () => {
	it('folds days into Monday-first week columns', () => {
		// 2026-09-30 is a Wednesday.
		const weeks = heatmapCells([
			{ key: '2026-09-30', value: 4 },
			{ key: '2026-10-01', value: 8 },
			{ key: '2026-10-04', value: 2 }
		]);
		expect(weeks).toHaveLength(1);
		expect(weeks[0].map((cell) => cell.key)).toEqual([
			'2026-09-28',
			'2026-09-29',
			'2026-09-30',
			'2026-10-01',
			'2026-10-02',
			'2026-10-03',
			'2026-10-04'
		]);
		expect(weeks[0][2]).toMatchObject({ value: 4, level: 2 });
		expect(weeks[0][3]).toMatchObject({ value: 8, level: 4 });
		expect(weeks[0][4]).toMatchObject({ value: 0, level: 0 });
	});

	it('starts a new column on the next Monday', () => {
		const weeks = heatmapCells([
			{ key: '2026-09-28', value: 1 },
			{ key: '2026-10-05', value: 1 }
		]);
		expect(weeks).toHaveLength(2);
		expect(weeks[0][0].key).toBe('2026-09-28');
		expect(weeks[1][0].key).toBe('2026-10-05');
	});

	it('leaves out a key that is not a day and answers empty with empty', () => {
		expect(heatmapCells([{ key: 'soon', value: 3 }])).toEqual([]);
		expect(heatmapCells([])).toEqual([]);
	});
});

describe('heatLevel', () => {
	it('buckets a day against the busiest one and keeps zero quiet', () => {
		expect(heatLevel(0, 8)).toBe(0);
		expect(heatLevel(2, 8)).toBe(1);
		expect(heatLevel(8, 8)).toBe(4);
		expect(heatLevel(5, 0)).toBe(0);
	});
});

describe('bucketDays', () => {
	function day(key: string, requests: number): UsageRollupRow {
		return row({
			Key: key,
			Requests: requests,
			DurationMs: requests * 1_000,
			DurationMaxMs: requests * 100
		});
	}

	it('passes a short series through in day order', () => {
		const rows = [day('2026-09-03', 3), day('2026-09-01', 1), day('2026-09-02', 2)];
		const buckets = bucketDays(rows, 92);
		expect(buckets.map((bucket) => bucket.Key)).toEqual(['2026-09-01', '2026-09-02', '2026-09-03']);
		expect(buckets.map((bucket) => bucket.Requests)).toEqual([1, 2, 3]);
	});

	it('folds a wide window into whole columns that add up', () => {
		const rows = Array.from({ length: 10 }, (_, index) =>
			day('2026-09-' + String(index + 1).padStart(2, '0'), index + 1)
		);
		const buckets = bucketDays(rows, 4);
		expect(buckets).toHaveLength(4);
		// 10 days into 4 columns of 3, 3, 3 and 1.
		expect(buckets.map((bucket) => bucket.Requests)).toEqual([6, 15, 24, 10]);
		expect(buckets[0].Key).toBe('2026-09-01');
		// The slowest request survives the fold, and latency still divides the
		// summed durations by the summed requests.
		expect(buckets[0].DurationMaxMs).toBe(300);
		expect(metricValueOf(buckets[0], 'latency')).toBe(1_000);
	});
});

describe('barGeom', () => {
	it('lays columns on the shared baseline, tallest at the series maximum', () => {
		expect(barGeom(0, 1, 84, 84)).toEqual({ x: 0, y: 0, w: 640, h: 84 });
		expect(barGeom(0, 1, 42, 84)).toEqual({ x: 0, y: 42, w: 640, h: 42 });
	});

	it('keeps a quiet column visible with a 3px floor', () => {
		expect(barGeom(0, 1, 0, 84).h).toBe(3);
	});

	it('shares the width between columns, gap included', () => {
		const first = barGeom(0, 2, 1, 1);
		const second = barGeom(1, 2, 1, 1);
		expect(first.w).toBe(317);
		expect(second.x).toBe(323);
		expect(first.x + first.w).toBeLessThan(second.x);
	});
});

describe('shortDay', () => {
	it('names a day the way a chart axis reads it', () => {
		expect(shortDay('2026-09-03')).toBe(
			new Date('2026-09-03T00:00:00Z').toLocaleDateString(undefined, {
				month: 'short',
				day: 'numeric',
				timeZone: 'UTC'
			})
		);
	});

	it('names a key that is not a day as it arrived', () => {
		expect(shortDay('openai')).toBe('openai');
		expect(shortDay('')).toBe('');
	});
});
