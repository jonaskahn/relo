import { describe, expect, it } from 'vitest';

import {
	autoMember,
	displayMembers,
	eligibleMemberCount,
	emptyRouteDraft,
	memberKind,
	memberStanding,
	resolutionReason,
	routeDraftFrom,
	routeSaveBody,
	routeSnapshot,
	validateBasics,
	validateMembers,
	weightShare
} from './route-stepper';
import { emptyPrices, modelOf, modelPrices, pricesOf } from './provider-fixtures';
import type { Group, GroupMember } from './types';

function member(overrides: Partial<GroupMember> = {}): GroupMember {
	return {
		provider_id: 'openai',
		model_id: 'gpt-4o',
		weight: 1,
		enabled: true,
		eligible: true,
		...overrides
	};
}

describe('auto members', () => {
	it('stores a bare model identifier without a connection', () => {
		const draft = {
			...emptyRouteDraft(),
			id: 'smart',
			members: [autoMember('gpt-5.1'), member({ provider_id: 'claude', model_id: 'sonnet' })]
		};
		const body = routeSaveBody(draft) as { members: GroupMember[] };
		expect(body.members[0]).toMatchObject({ provider_id: '', model_id: 'gpt-5.1', kind: 'auto' });
		expect(body.members[1]).toMatchObject({ provider_id: 'claude', kind: 'model' });
	});

	it('reads a member stored before kinds existed as one connection’s model', () => {
		expect(memberKind(member())).toBe('model');
		expect(memberKind(autoMember('gpt-5.1'))).toBe('auto');
	});

	it('keeps an auto member when a stored route is opened again', () => {
		const group: Group = {
			id: 'smart',
			label: 'Smart',
			strategy: 'priority',
			enabled: true,
			listed: true,
			switch_on_4xx: true,
			switch_on_5xx: true,
			shadows_model: false,
			members: [autoMember('gpt-5.1')],
			created_at_ms: 0,
			updated_at_ms: 0
		};
		const draft = routeDraftFrom(group);
		expect(draft.members[0].kind).toBe('auto');
		expect(draft.members[0].provider_id).toBe('');
		const body = routeSaveBody(draft) as { members: GroupMember[] };
		expect(body.members[0]).toMatchObject({ kind: 'auto', model_id: 'gpt-5.1' });
	});
});

describe('route validation', () => {
	it('needs a name before the first step is done', () => {
		expect(validateBasics(emptyRouteDraft()).id).toBe('ui.pages.groupsPage.needId');
		expect(Object.keys(validateBasics({ ...emptyRouteDraft(), id: 'fast' }))).toHaveLength(0);
		expect(validateBasics({ ...emptyRouteDraft(), id: '   ' }).id).toBeTruthy();
	});

	it('needs a member before the route can resolve', () => {
		const draft = { ...emptyRouteDraft(), id: 'fast' };
		expect(validateMembers(draft).members).toBe('ui.pages.groupsPage.needMembers');
		expect(Object.keys(validateMembers({ ...draft, members: [member()] }))).toHaveLength(0);
	});
});

describe('route save body', () => {
	it('stores the trimmed name and the members in order', () => {
		const draft = {
			...emptyRouteDraft(),
			id: 'fast',
			label: '  Fast  ',
			strategy: 'weighted',
			members: [member({ weight: 4 }), member({ model_id: 'gpt-4o-mini', enabled: false })]
		};
		expect(routeSaveBody(draft)).toEqual({
			label: 'Fast',
			strategy: 'weighted',
			enabled: true,
			listed: true,
			switch_on_4xx: true,
			switch_on_5xx: true,
			members: [
				{ provider_id: 'openai', model_id: 'gpt-4o', kind: 'model', weight: 4, enabled: true },
				{ provider_id: 'openai', model_id: 'gpt-4o-mini', kind: 'model', weight: 1, enabled: false }
			]
		});
	});

	it('leaves eligibility out of what a save would store', () => {
		const draft = { ...emptyRouteDraft(), id: 'fast', members: [member()] };
		const changed = { ...draft, members: [member({ eligible: false, reason: 'no key' })] };
		expect(routeSnapshot(changed)).toEqual(routeSnapshot(draft));
	});
});

