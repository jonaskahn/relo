import { describe, expect, test } from 'vitest';

import {
	EMPTY_MODEL_FILTERS,
	categoriesOf,
	filterModels,
	hasActiveFilters,
	isFreeModel,
	isPriced,
	modelCounts,
	priceTag,
	supportsPaidFree,
	visiblePrices
} from './model-filters';
import { emptyPrices, modelOf, modelPrices, pricesOf } from './provider-fixtures';

describe('model filters', () => {
	test('reports whether any filter is narrowing the list', () => {
		expect(hasActiveFilters(EMPTY_MODEL_FILTERS)).toBe(false);
		// Whitespace alone is not a search, so a trimmed-away search does not
		// count as an active filter.
		expect(hasActiveFilters({ ...EMPTY_MODEL_FILTERS, search: '   ' })).toBe(false);
		expect(hasActiveFilters({ ...EMPTY_MODEL_FILTERS, search: 'gpt' })).toBe(true);
		expect(hasActiveFilters({ ...EMPTY_MODEL_FILTERS, category: 'tools' })).toBe(true);
		expect(hasActiveFilters({ ...EMPTY_MODEL_FILTERS, status: 'off' })).toBe(true);
		expect(hasActiveFilters({ ...EMPTY_MODEL_FILTERS, label: 'unpriced' })).toBe(true);
		expect(hasActiveFilters({ ...EMPTY_MODEL_FILTERS, pricing: 'free' })).toBe(true);
	});

	test('lists the price columns a row shows', () => {
		const priced = pricesOf({ input: 1_000_000, output: 2_000_000 });
		expect(visiblePrices(priced)).toEqual({ input: 1_000_000, output: 2_000_000 });
		// A model with one rate missing still shows the column, empty, rather
		// than hiding the comparison the operator came for.
		expect(visiblePrices({ ...priced, input: null })).toEqual({ input: null, output: 2_000_000 });
		expect(visiblePrices({ ...priced, output: null })).toEqual({ input: 1_000_000, output: null });
		expect(visiblePrices(emptyPrices())).toEqual({ input: null, output: null });
	});

	test('an untouched set of filters keeps every model', () => {
		const models = [modelOf(), modelOf({ model_id: 'gpt-5-mini', enabled: false })];
		expect(filterModels(models, EMPTY_MODEL_FILTERS)).toHaveLength(2);
	});

	test('a search covers the id and the name', () => {
		const models = [
			modelOf({ model_id: 'gpt-5', name: 'GPT-5' }),
			modelOf({ model_id: 'o3', name: 'o3' })
		];
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, search: 'gpt' }).map((m) => m.model_id)
		).toEqual(['gpt-5']);
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, search: 'O3' }).map((m) => m.model_id)
		).toEqual(['o3']);
	});

	test('a category narrows to models with that capability', () => {
		const models = [
			modelOf({ model_id: 'a', capabilities: { tools: null, reasoning: true, vision: null } }),
			modelOf({ model_id: 'b', capabilities: { tools: true, reasoning: null, vision: null } })
		];
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, category: 'tools' }).map((m) => m.model_id)
		).toEqual(['b']);
	});

	test('a label and status both narrow the list', () => {
		const models = [
			modelOf({ model_id: 'priced-on', enabled: true }),
			modelOf({
				model_id: 'unpriced-on',
				enabled: true,
				prices: modelPrices(emptyPrices(), emptyPrices(), emptyPrices())
			}),
			modelOf({
				model_id: 'unpriced-off',
				enabled: false,
				prices: modelPrices(emptyPrices(), emptyPrices(), emptyPrices())
			})
		];
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, label: 'unpriced' }).map((m) => m.model_id)
		).toEqual(['unpriced-on', 'unpriced-off']);
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, label: 'unpriced', status: 'off' }).map(
				(m) => m.model_id
			)
		).toEqual(['unpriced-off']);
	});

	test('the status segment keeps the models on the side it names', () => {
		const models = [modelOf({ model_id: 'on' }), modelOf({ model_id: 'off', enabled: false })];
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, status: 'off' }).map((m) => m.model_id)
		).toEqual(['off']);
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, status: 'on' }).map((m) => m.model_id)
		).toEqual(['on']);
		expect(filterModels(models, { ...EMPTY_MODEL_FILTERS, status: 'all' })).toHaveLength(2);
	});

	test('the counts size every control', () => {
		const models = [
			modelOf({ model_id: 'on' }),
			modelOf({ model_id: 'off', enabled: false }),
			modelOf({
				model_id: 'unpriced',
				prices: modelPrices(emptyPrices(), emptyPrices(), emptyPrices()),
				overridden: true,
				available: false
			})
		];
		expect(modelCounts(models)).toEqual({
			all: 3,
			on: 2,
			off: 1,
			unpriced: 1,
			overridden: 1,
			unavailable: 1,
			paid: 3,
			free: 0
		});
	});

	test('isFreeModel treats free as its own token in the id', () => {
		expect(isFreeModel('x-ai/grok:free')).toBe(true);
		expect(isFreeModel('big-pickle-free')).toBe(true);
		expect(isFreeModel('a/b free')).toBe(true);
		expect(isFreeModel('FREE')).toBe(true);
		expect(isFreeModel('freedom')).toBe(false);
		expect(isFreeModel('carefree')).toBe(false);
	});

	test('the pricing filter keeps paid or free models', () => {
		const models = [
			modelOf({ model_id: 'x-ai/grok:free', enabled: true }),
			modelOf({ model_id: 'x-ai/grok', enabled: true }),
			modelOf({ model_id: 'big-pickle-free', enabled: false })
		];
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, pricing: 'free' }).map((m) => m.model_id)
		).toEqual(['x-ai/grok:free', 'big-pickle-free']);
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, pricing: 'paid' }).map((m) => m.model_id)
		).toEqual(['x-ai/grok']);
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, pricing: 'free', status: 'off' }).map(
				(m) => m.model_id
			)
		).toEqual(['big-pickle-free']);
		expect(modelCounts(models)).toMatchObject({ paid: 1, free: 2 });
	});

	test('supportsPaidFree names the routers that list both', () => {
		expect(supportsPaidFree('opencode')).toBe(true);
		expect(supportsPaidFree('openrouter')).toBe(true);
		expect(supportsPaidFree('orcarouter')).toBe(true);
		expect(supportsPaidFree('orcarouter-oauth')).toBe(true);
		expect(supportsPaidFree('teamorouter')).toBe(true);
		expect(supportsPaidFree('opencode-go')).toBe(false);
		expect(supportsPaidFree('anthropic')).toBe(false);
	});

	test('the unavailable chip keeps the models a listing dropped', () => {
		const models = [modelOf({ model_id: 'here' }), modelOf({ model_id: 'gone', available: false })];
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, label: 'unavailable' }).map((m) => m.model_id)
		).toEqual(['gone']);
	});

	test('the overridden label reads the daemon flag', () => {
		const models = [
			modelOf({ model_id: 'plain' }),
			modelOf({ model_id: 'edited', overridden: true })
		];
		expect(
			filterModels(models, { ...EMPTY_MODEL_FILTERS, label: 'overridden' }).map((m) => m.model_id)
		).toEqual(['edited']);
	});

	test('lists only the categories the models use', () => {
		expect(
			categoriesOf([
				modelOf({ category: 'chat' }),
				modelOf({ category: 'vision' }),
				modelOf({ category: 'chat' })
			])
		).toEqual(['chat', 'vision']);
	});

	test('a model needs an input or an output rate to count as priced', () => {
		expect(isPriced(modelPrices(emptyPrices(), pricesOf({ input: 1 }), emptyPrices()))).toBe(true);
		expect(isPriced(modelPrices(emptyPrices(), emptyPrices(), pricesOf({ output: 1 })))).toBe(true);
		expect(isPriced(modelPrices(emptyPrices(), emptyPrices(), pricesOf({ cache_read: 1 })))).toBe(
			false
		);
	});

	test('the price tag follows the input rate, then the output rate', () => {
		expect(priceTag(modelPrices(emptyPrices(), emptyPrices(), pricesOf({ input: 1 })))).toBe(
			'modelsdev'
		);
		expect(priceTag(modelPrices(emptyPrices(), pricesOf({ input: 1 }), emptyPrices()))).toBe(
			'provider'
		);
		expect(
			priceTag(modelPrices(pricesOf({ input: 1 }), pricesOf({ input: 2 }), emptyPrices()))
		).toBe('you');
		expect(priceTag(modelPrices(emptyPrices(), emptyPrices(), pricesOf({ output: 1 })))).toBe(
			'modelsdev'
		);
		expect(priceTag(modelPrices(emptyPrices(), emptyPrices(), emptyPrices()))).toBe('none');
	});
});
