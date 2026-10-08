import type { Provider, QuotaWindow } from './types';

// What the connections overview needs to raise attention: a connection whose
// worst window is near or at its limit, and models the catalog cannot price.
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

/** Reads the connections that need attention from the quota they reported. */
export function connectionAttention(
	providers: readonly Provider[],
	windowsByConnection: Map<string, QuotaWindow[]>
): AttentionItem[] {
	const items: AttentionItem[] = [];
	for (const provider of providers) {
		let worst: QuotaWindow | null = null;
		for (const window of windowsByConnection.get(provider.id) ?? []) {
			if (window.amount !== undefined) continue;
			if (worst === null || window.used_percent > worst.used_percent) worst = window;
		}
		if (worst === null || worst.used_percent < ATTENTION_PERCENT) continue;
		items.push({
			providerId: provider.id,
			label: provider.label || provider.id,
			window: worst.window,
			percent: Math.round(worst.used_percent),
			resetAtMs: worst.reset_at_ms,
			tone: worst.used_percent >= 100 ? 'danger' : 'warn'
		});
	}
	return items.sort((a, b) => b.percent - a.percent || a.label.localeCompare(b.label));
}

/** Counts the models that still need a price. */
export function unpricedTotal(providers: Provider[]): number {
	return providers.reduce((sum, provider) => sum + provider.counts.unpriced_models, 0);
}
