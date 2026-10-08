import { describe, expect, it } from 'vitest';

import { emptyPrices, modelPrices, pricesOf } from '$lib/provider-fixtures';
import { headingPriceSource, tilePriceSource } from './model-sources';

describe('headingPriceSource', () => {
	it('names models.dev when the catalog supplies either rate', () => {
		const both = modelPrices(emptyPrices(), emptyPrices(), pricesOf({ input: 1, output: 2 }));
		const outputOnly = modelPrices(pricesOf({ input: 1 }), emptyPrices(), pricesOf({ output: 2 }));

		expect(headingPriceSource(both)).toBe('modelsdev');
		expect(headingPriceSource(outputOnly)).toBe('modelsdev');
	});

	it('names nothing when the rates come from elsewhere', () => {
		const provider = modelPrices(emptyPrices(), pricesOf({ input: 1, output: 2 }), emptyPrices());
		const mine = modelPrices(pricesOf({ input: 1 }), emptyPrices(), emptyPrices());

		expect(headingPriceSource(provider)).toBe('');
		expect(headingPriceSource(mine)).toBe('');
		expect(headingPriceSource(null)).toBe('');
	});
});

describe('tilePriceSource', () => {
	it('tags the rate with the layer that decided it', () => {
		expect(tilePriceSource('provider')).toBe('ui.pages.providersPage.models.priceProvider');
		expect(tilePriceSource('override')).toBe('ui.pages.providersPage.models.priceYou');
	});

	it('stays silent for models.dev, which the heading already names', () => {
		expect(tilePriceSource('modelsdev')).toBe('');
		expect(tilePriceSource(undefined)).toBe('');
		expect(tilePriceSource('')).toBe('');
	});
});
