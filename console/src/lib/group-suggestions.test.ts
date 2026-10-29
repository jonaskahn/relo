import { describe, expect, it } from 'vitest';

import { modelOf } from './provider-fixtures';
import type { GroupMember, Provider } from './types';
import {
	allowedByLanes,
	familyKey,
	groupSuggestions,
	laneCounts,
	modelsInLanes,
	similarModels
} from './group-suggestions';

function provider(
	id: string,
	templateId: string,
	kind: 'signin' | 'key' | 'cloud' | 'local' = 'key'
): Provider {
	return {
		id,
		template_id: templateId,
		label: id,
		kind,
		available_formats: null,
		variable_defs: null,
		origin: 'template',
		login_flows: [],
		auth: 'api_key',
		api_format: 'openai-chat',
		api_formats: [],
		key_header: 'Authorization',
		models_source: 'listing',
		models_format: 'openai',
		modelsdev_provider_id: '',
		base_url: 'https://example.test',
		doc_url: '',
		key_env: [],
		headers: {},
		variables: {},
		needs_setup: [],
		routable: true,
		unroutable_reason: '',
		serving_accounts: 1,
		active_accounts: 1,
		enabled: true,
		use_proxy: false,
		timeout_seconds: null,
		retry_attempts: null,
		switch_on_4xx: true,
		switch_on_5xx: true,
		rank: 0,
		pool_strategy: '',
		configured: true,
		counts: {
			models: 1,
			enabled_models: 1,
			available_models: 1,
			unpriced_models: 0,
			free_models: 0,
			accounts: 1,
			active_accounts: 1,
			paused_accounts: 0,
			reauth_accounts: 0
		},
		created_at_ms: 0,
		updated_at_ms: 0
	};
}

function member(overrides: Partial<GroupMember> = {}): GroupMember {
	return {
		provider_id: 'openai',
		model_id: 'gpt-5',
		weight: 1,
		enabled: true,
		eligible: true,
		...overrides
	};
}

describe('familyKey', () => {
	it('prefers the models.dev family', () => {
		expect(
			familyKey(modelOf({ family: 'claude-sonnet', model_id: 'claude-sonnet-5@default' }))
		).toBe('claude-sonnet');
	});
	it('strips versions, dates and vendor prefix without a family', () => {
		expect(
			familyKey(modelOf({ family: '', model_id: 'anthropic/claude-sonnet-4-6@20250805' }))
		).toBe('claude-sonnet');
		expect(familyKey(modelOf({ family: '', model_id: 'claude-sonnet-5' }))).toBe('claude-sonnet');
	});
	it('keeps tier words so mini and pro do not collapse together', () => {
		expect(familyKey(modelOf({ family: '', model_id: 'gpt-5-mini' }))).toBe('gpt-mini');
		expect(familyKey(modelOf({ family: '', model_id: 'gpt-5' }))).toBe('gpt');
	});
});

describe('allowedByLanes', () => {
	it('allows everything for an empty lane set', () => {
		const p = provider('p', 'opencode-free');
		expect(allowedByLanes(p, [])).toBe(true);
	});
	it('allows everything when manual is chosen', () => {
		const p = provider('p', 'opencode-free');
		expect(allowedByLanes(p, ['manual'])).toBe(true);
	});
	it('filters by lane otherwise', () => {
		const free = provider('free', 'opencode-free');
		const payg = provider('payg', 'openai', 'key');
		expect(allowedByLanes(free, ['free'])).toBe(true);
		expect(allowedByLanes(payg, ['free'])).toBe(false);
		expect(allowedByLanes(payg, ['payg', 'free'])).toBe(true);
	});
});

describe('laneCounts', () => {
	it('counts connections by their billing lane', () => {
		const providers = [
			provider('a', 'opencode-free'),
			provider('b', 'claude', 'signin'),
			provider('c', 'openai', 'key'),
			provider('d', 'ollama', 'local')
		];
		const counts = laneCounts(providers);
		expect(counts.free).toBe(1);
		expect(counts.plan).toBe(1);
		expect(counts.payg).toBe(1);
		expect(counts.manual).toBe(1);
	});

	it('adds up to the connections it was given', () => {
		const providers = [
			provider('a', 'opencode-free'),
			provider('b', 'claude', 'signin'),
			provider('c', 'openai', 'key'),
			provider('d', 'ollama', 'local'),
			provider('e', 'lm-studio', 'local')
		];
		const counts = laneCounts(providers);
		expect(counts.free + counts.plan + counts.payg + counts.manual).toBe(providers.length);
	});
});