describe('route draft from a stored route', () => {
	it('keeps the members in the stored order', () => {
		const group: Group = {
			id: 'fast',
			label: 'Fast',
			strategy: 'priority',
			enabled: false,
			listed: false,
			switch_on_4xx: false,
			switch_on_5xx: true,
			shadows_model: false,
			members: [member({ model_id: 'b' }), member({ model_id: 'a' })],
			created_at_ms: 1,
			updated_at_ms: 1
		};
		const draft = routeDraftFrom(group);
		expect(draft.id).toBe('fast');
		expect(draft.enabled).toBe(false);
		expect(draft.members.map((entry) => entry.model_id)).toEqual(['b', 'a']);
		expect(routeSnapshot(draft)).toEqual(routeSnapshot(draft));
	});

	// The failover switches are the route's own choice, so a draft reads them
	// from the stored route, sends them back on save, and counts them as a
	// change a close would throw away.
	it('keeps the failover choice, saves it back, and counts it as a change', () => {
		const group: Group = {
			id: 'fast',
			label: 'Fast',
			strategy: 'priority',
			enabled: true,
			listed: true,
			switch_on_4xx: false,
			switch_on_5xx: true,
			shadows_model: false,
			members: [member()],
			created_at_ms: 1,
			updated_at_ms: 1
		};
		const draft = routeDraftFrom(group);
		expect(draft.switchOn4xx).toBe(false);
		expect(draft.switchOn5xx).toBe(true);
		expect(routeSaveBody(draft)).toMatchObject({ switch_on_4xx: false, switch_on_5xx: true });
		expect(routeSnapshot(draft)).toMatchObject({ switchOn4xx: false, switchOn5xx: true });
		expect(routeSnapshot({ ...draft, switchOn5xx: false })).not.toEqual(routeSnapshot(draft));
	});

	it('starts a new route with both failover switches on', () => {
		expect(emptyRouteDraft()).toMatchObject({ switchOn4xx: true, switchOn5xx: true });
	});
});

describe('member standing', () => {
	it('reads serving, paused and ineligible in that order', () => {
		expect(memberStanding(member())).toBe('serving');
		expect(memberStanding(member({ enabled: false }))).toBe('paused');
		expect(memberStanding(member({ eligible: false, reason: 'no key' }))).toBe('ineligible');
	});

	it('counts only enabled members that can serve', () => {
		expect(
			eligibleMemberCount([member(), member({ enabled: false }), member({ eligible: false })])
		).toBe(1);
	});
});

describe('weighted shares', () => {
	it('gives each enabled member its part of the total weight', () => {
		const members = [member({ weight: 3 }), member({ weight: 1 })];
		expect(weightShare(members[0], members)).toBe(75);
		expect(weightShare(members[1], members)).toBe(25);
	});

	it('gives a paused member no share and the enabled ones the whole total', () => {
		const members = [member({ weight: 1 }), member({ weight: 1, enabled: false })];
		expect(weightShare(members[0], members)).toBe(100);
		expect(weightShare(members[1], members)).toBe(0);
	});

	it('gives no share when every weight is zero', () => {
		const members = [member({ weight: 0 }), member({ weight: 0 })];
		expect(weightShare(members[0], members)).toBe(0);
	});
});

describe('cheapest display order', () => {
	it('sorts by the combined catalog rate, unknown rates last, without touching the draft', () => {
		const draft = {
			...emptyRouteDraft(),
			strategy: 'cheapest',
			members: [
				member({ model_id: 'b' }),
				member({ model_id: 'noprice' }),
				member({ model_id: 'a' }),
				member({ model_id: 'unknown' })
			]
		};
		const meta = {
			'openai/a': modelOf({
				model_id: 'a',
				prices: modelPrices(pricesOf({ input: 1_000_000 }), emptyPrices(), emptyPrices())
			}),
			'openai/b': modelOf({
				model_id: 'b',
				prices: modelPrices(pricesOf({ input: 5_000_000 }), emptyPrices(), emptyPrices())
			}),
			'openai/noprice': modelOf({
				model_id: 'noprice',
				prices: modelPrices(emptyPrices(), emptyPrices(), emptyPrices())
			})
		};
		expect(displayMembers(draft, meta).map((entry) => entry.model_id)).toEqual([
			'a',
			'b',
			'noprice',
			'unknown'
		]);
		expect(draft.members.map((entry) => entry.model_id)).toEqual(['b', 'noprice', 'a', 'unknown']);
	});

	it('keeps the stored order for every other ordering', () => {
		const draft = {
			...emptyRouteDraft(),
			strategy: 'priority',
			members: [member({ model_id: 'b' }), member({ model_id: 'a' })]
		};
		expect(displayMembers(draft, {}).map((entry) => entry.model_id)).toEqual(['b', 'a']);
	});
});

describe('resolution reasons', () => {
	it('reads first, then the fallback wording for the ordered strategies', () => {
		expect(resolutionReason('priority', 0)).toBe('ui.pages.groupsPage.when.first');
		expect(resolutionReason('cheapest', 1)).toBe('ui.pages.groupsPage.when.afterUnavailable');
		expect(resolutionReason('fastest', 2)).toBe('ui.pages.groupsPage.when.afterUnavailable');
	});

	it('reads the loop wording for round-robin', () => {
		expect(resolutionReason('round-robin', 0)).toBe('ui.pages.groupsPage.when.roundRobin');
		expect(resolutionReason('round-robin', 3)).toBe('ui.pages.groupsPage.when.roundRobin');
	});
});
