import { describe, expect, it } from 'vitest';

import { connectionAttention, unpricedTotal } from './provider-attention';
import { providerOf, quotaWindowOf } from './provider-fixtures';

describe('connection attention', () => {
	it('raises the worst window of a connection above the threshold', () => {
		const items = connectionAttention(
			[providerOf({ id: 'claude', label: 'Claude.ai' })],
			new Map([
				[
					'claude',
					[
						quotaWindowOf({ connection_id: 'claude', window: '5h', used_percent: 12 }),
						quotaWindowOf({
							connection_id: 'claude',
							window: '7d',
							used_percent: 100,
							reset_at_ms: 60_000
						})
					]
				]
			])
		);

		expect(items).toEqual([
			{
				providerId: 'claude',
				label: 'Claude.ai',
				window: '7d',
				percent: 100,
				resetAtMs: 60_000,
				tone: 'danger'
			}
		]);
	});

	it('keeps a connection under the threshold out of the strip', () => {
		const items = connectionAttention(
			[providerOf({ id: 'claude' })],
			new Map([['claude', [quotaWindowOf({ connection_id: 'claude', used_percent: 89 })]]])
		);

		expect(items).toEqual([]);
	});

	it('never raises a money reading', () => {
		const items = connectionAttention(
			[providerOf({ id: 'deepseek' })],
			new Map([
				[
					'deepseek',
					[
						quotaWindowOf({
							connection_id: 'deepseek',
							window: 'balance',
							used_percent: 100,
							amount: 0.39,
							currency: 'USD'
						})
					]
				]
			])
		);

		expect(items).toEqual([]);
	});

	it('sorts the worst reading first', () => {
		const items = connectionAttention(
			[providerOf({ id: 'a', label: 'A' }), providerOf({ id: 'b', label: 'B' })],
			new Map([
				['a', [quotaWindowOf({ connection_id: 'a', used_percent: 92 })]],
				['b', [quotaWindowOf({ connection_id: 'b', used_percent: 100 })]]
			])
		);

		expect(items.map((item) => item.providerId)).toEqual(['b', 'a']);
	});

	it('counts every unpriced model', () => {
		const total = unpricedTotal([
			providerOf({ counts: { ...providerOf().counts, unpriced_models: 18 } }),
			providerOf({ counts: { ...providerOf().counts, unpriced_models: 4 } })
		]);

		expect(total).toBe(22);
	});
});
