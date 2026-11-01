import { describe, expect, it } from 'vitest';

import {
	attemptChain,
	hasQuota,
	memberState,
	overflowMembers,
	orderMembers,
	resetMinutes,
	selectCurrentMember,
	selectNextMember,
	windowMinutes
} from './route-inspector';
import type { GroupMember, QuotaWindow, RouteCandidate, UsageAttemptRow } from './types';

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
		expect(current).toEqual({ index: 0, accountLabel: 'work' });
	});

	it('falls back to the first attempt when none served', () => {
		expect(selectCurrentMember(members, [attempt(1, 500, 'claude')])).toEqual({
			index: 1,
			accountLabel: ''
		});
	});

	it('takes the first preview candidate as the next member', () => {
		expect(
			selectNextMember(members, [{ provider_id: 'anthropic', model_id: 'claude', eligible: true }])
		).toBe(1);
		expect(selectNextMember(members, [])).toBe(-1);
	});

	it('rounds windows and resets up to whole minutes', () => {
		const window = { window_seconds: 18_060 } as QuotaWindow;
		expect(windowMinutes(window)).toBe(301);
		expect(resetMinutes(61_000, 0)).toBe(2);
	});

	it('marks a member limited when any account is limited', () => {
		const accounts = [
			{
				id: 'a',
				provider_id: 'openai',
				kind: 'api_key',
				label: 'a',
				status: 'active',
				priority: 1
			},
			{
				id: 'b',
				provider_id: 'openai',
				kind: 'api_key',
				label: 'b',
				status: 'active',
				priority: 1,
				limit_state: 'limited'
			}
		];
		expect(memberState(accounts, 1)).toBe('limited');
	});

	it('keeps no quota distinct from a measured zero', () => {
		expect(hasQuota([])).toBe(false);
		expect(hasQuota([{ used_percent: 0 } as QuotaWindow])).toBe(true);
	});

	it('renders a failover chain', () => {
		expect(attemptChain([{ ...attempt(1, 429, 'claude') }, { ...attempt(2, 200, 'gpt-5') }])).toBe(
			'#1 429 → #2 200'
		);
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
