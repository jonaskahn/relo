// The Client keys page lists the keys an operator can still use above the
// ones that lapsed, and keeps revoked keys in their own group. The grouping
// and the status pill live here so both are decisions a test can hold.

import { formatCountdown } from '$lib/dashboard';

import type { AccessKey } from '$lib/types';

/** Narrows the key list by whether a key still works. */
export type KeyStatusFilter = 'all' | 'active' | 'inactive';

/** The key list split into the sections the page draws. */
export interface KeyGroups {
	active: AccessKey[];
	expired: AccessKey[];
	revoked: AccessKey[];
}

/** Splits the keys the page holds into the three groups its list draws.
 *  The daemon's own order is kept inside each group.
 *  The filter decides which groups the list shows, and the others stay empty. */
export function groupKeys(keys: AccessKey[], filter: KeyStatusFilter): KeyGroups {
	const active = keys.filter((key) => key.status === 'active');
	const expired = keys.filter((key) => key.status === 'expired');
	const revoked = keys.filter((key) => key.status !== 'active' && key.status !== 'expired');
	if (filter === 'active') return { active, expired: [], revoked: [] };
	if (filter === 'inactive') return { active: [], expired, revoked };
	return { active, expired, revoked };
}

/** Reports an expired operator key the page may remove. */
export function canDeleteExpired(key: AccessKey): boolean {
	return key.status === 'expired' && !key.owner;
}

/** The shape and tone a status pill wears, in the vocabulary the shared StatusBadge draws:
 *  settled, caution, stopped, or plain. */
export type PillKind = 'pass' | 'warn' | 'fail' | 'muted';

/** The status a key's pill wears. */
export interface KeyStatusPill {
	kind: PillKind;
	// labelKey is the copy key the pill reads. A key that lapses carries the
	// time left as its value.
	labelKey: string;
	timeLeft?: string;
}

/** Reads one key as the pill the card and the detail show.
 *  A lapsed key is warn and a revoked one danger; a live key with an expiry says when it lapses
 *  instead of only "Active", because that is the fact an operator decides on.
 *  A key that lapses within a day still reads ok: any urgency threshold would be invented here,
 *  so the pill states the time and lets the operator judge. */
export function keyStatusPill(key: AccessKey, now = Date.now()): KeyStatusPill {
	if (key.status === 'expired') return { kind: 'warn', labelKey: 'ui.status.badge.expired' };
	if (key.status !== 'active') return { kind: 'fail', labelKey: 'ui.status.badge.revoked' };
	const left = (key.expires_at_ms ?? 0) - now;
	if (left > 0) {
		return {
			kind: 'pass',
			labelKey: 'ui.pages.keysPage.expiresIn',
			timeLeft: formatCountdown(left)
		};
	}
	return { kind: 'pass', labelKey: 'ui.status.badge.active' };
}

/** The interpolation a status pill's copy key takes: only a key that lapses carries the time it
 *  has left. */
export function keyPillValues(pill: KeyStatusPill): Record<string, string> {
	return pill.timeLeft === undefined ? {} : { time: pill.timeLeft };
}
