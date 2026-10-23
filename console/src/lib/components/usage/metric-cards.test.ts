import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import { EMPTY_TOTALS, type UsageTotals } from '$lib/usage-filters';
import { metricValue } from '$lib/usage-metrics';
import type { UsageRollupRow } from '$lib/types';

import MetricCards from './metric-cards.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

function day(key: string, multiplier: number): UsageRollupRow {
	return {
		Key: key,
		Requests: 20 * multiplier,
		Errors: multiplier,
		InputTokens: 1_000 * multiplier,
		OutputTokens: 400 * multiplier,
		CacheReadTokens: 300 * multiplier,
		CacheWriteTokens: 100 * multiplier,
		CostMicros: 2_000 * multiplier,
		UnpricedRequests: multiplier,
		DurationMs: 40_000 * multiplier,
		Attempts: 22 * multiplier,
		RetriedRequests: 2 * multiplier,
		DurationMaxMs: 9_000 * multiplier
	};
}

const TOTALS: UsageTotals = {
	...EMPTY_TOTALS,
	requests: 60,
	errors: 3,
	inputTokens: 3_000,
	outputTokens: 1_200,
	totalTokens: 5_400,
	spendMicros: 6_000,
	planUsageMicros: 2_000,
	cacheReadTokens: 900,
	cacheWriteTokens: 300,
	durationMs: 120_000,
	attempts: 66,
	retriedRequests: 6,
	durationMaxMs: 9_000
};

const ALL_METRICS = [
	'requests',
	'successes',
	'errors',
	'success_rate',
	'error_rate',
	'attempts',
	'retried',
	'tokens_input',
	'tokens_output',
	'tokens_total',
	'tokens_per_request',
	'cache_read',
	'cache_write',
	'spend',
	'plan_usage',
	'spend_per_request',
	'priced_requests',
	'unpriced_requests',
	'duration_avg',
	'duration_max'
];

describe('the metric cards', () => {
	it('draws a data line for every metric the picker offers', () => {
		for (const id of ALL_METRICS) {
			const card = metricValue(id, TOTALS);
			expect(card, id + ' rendered no card').not.toBeNull();
			const html = render(MetricCards, {
				props: {
					cards: card ? [card] : [],
					days: [day('2026-10-21', 1), day('2026-10-22', 2)],
					planDays: [day('2026-10-21', 1), day('2026-10-22', 2)]
				}
			}).body;
			expect(html, id + ' drew no data line').toContain('viewBox="0 0 104 44"');
		}
	});
});
