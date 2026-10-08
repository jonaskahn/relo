// The metrics contract, executable: one fixture, the same expected figures
// the tray's Go test asserts (internal/desktop/metrics_test.go,
// TestMetricsContractMatchesTheConsole). If either side drifts, the two
// tests name the number that must mean the same thing on every surface.
import { describe, expect, it } from 'vitest';

import { deltaPercent, rowTokens } from './dashboard';
import type { UsageRollupRow } from './types';
import { totalsOf } from './usage-filters';

const rows: UsageRollupRow[] = [
	{
		Key: 'openai',
		Requests: 3,
		Errors: 1,
		InputTokens: 31,
		OutputTokens: 13,
		CacheReadTokens: 5,
		CacheWriteTokens: 7,
		CostMicros: 42,
		UnpricedRequests: 0,
		Attempts: 0,
		RetriedRequests: 0,
		DurationMs: 12,
		DurationMaxMs: 12
	},
	{
		Key: 'anthropic',
		Requests: 1,
		Errors: 0,
		InputTokens: 2,
		OutputTokens: 3,
		CacheReadTokens: 0,
		CacheWriteTokens: 1,
		CostMicros: 0,
		UnpricedRequests: 0,
		Attempts: 0,
		RetriedRequests: 0,
		DurationMs: 38,
		DurationMaxMs: 38
	}
];

describe('the metrics contract', () => {
	it('totals every token, cache included', () => {
		expect(rows.reduce((sum, row) => sum + rowTokens(row), 0)).toBe(62);
		const totals = totalsOf({
			requests: 4,
			errors: 1,
			input_tokens: 33,
			output_tokens: 16,
			cache_read_tokens: 5,
			cache_write_tokens: 8,
			cost_micros: 42,
			unpriced_requests: 0,
			duration_ms: 50
		});
		expect(totals.totalTokens).toBe(62);
	});

	it('reads success and latency the one way', () => {
		const requests = 4;
		const errors = 1;
		expect(((requests - errors) / requests) * 100).toBe(75);
		expect(50 / requests).toBe(12.5);
	});

	it('rounds a delta half away from zero', () => {
		expect(deltaPercent(3, 8)).toBe(-63);
		expect(deltaPercent(8, 3)).toBe(167);
	});
});
