// Read-only routing-inspector decisions: ordering, serving state, quota readings, and the
// compact labels the recent-route table renders.

import type {
	Account,
	GroupMember,
	QuotaWindow,
	RouteCandidate,
	UsageAttemptRow,
	UsageLogRow
} from './types';
import { accountSpent, governingWindow } from './provider-quota';

export type MemberState =
	'ready' | 'limited' | 'paused' | 'needsSignIn' | 'unavailable' | 'cooling';

export interface MemberView {
	member: GroupMember;
	providerLabel: string;
	modelName: string;
	accounts: readonly Account[];
	windows: readonly QuotaWindow[];
	/** The account the windows belong to, empty when they are not one account's. */
	quotaAccount?: string;
	/** Every window the member's accounts reported, which decides its state. */
	reported?: readonly QuotaWindow[];
}

export interface CurrentMember {
	index: number;
	accountLabel: string;
	credentialId?: string;
}

/** One box in the routing diagram, in viewBox coordinates. */
export interface RoutingRect {
	readonly x: number;
	readonly y: number;
	readonly width: number;
	readonly height: number;
}

/** One group-to-member connector, named by the member it reaches. */
export interface RoutingBranch {
	readonly index: number;
	readonly path: string;
	readonly motionPath: string;
}

/** The diagram geometry for the width the inspector currently has.
 *  `wide` keeps the two capsules on the left and fans right; `compact` puts
 *  them on top and runs a curved spine down the cards. */
export interface RoutingLayout {
	readonly kind: 'wide' | 'compact';
	readonly width: number;
	readonly height: number;
	readonly request: RoutingRect;
	readonly group: RoutingRect;
	readonly requestConnector: string;
	readonly cards: readonly RoutingRect[];
	readonly overflow: RoutingRect | null;
	readonly branches: readonly RoutingBranch[];
	readonly overflowBranch: string | null;
}

export interface RoutingLayoutInput {
	readonly width: number;
	readonly members: number;
	readonly hidden?: number;
}

const WIDE_MIN_WIDTH = 1024;
const MIN_LAYOUT_WIDTH = 240;

function glyphIsWide(glyph: string): boolean {
	const code = glyph.codePointAt(0) ?? 0;
	return (
		code > 0xffff ||
		(code >= 0x1100 && code <= 0x115f) ||
		(code >= 0x2e80 && code <= 0x303e) ||
		(code >= 0x3040 && code <= 0x30ff) ||
		(code >= 0x3100 && code <= 0x31ff) ||
		(code >= 0x3400 && code <= 0x4dbf) ||
		(code >= 0x4e00 && code <= 0x9fff) ||
		(code >= 0xac00 && code <= 0xd7af) ||
		(code >= 0xf900 && code <= 0xfaff) ||
		(code >= 0xfe30 && code <= 0xfe4f) ||
		(code >= 0xff00 && code <= 0xff60) ||
		(code >= 0xffe0 && code <= 0xffe6)
	);
}

/** The width one state chip needs, including its dot, gap, and horizontal padding. */
export function statePillWidth(label: string): number {
	let textWidth = 0;
	for (const glyph of label) textWidth += glyphIsWide(glyph) ? 9.5 : 6;
	const dot = 5;
	const gap = 4;
	const padding = 16;
	return Math.max(44, Math.ceil(textWidth + dot + gap + padding));
}

function cardBox(x: number, y: number, width: number, height: number): RoutingRect {
	return { x, y, width, height };
}

function wideLayout(width: number, members: number, hidden: number): RoutingLayout {
	const pad = 20;
	const top = 16;
	const nodeWidth = 128;
	const nodeHeight = 64;
	const requestX = pad;
	const groupX = requestX + nodeWidth + 28;
	const groupRight = groupX + nodeWidth;
	const cardX = groupRight + 64;
	const cardWidth = width - cardX - pad;
	const cardHeight = 96;
	const rowHeight = 108;
	const overflowHeight = 52;
	const cards = Array.from({ length: members }, (_, index) =>
		cardBox(cardX, top + index * rowHeight, cardWidth, cardHeight)
	);
	const lastCard = cards[cards.length - 1];
	const overflowY = lastCard ? lastCard.y + lastCard.height + 14 : top;
	const overflow = hidden > 0 ? cardBox(cardX, overflowY, cardWidth, overflowHeight) : null;
	const height = overflow
		? overflow.y + overflow.height + pad
		: (lastCard?.y ?? top) + cardHeight + pad;
	const centerY = cards.length
		? (cards[0].y + cardHeight / 2 + (lastCard?.y ?? cards[0].y) + cardHeight / 2) / 2
		: top + nodeHeight / 2;
	const nodeY = centerY - nodeHeight / 2;
	const motionStartX = requestX + nodeWidth;
	const branches = cards.map((card, index) => ({
		index,
		path: `M ${groupRight} ${centerY} C ${groupRight + 44} ${centerY}, ${cardX - 60} ${card.y + cardHeight / 2}, ${cardX} ${card.y + cardHeight / 2}`,
		motionPath: `M ${motionStartX} ${centerY} H ${groupRight} C ${groupRight + 44} ${centerY}, ${cardX - 60} ${card.y + cardHeight / 2}, ${cardX} ${card.y + cardHeight / 2}`
	}));
	const overflowBranch = overflow
		? `M ${groupRight} ${centerY} C ${groupRight + 44} ${centerY}, ${cardX - 60} ${overflow.y + overflow.height / 2}, ${cardX} ${overflow.y + overflow.height / 2}`
		: null;
	return {
		kind: 'wide',
		width,
		height,
		request: cardBox(requestX, nodeY, nodeWidth, nodeHeight),
		group: cardBox(groupX, nodeY, nodeWidth, nodeHeight),
		requestConnector: `M ${requestX + nodeWidth} ${centerY} H ${groupX}`,
		cards,
		overflow,
		branches,
		overflowBranch
	};
}

