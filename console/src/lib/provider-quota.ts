// The quota footer of a connection card reads the stored windows of that
// connection's accounts: which accounts it has, which windows each one
// reported, and which of them fit on the card.

import type { Account, Provider, QuotaDisplay, QuotaWindow } from './types';

/** Lists the two ways a quota chart may read, in the order the settings control shows them. */
export const QUOTA_DISPLAYS: readonly QuotaDisplay[] = ['used', 'remaining'];

/** Renders the share a quota chart presents, which is the one question every ring, dial and bar
 *  answers the same way: what is used, or what is left.
 *  A provider that reports more than its own limit is clamped rather than drawn past the end of
 *  the chart. */
export function shownPercent(usedPercent: number, display: QuotaDisplay): number {
	const used = Math.min(100, Math.max(0, usedPercent));
	return display === 'remaining' ? 100 - used : used;
}

/** Names the caption one reading carries, so a ring, a dial and a bar all state the percent in
 *  the unit the operator chose. */
export function quotaValueKey(display: QuotaDisplay): string {
	return display === 'remaining'
		? 'ui.pages.providersPage.accounts.quotaRemaining'
		: 'ui.pages.providersPage.accounts.quotaUsed';
}

/** How many windows a card draws before it stops and counts the rest.
 *  An account usually reports two, a short burst window and a weekly one, and four rings is the
 *  most that still reads at a glance. */
export const QUOTA_RINGS = 4;

/** Lists the accounts of one connection in the order the daemon returned them, which is the
 *  order every other surface reads them in. */
export function accountsOf(accounts: Account[], connectionId: string): Account[] {
	return accounts.filter((account) => account.provider_id === connectionId);
}

function groupBy<T>(items: T[], key: (item: T) => string): Map<string, T[]> {
	const grouped = new Map<string, T[]>();
	for (const item of items) {
		const held = grouped.get(key(item));
		if (held) held.push(item);
		else grouped.set(key(item), [item]);
	}
	return grouped;
}

/** Groups the stored quota by the account that reported it, which is what a card footer reads. */
export function windowsByCredential(windows: QuotaWindow[]): Map<string, QuotaWindow[]> {
	return groupBy(windows, (window) => window.credential_id);
}

/** Indexes accounts by their connection, and groupQuotaByConnection does the same for the
 *  windows a connection published, so a page hands each card its own slice in one pass. */
export function groupAccountsByConnection(accounts: Account[]): Map<string, Account[]> {
	return groupBy(accounts, (account) => account.provider_id);
}

/** Indexes the windows a connection published. */
export function groupQuotaByConnection(windows: QuotaWindow[]): Map<string, QuotaWindow[]> {
	return groupBy(windows, (window) => window.connection_id);
}

/** One connection beside the quota its accounts reported, which is everything an overview card
 *  draws and the narrowest data it needs. */
export interface ConnectionCard {
	provider: Provider;
	accounts: Account[];
	windows: QuotaWindow[];
}

/** Pairs every connection with its own slice of the accounts and the stored windows, in one pass
 *  over the two indexes. */
export function connectionCards(
	providers: readonly Provider[],
	accountsByConnection: ReadonlyMap<string, Account[]>,
	quotaByConnection: ReadonlyMap<string, QuotaWindow[]>
): ConnectionCard[] {
	return providers.map((provider) => ({
		provider,
		accounts: accountsByConnection.get(provider.id) ?? [],
		windows: quotaByConnection.get(provider.id) ?? []
	}));
}

/** Splits the windows a card draws from the ones it only counts. */
export function ringsOf(
	windows: readonly QuotaWindow[],
	limit = QUOTA_RINGS
): { shown: QuotaWindow[]; extra: number } {
	return { shown: windows.slice(0, limit), extra: Math.max(0, windows.length - limit) };
}

/** How much of a ring's outline stays undrawn for a share used.
 *  A provider that reports more than its own limit is clamped rather than drawn past the end of
 *  the circle. */
export function ringDash(percent: number, circumference: number): number {
	const share = Math.min(100, Math.max(0, percent)) / 100;
	return circumference * (1 - share);
}

/** Names the window a provider publishes. An account usually reports a short burst window and a
 *  weekly one; anything else keeps the tag the provider used. */
export function quotaWindowLabelKey(window: string): string {
	if (window === 'balance' || window.startsWith('balance-')) {
		return 'ui.pages.providersPage.accounts.quotaWindowBalance';
	}
	if (window === 'spent') return 'ui.pages.providersPage.accounts.quotaWindowSpent';
	if (window === '7d') return 'ui.pages.providersPage.accounts.quotaWindowWeekly';
	if (window === '5h') return 'ui.pages.providersPage.accounts.quotaWindowHours';
	if (window === '30d') return 'ui.pages.providersPage.accounts.quotaWindowMonthly';
	if (window === 'daily') return 'ui.pages.providersPage.accounts.quotaWindowDaily';
	if (window === 'credits') return 'ui.pages.providersPage.accounts.quotaWindowCredits';
	if (window === 'key') return 'ui.pages.providersPage.accounts.quotaWindowKey';
	return '';
}

/** Distinguishes an account balance or spend from a percent limit so health and chart surfaces
 *  never treat money as an empty window. */
export function isMoneyReading(window: QuotaWindow): boolean {
	return window.amount !== undefined && window.currency !== undefined;
}

/** Keeps the account a card shows valid when the list under it changes: an account that is gone
 *  falls back to the first one, and a connection with no accounts has none selected. */
export function selectedAccount(accounts: Account[], index: number): Account | null {
	if (accounts.length === 0) return null;
	if (index < 0 || index >= accounts.length) return accounts[0];
	return accounts[index];
}
