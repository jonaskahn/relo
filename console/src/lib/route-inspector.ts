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
