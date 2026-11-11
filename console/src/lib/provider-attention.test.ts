import { describe, expect, it } from 'vitest';

import { connectionAttention, unpricedTotal } from './provider-attention';
import { providerOf, quotaWindowOf, accountOf } from './provider-fixtures';

function cardOf(
	provider: ReturnType<typeof providerOf>,
	accounts: ReturnType<typeof accountOf>[] = [accountOf({ id: 'acc-1', provider_id: provider.id })],
	windows: ReturnType<typeof quotaWindowOf>[] = []
) {
	return {
		provider,
		accounts,
		windows: windows.map((window) => ({ ...window, connection_id: provider.id }))
	};
}

describe('connection attention', () => {
	it('raises the worst window of a connection above the threshold', () => {
		const items = connectionAttention([
			cardOf(
				providerOf({ id: 'claude', label: 'Claude.ai' }),
				[accountOf({ id: 'acc-1', provider_id: 'claude' })],
				[
					quotaWindowOf({ connection_id: 'claude', window: '5h', used_percent: 12 }),
					quotaWindowOf({
						connection_id: 'claude',
						window: '7d',
						used_percent: 100,
						reset_at_ms: 60_000
					})
				]
			)
		]);

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
		const items = connectionAttention([
			cardOf(
				providerOf({ id: 'claude' }),
				[accountOf({ id: 'acc-1', provider_id: 'claude' })],
				[quotaWindowOf({ connection_id: 'claude', used_percent: 89 })]
			)
		]);

		expect(items).toEqual([]);
	});

	it('never raises a money reading', () => {
		const items = connectionAttention([
			cardOf(
				providerOf({ id: 'deepseek' }),
				[accountOf({ id: 'acc-1', provider_id: 'deepseek' })],
				[
					quotaWindowOf({
						connection_id: 'deepseek',
						window: 'balance',
						used_percent: 100,
						amount: 0.39,
						currency: 'USD'
					})
				]
			)
		]);

		expect(items).toEqual([]);
	});

	it('never raises a pool of a larger allowance', () => {
		const items = connectionAttention([
			cardOf(
				providerOf({ id: 'cursor' }),
				[accountOf({ id: 'acc-1', provider_id: 'cursor' })],
				[
					quotaWindowOf({ connection_id: 'cursor', window: '30d', used_percent: 46.7 }),
					quotaWindowOf({ connection_id: 'cursor', window: 'api', used_percent: 100, sub: true })
				]
			)
		]);

		expect(items).toEqual([]);
	});

	it('leaves a connection alone while one account can still serve', () => {
		const items = connectionAttention([
			cardOf(
				providerOf({ id: 'opencode-go' }),
				[
					accountOf({ id: 'acc-1', provider_id: 'opencode-go', label: 'Bison Humble' }),
					accountOf({ id: 'acc-2', provider_id: 'opencode-go', label: 'Bison Energetic' })
				],
				[
					quotaWindowOf({
						credential_id: 'acc-1',
						connection_id: 'opencode-go',
						window: '7d',
						used_percent: 100
					}),
					quotaWindowOf({
						credential_id: 'acc-2',
						connection_id: 'opencode-go',
						window: '7d',
						used_percent: 61
					})
				]
			)
		]);

		expect(items).toEqual([]);
	});

	it('sorts the worst reading first', () => {
		const items = connectionAttention([
			cardOf(
				providerOf({ id: 'a', label: 'A' }),
				[accountOf({ id: 'acc-a', provider_id: 'a' })],
				[quotaWindowOf({ credential_id: 'acc-a', connection_id: 'a', used_percent: 92 })]
			),
			cardOf(
				providerOf({ id: 'b', label: 'B' }),
				[accountOf({ id: 'acc-b', provider_id: 'b' })],
				[quotaWindowOf({ credential_id: 'acc-b', connection_id: 'b', used_percent: 100 })]
			)
		]);

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
