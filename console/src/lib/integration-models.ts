import type { IntegrationModel } from './types';

// The Agents picker's filter rules, kept pure so the page and the tests read
// one implementation. A model the data plane lists is namespaced: a
// connection's model is relo-<provider>-<model> and a route is reloc-<route>,
// with the million-token marker some clients read appended. The listing also
// reports the connection and the provider's own identifier beside each entry,
// so nothing here decodes a public name that carries hyphens of its own.

/** The suffix a listed model id carries when it serves a million-token window. */
export const CONTEXT_SUFFIX = '[1m]';

// ROUTE_PREFIX marks a published route, which names Relo rather than a
// connection.
const ROUTE_PREFIX = 'reloc-';

/** Drops the marker a Claude Code client reads as a million-token window, so one model has one
 *  identity whatever spelling listed it. */
export function bareModelID(id: string): string {
	return id.endsWith(CONTEXT_SUFFIX) ? id.slice(0, -CONTEXT_SUFFIX.length) : id;
}

/** Names the connection a listed model came from, read from the metadata the data plane sends
 *  rather than decoded from the public name.
 *  A route names Relo, which owns the choice of where to send a request. */
export function providerOf(model: IntegrationModel): string {
	if (model.provider_id) return model.provider_id;
	return bareModelID(model.id).startsWith(ROUTE_PREFIX) ? 'relo' : '';
}

/** Names the model within its namespace, which is the spelling a provider's own roster uses. */
export function modelNameOf(model: IntegrationModel): string {
	if (model.source_model_id) return model.source_model_id;
	return bareModelID(model.id);
}

/** Narrows an agent's model list: a free-text query, a provider, and the models one account is
 *  known to serve. */
export interface ModelFilter {
	query: string;
	provider: string;
	// accountModels is the chosen account's own roster. An empty list means
	// the account's roster is unknown, which filters nothing rather than
	// hiding every model.
	accountModels?: string[] | null;
}

function matchesAccount(model: IntegrationModel, accountModelID: string): boolean {
	if (accountModelID === '') return false;
	const name = modelNameOf(model);
	return name === accountModelID || name.endsWith('/' + accountModelID);
}

/** Applies a filter to an agent's model list. */
export function filterModels(models: IntegrationModel[], filter: ModelFilter): IntegrationModel[] {
	const query = filter.query.trim().toLowerCase();
	const roster = filter.accountModels ?? null;
	return models.filter((model) => {
		if (filter.provider && providerOf(model) !== filter.provider) return false;
		if (roster && roster.length > 0 && !roster.some((id) => matchesAccount(model, id)))
			return false;
		if (query) {
			const haystack = (model.id + ' ' + (model.name ?? '')).toLowerCase();
			if (!haystack.includes(query)) return false;
		}
		return true;
	});
}

/** Lists the providers a model list names, in order, so the picker offers only what is actually
 *  there. */
export function providersOf(models: IntegrationModel[]): string[] {
	const seen = new Set<string>();
	for (const model of models) {
		const provider = providerOf(model);
		if (provider) seen.add(provider);
	}
	return [...seen].sort();
}

/** Renders a model's advertised window, or an empty string when the listing stated none. */
export function contextLabelOf(model: IntegrationModel | null | undefined): string {
	const value = model?.context_window;
	if (value === null || value === undefined || !Number.isFinite(value) || value <= 0) return '';
	if (value >= 1_000_000 && value % 1_000 === 0) return value / 1_000_000 + 'M';
	if (value >= 1_000 && value % 1_000 === 0) return value / 1_000 + 'K';
	return value.toLocaleString();
}
