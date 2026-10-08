import { describe, expect, test } from 'vitest';

import { effectiveDetailValue, rowDetails } from './model-detail';
import { modelOf } from './provider-fixtures';
import type { ModelDetails } from './types';

describe('row details', () => {
	test('carries every value a row states', () => {
		const details = rowDetails(
			modelOf({
				name: 'GPT-5',
				description: 'A frontier model',
				family: 'gpt',
				category: 'chat',
				context_window: 400000,
				max_input: 272000,
				max_output: 128000,
				capabilities: { tools: true, reasoning: false, vision: null },
				status: 'active',
				release_date: '2025-08-07'
			})
		);

		expect(details.provider).toEqual({
			name: 'GPT-5',
			description: 'A frontier model',
			family: 'gpt',
			category: 'chat',
			context_window: 400000,
			max_input: 272000,
			max_output: 128000,
			tools: true,
			reasoning: false,
			vision: null,
			status: 'active',
			release_date: '2025-08-07'
		});
	});

	test('a value the row does not state is null rather than blank', () => {
		const details = rowDetails(
			modelOf({
				description: '',
				release_date: '   ',
				context_window: null,
				max_input: null,
				capabilities: { tools: null, reasoning: null, vision: null }
			})
		);

		expect(details.provider.description).toBeNull();
		expect(details.provider.release_date).toBeNull();
		expect(details.provider.context_window).toBeNull();
		expect(details.provider.max_input).toBeNull();
		expect(details.provider.tools).toBeNull();
	});

	test('the layers a row cannot carry stay silent', () => {
		const details = rowDetails(modelOf());

		expect(details.override).toEqual(details.modelsdev);
		expect(Object.values(details.override).every((value) => value === null)).toBe(true);
		// No source badges: the row states the resolved value, never which
		// layer it came from.
		expect(details.effective_source).toEqual({});
	});
});

describe('effective model details', () => {
	test('honors override values, including false and zero', () => {
		const model = modelOf({
			context_window: 100000,
			capabilities: { tools: true, reasoning: true, vision: null }
		});
		const base = rowDetails(model);
		const details: ModelDetails = {
			...base,
			override: { ...base.override, context_window: 0, tools: false },
			provider: { ...base.provider, context_window: 100000, tools: true },
			effective_source: { context_window: 'override', tools: 'override' }
		};
		expect(effectiveDetailValue(model, details, 'context_window')).toBe(0);
		expect(effectiveDetailValue(model, details, 'tools')).toBe(false);
	});

	test('falls back to the resolved row while detail layers load', () => {
		const model = modelOf({ name: 'Resolved name' });
		expect(effectiveDetailValue(model, null, 'name')).toBe('Resolved name');
	});

	test('uses provider facts before models.dev and uses models.dev when provider is silent', () => {
		const model = modelOf();
		const base = rowDetails(model);
		const details: ModelDetails = {
			...base,
			provider: { ...base.provider, category: 'reasoning' },
			modelsdev: { ...base.modelsdev, category: 'chat', max_output: 16384 },
			effective_source: { category: 'provider', max_output: 'modelsdev' }
		};
		expect(effectiveDetailValue(model, details, 'category')).toBe('reasoning');
		expect(effectiveDetailValue(model, details, 'max_output')).toBe(16384);
	});
});
