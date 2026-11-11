import type { Provider } from './types';
import { connectionQuota, type ConnectionCard } from './provider-quota';

// What the connections overview needs to raise attention: a connection whose
// accounts are running out, and models the catalog cannot price.
// A money reading (balance, spend) is a fact, not a limit, so it never raises
// attention on its own.

/** The share of a quota window at which a connection starts raising attention. A reading below it
 *  is a fact to show, not a limit to warn about. */
export const ATTENTION_PERCENT = 90;

/** One connection that needs an operator, and why. */
export interface AttentionItem {
	providerId: string;
	label: string;
	// window is the provider's own name for the period, translated by the
	// caller through quotaWindowLabelKey.
	window: string;
	percent: number;
	resetAtMs: number;
	tone: 'danger' | 'warn';
}

/** Reads the connections that need attention from the quota they reported. A connection raises
 *  attention when the account that still serves it is close to its limit, so one spent account
 *  beside a healthy one does not raise a warning the connection does not deserve. */
export function connectionAttention(cards: readonly ConnectionCard[]): AttentionItem[] {
	const items: AttentionItem[] = [];
	for (const card of cards) {
		const quota = connectionQuota(card.accounts, card.windows);
		if (quota.atLimit) {
			const worst = quota.spentWindows[0];
			items.push({
				providerId: card.provider.id,
				label: card.provider.label || card.provider.id,
				window: worst?.window ?? '',
				percent: 100,
				resetAtMs: worst?.reset_at_ms ?? 0,
				tone: 'danger'
			});
			continue;
		}
		if (quota.servingWindow === null || quota.servingPercent < ATTENTION_PERCENT) continue;
		items.push({
			providerId: card.provider.id,
			label: card.provider.label || card.provider.id,
			window: quota.servingWindow.window,
			percent: Math.round(quota.servingPercent),
			resetAtMs: quota.servingWindow.reset_at_ms,
			tone: 'warn'
		});
	}
	return items.sort((a, b) => b.percent - a.percent || a.label.localeCompare(b.label));
}

/** Counts the models that still need a price. */
export function unpricedTotal(providers: Provider[]): number {
	return providers.reduce((sum, provider) => sum + provider.counts.unpriced_models, 0);
}