describe('modelsInLanes', () => {
	it('narrows the catalog to the chosen lanes', () => {
		const free = provider('free', 'opencode-free');
		const payg = provider('payg', 'openai', 'key');
		const models = [
			modelOf({ provider_id: 'free', model_id: 'm' }),
			modelOf({ provider_id: 'payg', model_id: 'm' })
		];
		const narrowed = modelsInLanes(models, [free, payg], ['free']);
		expect(narrowed).toHaveLength(1);
		expect(narrowed[0].provider_id).toBe('free');
	});
});

describe('similarModels', () => {
	it('finds same-family models on other connections, newest first', () => {
		const seed = modelOf({
			provider_id: 'plan',
			model_id: 'claude-sonnet-5',
			family: 'claude-sonnet',
			release_date: '2025-01-01'
		});
		const others = [
			modelOf({
				provider_id: 'free',
				model_id: 'claude-sonnet-4-6',
				family: 'claude-sonnet',
				release_date: '2025-02-02',
				routable: true,
				serving_accounts: 1
			}),
			modelOf({
				provider_id: 'payg',
				model_id: 'claude-sonnet-5-5',
				family: 'claude-sonnet',
				release_date: '2025-03-03',
				routable: true,
				serving_accounts: 1
			})
		];
		const providers = [
			provider('plan', 'claude', 'signin'),
			provider('free', 'opencode-free'),
			provider('payg', 'openai', 'key')
		];
		const result = similarModels(others, seed, providers, ['free', 'payg', 'plan']);
		expect(result.models.map((model) => model.model_id)).toEqual([
			'claude-sonnet-5-5',
			'claude-sonnet-4-6'
		]);
	});
	it('respects the chosen lanes', () => {
		const seed = modelOf({
			provider_id: 'plan',
			model_id: 'claude-sonnet-5',
			family: 'claude-sonnet'
		});
		const others = [
			modelOf({
				provider_id: 'free',
				model_id: 'claude-sonnet-4-6',
				family: 'claude-sonnet',
				routable: true,
				serving_accounts: 1
			})
		];
		const providers = [provider('plan', 'claude', 'signin'), provider('free', 'opencode-free')];
		expect(similarModels(others, seed, providers, ['payg']).models).toHaveLength(0);
	});
	it('skips disabled, unroutable and already-member models', () => {
		const seed = modelOf({
			provider_id: 'plan',
			model_id: 'claude-sonnet-5',
			family: 'claude-sonnet'
		});
		const others = [
			modelOf({
				provider_id: 'free',
				model_id: 'claude-sonnet-4-6',
				family: 'claude-sonnet',
				enabled: false,
				routable: true,
				serving_accounts: 1
			}),
			modelOf({
				provider_id: 'payg',
				model_id: 'claude-sonnet-5-5',
				family: 'claude-sonnet',
				routable: false,
				serving_accounts: 1
			})
		];
		const providers = [
			provider('plan', 'claude', 'signin'),
			provider('free', 'opencode-free'),
			provider('payg', 'openai', 'key')
		];
		const members: GroupMember[] = [member({ provider_id: 'payg', model_id: 'claude-sonnet-5-5' })];
		expect(
			similarModels(others, seed, providers, ['free', 'payg', 'plan'], members).models
		).toHaveLength(0);
	});
	it('does not offer a model that would conflict on tools', () => {
		const seed = modelOf({
			provider_id: 'plan',
			model_id: 'claude-sonnet-5',
			family: 'claude-sonnet',
			capabilities: { tools: true, reasoning: null, vision: null }
		});
		const others = [
			modelOf({
				provider_id: 'free',
				model_id: 'claude-sonnet-4-6',
				family: 'claude-sonnet',
				capabilities: { tools: false, reasoning: null, vision: null },
				routable: true,
				serving_accounts: 1
			})
		];
		const providers = [provider('plan', 'claude', 'signin'), provider('free', 'opencode-free')];
		expect(similarModels(others, seed, providers, ['free', 'plan'])).toMatchObject({
			models: [],
			conflicts: 1
		});
	});
});

describe('groupSuggestions', () => {
	it('groups suggestions by family', () => {
		const models = [
			modelOf({ family: 'claude-sonnet', model_id: 'a' }),
			modelOf({ family: 'claude-opus', model_id: 'b' }),
			modelOf({ family: 'claude-sonnet', model_id: 'c' })
		];
		const groups = groupSuggestions(models);
		expect(groups).toHaveLength(2);
		expect(groups.find((group) => group.family === 'claude-sonnet')?.models).toHaveLength(2);
	});
});
