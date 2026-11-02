import { describe, expect, it } from 'vitest';

import {
	hasQuota,
	memberState,
	overflowMembers,
	orderMembers,
	pickQuotaAccount,
	resolveLinkedRequest,
	routingLayout,
	selectCurrentMember,
	selectNextMember,
	statePillWidth,
	windowSpan
} from './route-inspector';
import type {
	Account,
	GroupMember,
	QuotaWindow,
	RouteCandidate,
	UsageAttemptRow,
	UsageLogRow
} from './types';

const members: GroupMember[] = [
	{ provider_id: 'openai', model_id: 'gpt-5', weight: 1, enabled: true, eligible: true },
	{ provider_id: 'anthropic', model_id: 'claude', weight: 1, enabled: true, eligible: true }
];

function attempt(ordinal: number, status: number, model: string): UsageAttemptRow {
	return {
		Ordinal: ordinal,
		Provider: ordinal === 1 ? 'anthropic' : 'openai',
		Model: model,
		CredentialLabel: ordinal === 1 ? 'stale' : 'work',
		CredentialID: 'cred-' + ordinal,
		Status: status,
		DurationMs: 10,
		ErrorCode: '',
		InputTokens: 0,
		OutputTokens: 0
	};
}

function account(id: string, extra: Partial<Account> = {}): Account {
	return {
		id,
		provider_id: 'openai',
		kind: 'api_key',
		label: id,
		status: 'active',
		priority: 1,
		...extra
	};
}

function win(credential: string, used: number, resetAt = 0): QuotaWindow {
	return {
		credential_id: credential,
		connection_id: 'openai',
		label: 'Week',
		window: '7d',
		window_seconds: 604_800,
		used_percent: used,
		reset_at_ms: resetAt,
		source: 'probe',
		observed_at_ms: 0,
		stale: false
	};
}

describe('routing inspector decisions', () => {
	it('orders members by the preview candidates', () => {
		const candidates: RouteCandidate[] = [
			{ provider_id: 'anthropic', model_id: 'claude', eligible: true },
			{ provider_id: 'openai', model_id: 'gpt-5', eligible: true }
		];
		expect(orderMembers(members, candidates).map((member) => member.provider_id)).toEqual([
			'anthropic',
			'openai'
		]);
	});

	it('lets the newest serving attempt win over an earlier failure', () => {
		const current = selectCurrentMember(members, [
			attempt(1, 429, 'claude'),
			attempt(2, 200, 'gpt-5')
		]);
		expect(current).toEqual({ index: 0, accountLabel: 'work', credentialId: 'cred-2' });
	});

	it('falls back to the first attempt when none served', () => {
		expect(selectCurrentMember(members, [attempt(1, 500, 'claude')])).toEqual({
			index: 1,
			accountLabel: '',
			credentialId: ''
		});
	});

	it('takes the first preview candidate as the next member', () => {
		expect(
			selectNextMember(members, [{ provider_id: 'anthropic', model_id: 'claude', eligible: true }])
		).toBe(1);
		expect(selectNextMember(members, [])).toBe(-1);
	});

	it('keeps a member ready while another account can still serve', () => {
		const accounts = [account('a'), account('b', { limit_state: 'limited' })];
		expect(memberState(accounts, 1)).toBe('ready');
		expect(memberState(accounts, 1, [win('a', 20), win('b', 100)])).toBe('ready');
	});

	it('marks a member limited only when every active account is limited', () => {
		const accounts = [account('a'), account('b', { limit_state: 'limited' })];
		expect(memberState([account('a', { limit_state: 'limited' }), accounts[1]], 1)).toBe('limited');
		expect(memberState(accounts, 1, [win('a', 100)])).toBe('limited');
	});

	it('shows the account with the most headroom, not the first one', () => {
		const accounts = [account('a'), account('b')];
		const picked = pickQuotaAccount(accounts, [win('a', 100), win('b', 20)], 1);
		expect(picked.label).toBe('b');
		expect(picked.windows.map((window) => window.credential_id)).toEqual(['b']);
	});

	it('shows the serving account even when another has more headroom', () => {
		const accounts = [account('a'), account('b')];
		expect(pickQuotaAccount(accounts, [win('a', 60), win('b', 5)], 1, 'a').label).toBe('a');
	});

	it('skips a limited account while a ready one reports quota', () => {
		const accounts = [account('a', { limit_state: 'limited' }), account('b')];
		expect(pickQuotaAccount(accounts, [win('a', 10), win('b', 90)], 1).label).toBe('b');
	});

	it('shows the soonest reset when every account is limited', () => {
		const accounts = [account('a'), account('b')];
		const picked = pickQuotaAccount(accounts, [win('a', 100, 9_000), win('b', 100, 5_000)], 1);
		expect(picked.label).toBe('b');
	});

	it('passes windows of no listed account through unlabeled', () => {
		const windows = [win('gone', 40)];
		expect(pickQuotaAccount([account('a')], windows, 1)).toEqual({ label: '', windows });
	});

	it('keeps no quota distinct from a measured zero', () => {
		expect(hasQuota([])).toBe(false);
		expect(hasQuota([{ used_percent: 0 } as QuotaWindow])).toBe(true);
	});

	it('reads a window length in hours and days, not raw minutes', () => {
		expect(windowSpan(43_200)).toBe('12h');
		expect(windowSpan(604_800)).toBe('7d');
		expect(windowSpan(18_000)).toBe('5h');
		expect(windowSpan(2_592_000)).toBe('30d');
		expect(windowSpan(3_900)).toBe('1h 5m');
		expect(windowSpan(0)).toBe('');
	});

	it('collapses everything beyond nine members', () => {
		expect(overflowMembers(Array.from({ length: 11 }, (_, index) => index))).toEqual({
			visible: [0, 1, 2, 3, 4, 5, 6, 7, 8],
			hidden: 2
		});
	});

	it('keeps disabled and empty previews distinct', () => {
		expect(selectNextMember([], [])).toBe(-1);
	});
});

