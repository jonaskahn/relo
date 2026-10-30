import type { Provider } from '$lib/types';

/** The two connections that are served by a keyless free pool, which is what a
 *  Free chip on a card says. Everything else that costs nothing (local
 *  endpoints, custom keys) is not a "Free plan". */
const FREE_TEMPLATES = new Set(['kilo-free', 'opencode-free']);

/** The number of keyless free lanes, which the picker shows while they are hidden. */
export const FREE_TEMPLATE_COUNT = FREE_TEMPLATES.size;

/** The two keyless lanes that can exist only once per daemon. */
export function isSingleFreeConnection(templateID: string): boolean {
	return FREE_TEMPLATES.has(templateID);
}

/** Reports whether one connection is one of the keyless free lanes. */
export function isFreeConnection(provider: Pick<Provider, 'template_id'>): boolean {
	return FREE_TEMPLATES.has(provider.template_id);
}

/** A paid key or cloud connection that publishes at least one free model, which is
 *  what the Free available chip answers. Keyless free lanes, sign-in plans, local
 *  endpoints, and custom endpoints never wear it. */
export function hasFreeAvailability(
	provider: Pick<Provider, 'kind' | 'template_id' | 'auth'> & {
		counts?: Pick<Provider['counts'], 'free_models'>;
		free_models?: number;
	}
): boolean {
	if (isFreeConnection({ template_id: provider.template_id })) return false;
	if (provider.auth === 'none') return false;
	if (provider.kind !== 'key' && provider.kind !== 'cloud') return false;
	const free = provider.counts?.free_models ?? provider.free_models ?? 0;
	return free > 0;
}

/** The Free filter's eligibility rule: a keyless free lane, or a paid key or
 *  cloud connection that publishes at least one free model. */
export function isFreeAvailableProvider(
	provider: Pick<Provider, 'kind' | 'template_id' | 'auth' | 'counts'>
): boolean {
	return isFreeConnection(provider) || hasFreeAvailability(provider);
}
