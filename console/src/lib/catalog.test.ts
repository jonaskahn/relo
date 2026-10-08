import { describe, expect, test } from 'vitest';

import {
	eligibleMembers,
	exposedName,
	formatContext,
	formatPrice,
	groupMembersOf,
	groupProviderCount,
	groupSetupChips,
	modelPath,
	modelPrice,
	moveMember,
	providerIconLimit,
	providerLabel,
	providerModelCounts,
	visibleProviderModelCounts
} from './catalog';
import { modelOf, providerOf } from './provider-fixtures';
import type { Group, GroupMember } from './types';

function memberOf(patch: Partial<GroupMember> = {}): GroupMember {
	return {
		provider_id: 'openai',
		model_id: 'gpt-5',
		weight: 1,
		enabled: true,
		eligible: true,
		...patch
	};
}

function groupOf(patch: Partial<Group> = {}): Group {
	return {
		id: 'fast',
		label: 'Fast',
		strategy: 'priority',
		enabled: true,
		listed: true,
		switch_on_4xx: true,
		switch_on_5xx: true,
		shadows_model: false,
		members: [memberOf()],
		created_at_ms: 1,
		updated_at_ms: 1,
		...patch
	};
}

describe('modelPath', () => {
	test('keeps a slash inside a model id a separator on the wire', () => {
		expect(modelPath('openai', 'gpt-5')).toBe('/models/openai/gpt-5');
		expect(modelPath('openai', 'org/model')).toBe('/models/openai/org/model');
		expect(modelPath('my provider', 'a b')).toBe('/models/my%20provider/a%20b');
	});
});

describe('formatPrice', () => {
	test('renders dollars per million tokens, and a dash for an unknown rate', () => {
		expect(formatPrice(null)).toBe('—');
		expect(formatPrice(0)).toBe('$0');
		expect(formatPrice(1_000)).toBe('$0.0010');
		expect(formatPrice(250_000)).toBe('$0.250');
		expect(formatPrice(1_250_000)).toBe('$1.25');
	});
});

describe('formatContext', () => {
	test('renders a window size in the short form', () => {
		expect(formatContext(null)).toBe('—');
		expect(formatContext(0)).toBe('—');
		expect(formatContext(512)).toBe('512');
		expect(formatContext(128_000)).toBe('128K');
		expect(formatContext(1_000_000)).toBe('1M');
		expect(formatContext(1_500_000)).toBe('1.5M');
	});
});

describe('modelPrice', () => {
	test('states both directions as one reading, and nothing without a model', () => {
		expect(modelPrice(modelOf())).toBe('$1.25 / $10.00');
		// A member the console holds no model for states nothing rather than a
		// price it cannot stand by.
		expect(modelPrice(undefined)).toBe('');
	});
});

describe('providerLabel', () => {
	test('names the connection, and falls back to its identifier', () => {
		const providers = [providerOf({ id: 'openai', label: 'OpenAI' })];
		expect(providerLabel(providers, 'openai')).toBe('OpenAI');
		expect(providerLabel(providers, 'groq')).toBe('groq');
		expect(providerLabel([], 'openai')).toBe('openai');
	});
});

describe('moveMember', () => {
	test('swaps a member with its neighbour and stops at each end', () => {
		const members = [
			memberOf({ model_id: 'a' }),
			memberOf({ model_id: 'b' }),
			memberOf({ model_id: 'c' })
		];
		expect(moveMember(members, 0, 1).map((m) => m.model_id)).toEqual(['b', 'a', 'c']);
		expect(moveMember(members, 2, 1)).toBe(members);
		expect(moveMember(members, 0, -1)).toBe(members);
	});
});

describe('groups', () => {
	test('treats an absent member list as none', () => {
		expect(groupMembersOf(groupOf({ members: null }))).toEqual([]);
	});

	test('counts the members a request could use', () => {
		expect(
			eligibleMembers(
				groupOf({
					members: [
						memberOf(),
						memberOf({ provider_id: 'groq', eligible: false }),
						memberOf({ provider_id: 'xai', enabled: false })
					]
				})
			)
		).toBe(1);
	});

	test('counts named connections and ignores a bare model id', () => {
		expect(
			groupProviderCount(
				groupOf({
					members: [
						memberOf(),
						memberOf({ provider_id: 'groq', model_id: 'flash' }),
						memberOf({ provider_id: '', model_id: 'gpt-5', kind: 'auto' })
					]
				})
			)
		).toBe(2);
	});

	test('caps visible connection logos by card layout', () => {
		expect(providerIconLimit('vertical')).toBe(5);
		expect(providerIconLimit('horizontal')).toBe(7);
		const chips = Array.from({ length: 9 }, (_, index) => ({
			providerId: 'p' + index,
			count: 1
		}));
		expect(visibleProviderModelCounts(chips, 5)).toEqual({
			visible: chips.slice(0, 5),
			overflow: 4
		});
		expect(visibleProviderModelCounts(chips.slice(0, 3), 5)).toEqual({
			visible: chips.slice(0, 3),
			overflow: 0
		});
	});

	test('groups models by the connection they were added from', () => {
		expect(
			providerModelCounts([
				memberOf(),
				memberOf({ model_id: 'gpt-5-mini' }),
				memberOf({ provider_id: 'groq', model_id: 'flash' }),
				memberOf({ provider_id: '', model_id: 'gpt-5', kind: 'auto' }),
				memberOf({ provider_id: '', model_id: 'gpt-5-mini', kind: 'auto' })
			])
		).toEqual([
			{ providerId: 'openai', count: 2 },
			{ providerId: 'groq', count: 1 },
			{ providerId: '', count: 2 }
		]);
	});

	test('names the client-facing name the daemon publishes, and falls back to the namespaced id', () => {
		expect(exposedName(groupOf({ client_id: 'reloc-fast' }))).toBe('reloc-fast');
		expect(exposedName(groupOf())).toBe('reloc-fast');
	});

	test('shows the setup a group can promise under its name', () => {
		expect(groupSetupChips(groupOf())).toEqual([]);
		expect(
			groupSetupChips(
				groupOf({ supports_vision: true, supports_reasoning: true, context_window: 1_000_000 })
			).map((chip) => chip.label ?? chip.labelKey)
		).toEqual(['ui.pages.groupsPage.chipVision', '1M', 'ui.pages.groupsPage.chipReasoning']);
		expect(
			groupSetupChips(
				groupOf({ supports_vision: false, supports_reasoning: false, context_window: 800_000 })
			).map((chip) => [chip.label ?? chip.labelKey, chip.tooltipKey])
		).toEqual([
			['ui.pages.groupsPage.chipVisionOff', 'ui.pages.groupsPage.hintNoVision'],
			['800K', 'ui.pages.groupsPage.budgetHint'],
			['ui.pages.groupsPage.chipReasoningOff', 'ui.pages.groupsPage.hintNoReasoning']
		]);
		const mixed = groupSetupChips(
			groupOf({
				supports_vision: false,
				supports_reasoning: false,
				capability_warnings: ['vision_off', 'reasoning_off', 'tools_mixed']
			})
		);
		expect(mixed.map((chip) => chip.tooltipKey)).toEqual([
			'ui.pages.groupsPage.warnVision',
			'ui.pages.groupsPage.warnReasoning'
		]);
	});
});