function compactLayout(width: number, members: number, hidden: number): RoutingLayout {
	const pad = 16;
	const top = 14;
	const gap = 24;
	const nodeWidth = (width - pad * 2 - gap) / 2;
	const nodeHeight = 64;
	const requestX = pad;
	const groupX = requestX + nodeWidth + gap;
	const cardX = pad + 34;
	const cardWidth = width - cardX - pad;
	const cardHeight = 158;
	const rowHeight = 172;
	const overflowHeight = 52;
	const cardTop = top + nodeHeight + 44;
	const cards = Array.from({ length: members }, (_, index) =>
		cardBox(cardX, cardTop + index * rowHeight, cardWidth, cardHeight)
	);
	const lastCard = cards[cards.length - 1];
	const overflowY = lastCard ? lastCard.y + lastCard.height + 14 : cardTop;
	const overflow = hidden > 0 ? cardBox(cardX, overflowY, cardWidth, overflowHeight) : null;
	const height = overflow
		? overflow.y + overflow.height + pad
		: (lastCard?.y ?? cardTop) + cardHeight + pad;
	const startX = groupX + nodeWidth / 2;
	const startY = top + nodeHeight;
	const motionStartX = requestX + nodeWidth;
	const motionStartY = top + nodeHeight / 2;
	const branches = cards.map((card, index) => ({
		index,
		path: `M ${startX} ${startY} C ${startX} ${card.y + cardHeight / 2 - 16}, ${cardX - 42} ${card.y + cardHeight / 2}, ${cardX} ${card.y + cardHeight / 2}`,
		motionPath: `M ${motionStartX} ${motionStartY} H ${startX} V ${startY} C ${startX} ${card.y + cardHeight / 2 - 16}, ${cardX - 42} ${card.y + cardHeight / 2}, ${cardX} ${card.y + cardHeight / 2}`
	}));
	const overflowBranch = overflow
		? `M ${startX} ${startY} C ${startX} ${overflow.y + overflow.height / 2 - 16}, ${cardX - 42} ${overflow.y + overflow.height / 2}, ${cardX} ${overflow.y + overflow.height / 2}`
		: null;
	return {
		kind: 'compact',
		width,
		height,
		request: cardBox(requestX, top, nodeWidth, nodeHeight),
		group: cardBox(groupX, top, nodeWidth, nodeHeight),
		requestConnector: `M ${requestX + nodeWidth} ${top + nodeHeight / 2} H ${groupX}`,
		cards,
		overflow,
		branches,
		overflowBranch
	};
}

/** Builds viewport-aware routing geometry without touching the DOM, so the component can redraw
 *  on resize and a test can assert every box stays inside the viewBox. */
export function routingLayout({ width, members, hidden = 0 }: RoutingLayoutInput): RoutingLayout {
	const cleanWidth = Number.isFinite(width)
		? Math.max(MIN_LAYOUT_WIDTH, Math.round(width))
		: WIDE_MIN_WIDTH;
	const count = Math.max(0, Math.floor(members));
	const overflow = Math.max(0, Math.floor(hidden));
	return cleanWidth >= WIDE_MIN_WIDTH
		? wideLayout(cleanWidth, count, overflow)
		: compactLayout(cleanWidth, count, overflow);
}

