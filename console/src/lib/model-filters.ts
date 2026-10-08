import type { Model, ModelPrices, Prices } from '$lib/types';

// The filters the model list offers: a search, a category, a status choice,
// one label, and, on routers that list both, one paid/free choice. Each
// starts on “all”, so an untouched list shows everything rather than a
// filtered view an operator did not ask for.

/** The narrowing a model list applies: a search, one category, one status, one label, and — on
 *  routers that list both — one paid or free choice. */
export interface ModelFilters {
	search: string;
	// category is one capability at a time: reasoning, tools, or vision.
	// Empty means every model regardless of capability.
	category: '' | ModelCategoryOption;
	// status is everything, only what is on, or only what is off.
	status: ModelStatusFilter;
	// label is one trait at a time: unpriced, overridden, or unavailable.
	label: ModelLabelFilter;
	// pricing is paid or free, used only on routers that name both.
	pricing: ModelPricingFilter;
}

/** Narrows a model list by whether the model answers. */
export type ModelStatusFilter = 'all' | 'on' | 'off';
/** Narrows by what a model still needs from an operator. */
export type ModelLabelFilter = 'all' | 'unpriced' | 'overridden' | 'unavailable';
/** Narrows by whether a model charges. */
export type ModelPricingFilter = 'all' | 'paid' | 'free';

/** The fixed capability choices the model filter offers: every model, or only the ones with that
 *  capability switched on. */
export const MODEL_CATEGORY_OPTIONS = ['reasoning', 'tools', 'vision'] as const;
/** One entry of the category filter. */
export type ModelCategoryOption = (typeof MODEL_CATEGORY_OPTIONS)[number];

/** A filter set that narrows nothing. */
export const EMPTY_MODEL_FILTERS: ModelFilters = {
	search: '',
	category: '',
	status: 'all',
	label: 'all',
	pricing: 'all'
};

/** Reports whether a filter set is narrowing the list. */
export function hasActiveFilters(filters: ModelFilters): boolean {
	return (
		filters.search.trim() !== '' ||
		filters.category !== '' ||
		filters.status !== 'all' ||
		filters.label !== 'all' ||
		filters.pricing !== 'all'
	);
}

/** Applies every active filter together: status, label and pricing all narrow the list when they
 *  are not “all”. */
export function filterModels(models: Model[], filters: ModelFilters): Model[] {
	const needle = filters.search.trim().toLowerCase();
	return models.filter((model) => {
		if (needle !== '') {
			const haystack = (model.model_id + ' ' + (model.name ?? '')).toLowerCase();
			if (!haystack.includes(needle)) return false;
		}
		if (filters.category !== '' && model.capabilities?.[filters.category] !== true) return false;
		if (filters.label === 'unpriced' && isPriced(model.prices)) return false;
		if (filters.label === 'overridden' && !model.overridden) return false;
		if (filters.label === 'unavailable' && model.available !== false) return false;
		if (filters.status === 'on' && !model.enabled) return false;
		if (filters.status === 'off' && model.enabled) return false;
		if (filters.pricing === 'free' && !isFreeModel(model.model_id)) return false;
		if (filters.pricing === 'paid' && isFreeModel(model.model_id)) return false;
		return true;
	});
}

/** How many models each control covers, so a segment or a chip shows the size of what choosing
 *  it would show. */
export interface ModelCounts {
	all: number;
	on: number;
	off: number;
	unpriced: number;
	overridden: number;
	unavailable: number;
	paid: number;
	free: number;
}

/** Reads what the filter options should report. */
export function modelCounts(models: Model[]): ModelCounts {
	const counts: ModelCounts = {
		all: models.length,
		on: 0,
		off: 0,
		unpriced: 0,
		overridden: 0,
		unavailable: 0,
		paid: 0,
		free: 0
	};
	for (const model of models) {
		if (model.enabled) counts.on++;
		else counts.off++;
		if (!isPriced(model.prices)) counts.unpriced++;
		if (model.overridden) counts.overridden++;
		if (model.available === false) counts.unavailable++;
		if (isFreeModel(model.model_id)) counts.free++;
		else counts.paid++;
	}
	return counts;
}

// These routers list paid and free variants of the same model, so the
// connection pane offers a paid/free filter and bulk toggle. OpenCode Go
// is a different product and is not in the set.
const PAID_FREE_TEMPLATES = new Set([
	'opencode',
	'openrouter',
	'orcarouter',
	'orcarouter-oauth',
	'teamorouter'
]);

/** Reports whether a provider's catalogue prices its models. */
export function supportsPaidFree(templateId: string): boolean {
	return PAID_FREE_TEMPLATES.has(templateId);
}

// A free model names “free” as its own token in the id: :free, -free,
// /free, _free or a space, not a substring of freedom or carefree.
const FREE_TOKEN = /(?:^|[:/_ -])free(?:$|[:/_ -])/i;

/** Reports whether a model id is one the provider serves at no cost. */
export function isFreeModel(modelId: string): boolean {
	return FREE_TOKEN.test(modelId);
}

/** Reports whether any rate is known for a model.
 *  A model with no input and no output rate is unpriced even when a cache rate is set, because a
 *  request to it cannot be costed. */
export function isPriced(prices: ModelPrices): boolean {
	return known(prices.effective.input) || known(prices.effective.output);
}

function known(value: number | null | undefined): boolean {
	return value !== null && value !== undefined;
}

/** Lists the categories a set of models actually uses, sorted, so the picker never offers a
 *  filter that empties the list. */
export function categoriesOf(models: Model[]): string[] {
	const seen = new Set<string>();
	for (const model of models) {
		if (model.category) seen.add(model.category);
	}
	return [...seen].sort();
}

/** Names the layer a price came from, which every row and every price table cell shows so a rate
 *  is never read without its origin. */
export type PriceTag = 'you' | 'provider' | 'modelsdev' | 'none';

/** Reads a price as the one word a row shows. */
export function priceTag(prices: ModelPrices): PriceTag {
	const input = tagFor(prices.effective_source.input ?? '', 'input', prices);
	if (input !== 'none') return input;
	return tagFor(prices.effective_source.output ?? '', 'output', prices);
}

function tagFor(source: string, key: keyof Prices, prices: ModelPrices): PriceTag {
	const override = prices.override[key];
	const provider = prices.provider[key];
	const modelsdev = prices.modelsdev[key];
	switch (source) {
		case 'override':
			return 'you';
		case 'provider':
			return 'provider';
		case 'modelsdev':
			return 'modelsdev';
	}
	if (override !== null && override !== undefined) return 'you';
	if (provider !== null && provider !== undefined) return 'provider';
	if (modelsdev !== null && modelsdev !== undefined) return 'modelsdev';
	return 'none';
}

/** The copy key each price tag reads. */
export const PRICE_TAG_KEY: Record<PriceTag, string> = {
	you: 'ui.pages.providersPage.models.priceYou',
	provider: 'ui.pages.providersPage.models.priceProvider',
	modelsdev: 'ui.pages.providersPage.models.priceModelsDev',
	none: 'ui.pages.providersPage.models.tagUnpriced'
};

/** Lists the price columns a row shows, which is the input and output rates an operator
 *  compares. */
export function visiblePrices(prices: Prices): { input: number | null; output: number | null } {
	return { input: prices.input, output: prices.output };
}
