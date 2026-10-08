import { describe, expect, it } from 'vitest';

import { chosenMetrics, metricLabelKey, metricValue, metricValues } from './usage-metrics';
import { EMPTY_TOTALS, type UsageTotals } from './usage-filters';

// One read with a figure in every field, so a metric that reads a total is
// exercised with something to state rather than with the empty default.
const BUSY: UsageTotals = {
	requests: 200,
	errors: 5,
	inputTokens: 1_000_000,
	outputTokens: 400_000,
	totalTokens: 1_400_000,
	spendMicros: 4_200_000,
	planUsageMicros: 1_500_000,
	unpricedRequests: 20,
	cacheReadTokens: 300_000,
	cacheWriteTokens: 50_000,
	durationMs: 40_000,
	attempts: 260,
	retriedRequests: 60,
	durationMaxMs: 9_000
};

describe('metricValue', () => {
	it('renders every metric the catalog names', () => {
		const ids = [
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
			'spend_per_request',
			'plan_usage',
			'priced_requests',
			'unpriced_requests',
			'duration_avg',
			'duration_max'
		];
		for (const id of ids) {
			const value = metricValue(id, BUSY);
			expect(value, id + ' rendered nothing').not.toBeNull();
			expect(value?.id).toBe(id);
			// Every card states a figure: a metric that drew nothing would be a
			// card with no number on it.
			expect(value?.value, id + ' drew no figure').not.toBe('');
			expect(value?.exact, id + ' has no exact figure').not.toBe('');
		}
	});

	it('renders nothing for a metric this build does not know', () => {
		expect(metricValue('not_a_metric', BUSY)).toBeNull();
		expect(metricValue('', BUSY)).toBeNull();
	});

	it('counts a request that succeeded as the requests less the errors', () => {
		expect(metricValue('successes', BUSY)?.value).toBe('195');
		expect(metricValue('requests', BUSY)?.value).toBe('200');
	});

	it('marks an error card only while there are errors', () => {
		expect(metricValue('errors', BUSY)?.danger).toBe(true);
		expect(metricValue('error_rate', BUSY)?.danger).toBe(true);
		expect(metricValue('success_rate', BUSY)?.danger).toBe(false);

		const quiet = { ...BUSY, errors: 0 };
		expect(metricValue('errors', quiet)?.danger).toBe(false);
		expect(metricValue('error_rate', quiet)?.danger).toBe(false);
	});

	it('states a rate to one decimal place and a dash without a whole', () => {
		// 195 of 200 succeeded, and 5 of 200 errored.
		expect(metricValue('success_rate', BUSY)?.value).toBe('97.5%');
		expect(metricValue('error_rate', BUSY)?.value).toBe('2.5%');

		expect(metricValue('success_rate', EMPTY_TOTALS)?.value).toBe('—');
		expect(metricValue('error_rate', EMPTY_TOTALS)?.value).toBe('—');
		expect(metricValue('retried', EMPTY_TOTALS)?.noteValues.value).toBe('—');
	});

	it('divides an average by the requests behind it', () => {
		expect(metricValue('attempts', BUSY)?.noteValues.value).toBe('1.30');
		expect(metricValue('duration_avg', BUSY)?.noteKey).toBe('noteSlowest');
		expect(metricValue('duration_max', BUSY)?.noteValues.value).not.toBe('—');

		// No requests means there is nothing to average.
		expect(metricValue('attempts', EMPTY_TOTALS)?.noteValues.value).toBe('—');
		expect(metricValue('tokens_per_request', EMPTY_TOTALS)?.value).toBe('—');
		expect(metricValue('duration_max', EMPTY_TOTALS)?.value).toBe('—');
	});

	it('reports the slowest request the average hides', () => {
		expect(metricValue('duration_avg', BUSY)?.noteValues.value).not.toBe(
			metricValue('duration_avg', { ...BUSY, durationMaxMs: 0 })?.noteValues.value
		);
		expect(metricValue('duration_avg', { ...BUSY, durationMaxMs: 0 })?.noteValues.value).toBe('—');
	});

	it('separates the requests that carry a price from the ones that do not', () => {
		expect(metricValue('priced_requests', BUSY)?.value).toBe('180');
		expect(metricValue('unpriced_requests', BUSY)?.value).toBe('20');
		expect(metricValue('spend', BUSY)?.noteValues.value).toBe('20');
		// A per-request figure is only meaningful over the priced requests.
		expect(metricValue('spend_per_request', EMPTY_TOTALS)?.value).toBe('—');
	});

	it('states plan usage beside API spend, and says what it is', () => {
		expect(metricValue('spend', BUSY)?.value).toBe('$4.20');
		expect(metricValue('plan_usage', BUSY)?.value).toBe('$1.50');
		expect(metricValue('plan_usage', BUSY)?.noteKey).toBe('notePlanUsage');
		expect(metricValue('plan_usage', EMPTY_TOTALS)?.value).toBe('$0.00');
	});

	it('never states a negative figure', () => {
		// A read that reported more errors than requests is a broken read, not
		// a negative count of successes.
		const inconsistent = { ...BUSY, requests: 3, errors: 10 };
		expect(metricValue('successes', inconsistent)?.value).toBe('0');
		expect(
			metricValue('priced_requests', {
				...BUSY,
				requests: 3,
				unpricedRequests: 10
			})?.value
		).toBe('0');
	});
});

describe('metricValues', () => {
	it('renders the chosen metrics in the order chosen', () => {
		const values = metricValues(['spend', 'requests'], BUSY);
		expect(values.map((value) => value.id)).toEqual(['spend', 'requests']);
	});

	it('skips a metric this build does not know', () => {
		const values = metricValues(['requests', 'not_a_metric', 'errors'], BUSY);
		expect(values.map((value) => value.id)).toEqual(['requests', 'errors']);
	});

	it('renders nothing for no choice', () => {
		expect(metricValues([], BUSY)).toEqual([]);
	});
});

describe('chosenMetrics', () => {
	it('keeps a stored choice the daemon still offers, in the stored order', () => {
		expect(chosenMetrics(['spend', 'requests'], ['requests', 'spend', 'errors'])).toEqual([
			'spend',
			'requests'
		]);
	});

	it('drops a choice the daemon dropped out', () => {
		expect(chosenMetrics(['spend', 'gone', 'requests'], ['requests', 'spend'])).toEqual([
			'spend',
			'requests'
		]);
	});

	it('names a card once however often it was chosen', () => {
		expect(chosenMetrics(['spend', 'spend'], ['spend'])).toEqual(['spend']);
	});

	it('keeps nothing when nothing is chosen', () => {
		expect(chosenMetrics([], ['requests'])).toEqual([]);
	});
});

describe('metricLabelKey', () => {
	it('names the catalogue entry of one metric', () => {
		expect(metricLabelKey('spend')).toBe('ui.pages.usagePage.metric.spend');
	});
});
