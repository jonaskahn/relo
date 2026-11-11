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
export function windowsByCredential(windows: readonly QuotaWindow[]): Map<string, QuotaWindow[]> {
	return groupBy([...windows], (window) => window.credential_id);
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

/** Names the pool an Antigravity account reports: the Claude and Gemini pools, each with a
 *  session window and an optional weekly one. */
function antigravityPool(window: string): string {
	if (window === 'Cla') return 'ui.pages.providersPage.accounts.quotaWindowClaude';
	if (window === 'Gem') return 'ui.pages.providersPage.accounts.quotaWindowGemini';
	if (window === 'Cla (Weekly)') return 'ui.pages.providersPage.accounts.quotaWindowClaudeWeekly';
	if (window === 'Gem (Weekly)') return 'ui.pages.providersPage.accounts.quotaWindowGeminiWeekly';
	return '';
}

/** Names the window a provider publishes. An account usually reports a short burst window and a
 *  weekly one; anything else keeps the tag the provider used. */
export function quotaWindowLabelKey(window: string): string {
	const pool = antigravityPool(window);
	if (pool !== '') return pool;
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

/** Reports whether a window decides whether an account can serve: a limit on the account's own
 *  allowance. A pool inside that allowance does not, a reading of money does not, and a period
 *  that has already ended does not. */
export function windowGoverns(window: QuotaWindow): boolean {
	if (window.sub) return false;
	if (window.rolled) return false;
	return !isMoneyReading(window);
}

/** The window that decides whether an account can serve: the fullest one that governs it. */
export function governingWindow(windows: readonly QuotaWindow[]): QuotaWindow | null {
	let worst: QuotaWindow | null = null;
	for (const window of windows) {
		if (!windowGoverns(window)) continue;
		if (worst === null || window.used_percent > worst.used_percent) worst = window;
	}
	return worst;
}

/** Reports whether an account is spent: a window that governs it reached its end. */
export function accountSpent(windows: readonly QuotaWindow[]): boolean {
	const worst = governingWindow(windows);
	return worst !== null && worst.used_percent >= 100;
}

/** What a connection's quota says about it: which accounts are spent, and how much room the
 *  connection still has to serve with. */
export interface ConnectionQuota {
	/** The window that governs each spent account, so a caller can name the limit it reached. */
	spentWindows: readonly QuotaWindow[];
	/** The share used by the account with the most room left, which is the one that serves next.
	 *  Zero when no account reports a limit. */
	servingPercent: number;
	/** The window that governs the account with the most room. */
	servingWindow: QuotaWindow | null;
	/** True when every account of the connection is spent, so the connection cannot serve. */
	atLimit: boolean;
}

/** Reads the state of a connection's quota from the accounts it holds and the windows they
 *  reported. A connection serves as long as one account can, so it is spent only when all of
 *  them are, and an account that has not reported a limit still counts as able to serve. */
export function connectionQuota(
	accounts: readonly Account[],
	windows: readonly QuotaWindow[]
): ConnectionQuota {
	const byCredential = windowsByCredential(windows);
	const spentWindows: QuotaWindow[] = [];
	let servingPercent = 0;
	let servingWindow: QuotaWindow | null = null;
	let servingFound = false;
	for (const account of accounts) {
		const own = byCredential.get(account.id) ?? [];
		const worst = governingWindow(own);
		if (worst !== null && worst.used_percent >= 100) {
			spentWindows.push(worst);
			continue;
		}
		const used = worst?.used_percent ?? 0;
		if (servingFound && used >= servingPercent) continue;
		servingFound = true;
		servingPercent = used;
		servingWindow = worst;
	}
	return {
		spentWindows,
		servingPercent,
		servingWindow,
		atLimit: accounts.length > 0 && spentWindows.length === accounts.length
	};
}

/** Keeps the account a card shows valid when the list under it changes: an account that is gone
 *  falls back to the first one, and a connection with no accounts has none selected. */
export function selectedAccount(accounts: Account[], index: number): Account | null {
	if (accounts.length === 0) return null;
	if (index < 0 || index >= accounts.length) return accounts[0];
	return accounts[index];
}