describe('routing layout', () => {
	function inside(layout: ReturnType<typeof routingLayout>) {
		const boxes = [
			layout.request,
			layout.group,
			...layout.cards,
			...(layout.overflow ? [layout.overflow] : [])
		];
		for (const box of boxes) {
			expect(box.x).toBeGreaterThanOrEqual(0);
			expect(box.y).toBeGreaterThanOrEqual(0);
			expect(box.x + box.width).toBeLessThanOrEqual(layout.width);
			expect(box.y + box.height).toBeLessThanOrEqual(layout.height);
		}
	}

	it('fans members right of the group on a wide viewport', () => {
		const layout = routingLayout({ width: 1280, members: 4, hidden: 2 });
		expect(layout.kind).toBe('wide');
		expect(layout.cards).toHaveLength(4);
		expect(layout.overflow).not.toBeNull();
		expect(layout.branches).toHaveLength(4);
		expect(layout.branches[0].path).toContain(' C ');
		expect(layout.branches[0].path).toContain(` ${layout.cards[0].x} `);
		expect(
			layout.branches[0].motionPath.startsWith(`M ${layout.request.x + layout.request.width} `)
		).toBe(true);
		expect(layout.overflowBranch).toContain(' C ');
		inside(layout);
	});

	it('stacks members on a tablet and keeps the compact layout below the phone breakpoint', () => {
		const tablet = routingLayout({ width: 768, members: 5, hidden: 1 });
		const phone = routingLayout({ width: 390, members: 5, hidden: 1 });
		expect(tablet.kind).toBe('compact');
		expect(phone.kind).toBe('compact');
		expect(tablet.cards[0].x).toBeLessThan(tablet.request.x + 100);
		expect(tablet.branches[0].path).toContain(' C ');
		expect(tablet.branches[0].path).toContain(` ${tablet.cards[0].x} `);
		expect(
			tablet.branches[0].motionPath.startsWith(
				`M ${tablet.request.x + tablet.request.width} ${tablet.request.y + tablet.request.height / 2}`
			)
		).toBe(true);
		inside(tablet);
		inside(phone);
	});

	it('keeps a usable width for the smallest viewport', () => {
		const layout = routingLayout({ width: 100, members: 2, hidden: 0 });
		expect(layout.width).toBeGreaterThanOrEqual(240);
		inside(layout);
	});
});

describe('state pill width', () => {
	it('covers English, long German, and CJK labels with dot and padding', () => {
		expect(statePillWidth('Ready')).toBeGreaterThanOrEqual(5 + 4 + 16 + 5 * 6);
		expect(statePillWidth('Anmeldung erforderlich')).toBeGreaterThanOrEqual(
			5 + 4 + 16 + 'Anmeldung erforderlich'.length * 6
		);
		expect(statePillWidth('需要登录')).toBeGreaterThanOrEqual(5 + 4 + 16 + 4 * 9.5);
		expect(statePillWidth('')).toBeGreaterThanOrEqual(44);
	});
});

function logRow(patch: Partial<UsageLogRow> = {}): UsageLogRow {
	return {
		ID: 1,
		RequestID: 'req-1',
		TimestampMs: 0,
		Provider: 'openai',
		Model: 'gpt-5',
		RequestedModel: 'gpt-5',
		GroupID: null,
		CredentialLabel: '',
		Surface: '',
		Status: 200,
		DurationMs: 0,
		InputTokens: 0,
		OutputTokens: 0,
		CacheReadTokens: 0,
		CacheWriteTokens: 0,
		EstimatedCostMicros: null,
		RouteProvider: '',
		RouteReason: '',
		ClientKeyID: '',
		ClientKeyName: '',
		ClientApp: '',
		Origin: '',
		Attempts: 1,
		Retried: false,
		...patch
	};
}

describe('deep-linked requests', () => {
	it('prefers the stored request id and falls back to the numeric row id', () => {
		const rows = [logRow({ ID: 7, RequestID: 'stored-7' }), logRow({ ID: 8, RequestID: '' })];
		expect(resolveLinkedRequest(rows, 'stored-7')?.ID).toBe(7);
		expect(resolveLinkedRequest(rows, '8')?.ID).toBe(8);
		expect(resolveLinkedRequest(rows, 'missing')).toBeNull();
	});
});
