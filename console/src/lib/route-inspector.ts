// Read-only routing-inspector decisions: ordering, serving state, quota readings, and the
// compact labels the recent-route table renders.

import type { Account, GroupMember, QuotaWindow, RouteCandidate, UsageAttemptRow } from './types';
import { formatCountdown } from './dashboard';

export type MemberState =
	'ready' | 'limited' | 'paused' | 'needsSignIn' | 'unavailable' | 'cooling';

export interface MemberView {
	member: GroupMember;
	providerLabel: string;
	modelName: string;
	accounts: readonly Account[];
	windows: readonly QuotaWindow[];
}

export interface CurrentMember {
	index: number;
	accountLabel: string;
}

/** Renders a stored window length against the largest unit that keeps it short. */
export function windowSpan(seconds: number): string {
	if (!Number.isFinite(seconds) || seconds <= 0) return '';
	return formatCountdown(seconds * 1000);
}

/** Orders members by the preview's candidate sequence, then keeps any member the preview did not
 *  name in its stored order. */
export function orderMembers(
	members: readonly GroupMember[],
	candidates: readonly RouteCandidate[]
): GroupMember[] {
	const remaining = [...members];
	const ordered: GroupMember[] = [];
	for (const candidate of candidates) {
		const at = remaining.findIndex(
			(member) =>
				member.provider_id === candidate.provider_id && member.model_id === candidate.model_id
		);
		if (at >= 0) ordered.push(...remaining.splice(at, 1));
	}
	return [...ordered, ...remaining];
}

/** Selects the member that actually served the newest request: the last attempt below 400 wins
 *  over earlier failures, falling back to the first attempt when none served. */
export function selectCurrentMember(
	members: readonly GroupMember[],
	attempts: readonly UsageAttemptRow[]
): CurrentMember {
	const serving = [...attempts].reverse().find((attempt) => attempt.Status < 400);
	const chosen = serving ?? attempts[0];
	if (!chosen) return { index: -1, accountLabel: '' };
	let index = members.findIndex(
		(member) => member.provider_id === chosen.Provider && member.model_id === chosen.Model
	);
	if (index < 0) {
		const providers = members.filter((member) => member.provider_id === chosen.Provider);
		if (providers.length === 1) index = members.indexOf(providers[0]);
	}
	return {
		index,
		accountLabel: serving?.CredentialLabel ?? ''
	};
}

/** Names the first preview candidate that belongs to the group, which is where the next request
 *  starts. */
export function selectNextMember(
	members: readonly GroupMember[],
	candidates: readonly RouteCandidate[]
): number {
	const first = candidates[0];
	if (!first) return -1;
	return members.findIndex(
		(member) => member.provider_id === first.provider_id && member.model_id === first.model_id
	);
}

/** Aggregates every account behind one member into the state the card shows. */
export function memberState(
	accounts: readonly Account[],
	nowMs: number,
	windows: readonly QuotaWindow[] = []
): MemberState {
	if (accounts.some((account) => account.status === 'needs_reauth')) return 'needsSignIn';
	if (
		accounts.some(
			(account) =>
				account.limit_state === 'limited' ||
				(account.limited_until_ms ?? 0) > nowMs ||
				windows.some((window) => window.used_percent >= 100)
		)
	) {
		return 'limited';
	}
	if (accounts.length === 0 || accounts.every((account) => account.status !== 'active')) {
		return accounts.some((account) => account.status === 'paused') ? 'paused' : 'unavailable';
	}
	if (accounts.some((account) => account.status === 'paused')) return 'paused';
	if (accounts.some((account) => account.limit_state === 'probing')) return 'cooling';
	return 'ready';
}

/** Renders one request's attempts as the compact chain the recent-route table shows. */
export function attemptChain(attempts: readonly UsageAttemptRow[]): string {
	return attempts.map((attempt) => `#${attempt.Ordinal} ${attempt.Status}`).join(' → ');
}

/** Keeps the first nine rows and reports what the summary row stands for. */
export function overflowMembers<T>(
	rows: readonly T[],
	limit = 9
): { visible: T[]; hidden: number } {
	return { visible: rows.slice(0, limit), hidden: Math.max(0, rows.length - limit) };
}

/** Counts the accounts that can serve over every active account behind a member. */
export function memberCoverage(member: GroupMember): { serving: number; active: number } {
	return {
		serving: member.serving_accounts ?? 0,
		active: member.active_accounts ?? 0
	};
}

/** Reports the two reading states an inspector must never confuse: no quota was reported, versus
 *  a measured zero-percent window. */
export function hasQuota(windows: readonly QuotaWindow[]): boolean {
	return windows.length > 0;
}
