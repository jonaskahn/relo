import type { ModelPrices } from '$lib/types';

/** Names the layer a resolved value came from, which every detail and every price in the console
 *  is tagged with. */
export const SOURCE_LABEL_KEY: Record<string, string> = {
	override: 'ui.pages.providersPage.models.priceYou',
	provider: 'ui.pages.providersPage.models.priceProvider',
	modelsdev: 'ui.pages.providersPage.models.priceModelsDev'
};

// The models.dev catalog prices a whole family at once, so a model the
// provider did not price itself names that one source for both rates. The
// pricing heading says it once instead of printing it under every number.

/** Names the layer the pricing heading carries as a chip, which is models.dev when either rate
 *  resolves from the catalog. */
export function headingPriceSource(prices: ModelPrices | null | undefined): string {
	const source = prices?.effective_source ?? {};
	if (source.input === 'modelsdev' || source.output === 'modelsdev') return 'modelsdev';
	return '';
}

/** Names the layer a single rate is tagged with beside the number. models.dev is silent here
 *  because the heading already says it. */
export function tilePriceSource(source: string | undefined): string {
	if (!source || source === 'modelsdev') return '';
	return SOURCE_LABEL_KEY[source] ?? '';
}
