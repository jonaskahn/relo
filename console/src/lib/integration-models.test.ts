import { describe, expect, it } from 'vitest';

import {
	bareModelID,
	contextLabelOf,
	filterModels,
	modelNameOf,
	providerOf,
	providersOf
} from './integration-models';
import type { IntegrationModel } from './types';

const models: IntegrationModel[] = [
	{
		id: 'relo-openai-gpt-5.5',
		name: 'OpenAI/GPT-5.5',
		context_window: 400_000,
		provider_id: 'openai',
		source_model_id: 'gpt-5.5'
	},
	{
		id: 'relo-openai-gpt-5.5[1m]',
		name: 'OpenAI/GPT-5.5',
		context_window: 1_000_000,
		provider_id: 'openai',
		source_model_id: 'gpt-5.5'
	},
	{
		id: 'relo-claude-claude-opus',
		name: 'Claude/Opus',
		context_window: 200_000,
		provider_id: 'claude',
		source_model_id: 'claude-opus'
	},
	{ id: 'reloc-smart', name: 'Relo/Smart route' }
];

describe('model identity', () => {
	it('names the connection a listed model came from, and Relo for a route', () => {
		expect(providerOf(models[0])).toBe('openai');
		expect(providerOf(models[1])).toBe('openai');
		expect(providerOf(models[3])).toBe('relo');
	});

	it('names the model within its namespace', () => {
		expect(modelNameOf(models[0])).toBe('gpt-5.5');
		expect(modelNameOf(models[3])).toBe('reloc-smart');
	});

	it('drops the million-token marker', () => {
		expect(bareModelID('relo-openai-gpt-5.5[1m]')).toBe('relo-openai-gpt-5.5');
	});
});

describe('providersOf', () => {
	it('lists what the list names, once each', () => {
		expect(providersOf(models)).toEqual(['claude', 'openai', 'relo']);
	});
});

describe('filterModels', () => {
	it('filters by provider', () => {
		const openai = filterModels(models, { query: '', provider: 'openai' });
		expect(openai.map((model) => model.id)).toEqual([
			'relo-openai-gpt-5.5',
			'relo-openai-gpt-5.5[1m]'
		]);
	});

	it('filters by free text over the id and the display name', () => {
		expect(filterModels(models, { query: 'opus', provider: '' }).map((m) => m.id)).toEqual([
			'relo-claude-claude-opus'
		]);
		expect(filterModels(models, { query: 'reloc-', provider: '' })).toHaveLength(1);
	});

	it('filters by the account roster, matching the marker spelling too', () => {
		const held = filterModels(models, { query: '', provider: '', accountModels: ['gpt-5.5'] });
		expect(held).toHaveLength(2);
	});

	it('leaves an unknown roster unfiltered', () => {
		expect(filterModels(models, { query: '', provider: '', accountModels: [] })).toHaveLength(4);
	});
});

describe('contextLabelOf', () => {
	it('writes a round window with its unit', () => {
		expect(contextLabelOf(models[0])).toBe('400K');
		expect(contextLabelOf(models[1])).toBe('1M');
		expect(contextLabelOf(null)).toBe('');
	});
});