/** Renders a stored window length against the largest unit that keeps it short. */
export function windowSpan(seconds: number): string {
	if (!Number.isFinite(seconds) || seconds <= 0) return '';
	const totalMinutes = Math.ceil(seconds / 60);
	const days = Math.floor(totalMinutes / 1440);
	const hours = Math.floor((totalMinutes % 1440) / 60);
	const minutes = totalMinutes % 60;
	if (days > 0) return hours > 0 ? days + 'd ' + hours + 'h' : days + 'd';
	if (hours > 0) return minutes > 0 ? hours + 'h ' + minutes + 'm' : hours + 'h';
	return minutes + 'm';
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
		accountLabel: serving?.CredentialLabel ?? '',
		credentialId: serving?.CredentialID ?? ''
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

function accountLimited(account: Account, nowMs: number, windows: readonly QuotaWindow[]): boolean {
	return (
		account.limit_state === 'limited' ||
		(account.limited_until_ms ?? 0) > nowMs ||
		accountSpent(windows)
	);
}

function windowsOf(account: Account, windows: readonly QuotaWindow[]): QuotaWindow[] {
	return windows.filter((window) => window.credential_id === account.id);
}

/** Aggregates every account behind one member into the state the card shows. A member is limited
 *  only when every active account is, since any other account can still take the request. */
export function memberState(
	accounts: readonly Account[],
	nowMs: number,
	windows: readonly QuotaWindow[] = []
): MemberState {
	if (accounts.some((account) => account.status === 'needs_reauth')) return 'needsSignIn';
	const active = accounts.filter((account) => account.status === 'active');
	if (
		active.length > 0 &&
		active.every((account) => accountLimited(account, nowMs, windowsOf(account, windows)))
	) {
		return 'limited';
	}
	if (accounts.length === 0 || active.length === 0) {
		return accounts.some((account) => account.status === 'paused') ? 'paused' : 'unavailable';
	}
	if (accounts.some((account) => account.status === 'paused')) return 'paused';
	if (accounts.some((account) => account.limit_state === 'probing')) return 'cooling';
	return 'ready';
}

/** Picks the one account whose quota a member card shows. The account that served the newest
 *  request wins; otherwise the account with the most headroom, so a limited account never hides
 *  one that can still serve. When every account is limited the soonest reset wins. Windows that
 *  belong to no listed account pass through unlabeled. */
export function pickQuotaAccount(
	accounts: readonly Account[],
	windows: readonly QuotaWindow[],
	nowMs: number,
	servingCredentialId = ''
): { label: string; windows: readonly QuotaWindow[] } {
	const candidates = accounts
		.map((account) => ({ account, own: windowsOf(account, windows) }))
		.filter((candidate) => candidate.own.length > 0);
	if (candidates.length === 0) return { label: '', windows };
	const headroom = (own: readonly QuotaWindow[]) => {
		const worst = governingWindow(own);
		return worst === null ? 100 : 100 - worst.used_percent;
	};
	const resetAt = (own: readonly QuotaWindow[]) => {
		const times = own.map((window) => window.reset_at_ms).filter((time) => time > 0);
		return times.length > 0 ? Math.min(...times) : Number.MAX_SAFE_INTEGER;
	};
	const serving = candidates.find((candidate) => candidate.account.id === servingCredentialId);
	const open = candidates.filter(
		(candidate) =>
			candidate.account.status === 'active' &&
			!accountLimited(candidate.account, nowMs, candidate.own)
	);
	const best = serving
		? serving
		: open.length > 0
			? open.reduce((a, b) => {
					const gap = headroom(b.own) - headroom(a.own);
					if (gap !== 0) return gap > 0 ? b : a;
					return b.account.priority < a.account.priority ? b : a;
				})
			: candidates.reduce((a, b) => (resetAt(b.own) < resetAt(a.own) ? b : a));
	return { label: best.account.label, windows: best.own };
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

/** Counts the accounts that can serve now over every active account behind a member. An account
 *  that is limited or out of quota is active but not serving, so four accounts with two limited
 *  read 2/4. The daemon's own count still caps it, since it knows which accounts serve the model. */
export function memberCoverage(
	member: GroupMember,
	accounts: readonly Account[] = [],
	nowMs = 0,
	windows: readonly QuotaWindow[] = []
): { serving: number; active: number } {
	const active = accounts.filter((account) => account.status === 'active');
	if (active.length === 0) {
		return { serving: member.serving_accounts ?? 0, active: member.active_accounts ?? 0 };
	}
	const ready = active.filter(
		(account) => !accountLimited(account, nowMs, windowsOf(account, windows))
	);
	return {
		serving: Math.min(member.serving_accounts ?? ready.length, ready.length),
		active: active.length
	};
}

/** Reports the two reading states an inspector must never confuse: no quota was reported, versus
 *  a measured zero-percent window. */
export function hasQuota(windows: readonly QuotaWindow[]): boolean {
	return windows.length > 0;
}

/** Finds the request a `?request=` deep link names, accepting either the stored request id or the
 *  numeric row id. */
export function resolveLinkedRequest(
	rows: readonly UsageLogRow[],
	wanted: string
): UsageLogRow | null {
	const needle = wanted.trim();
	if (needle === '') return null;
	return (
		rows.find((row) => row.RequestID === needle) ??
		rows.find((row) => String(row.ID) === needle) ??
		null
	);
}
