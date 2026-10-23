import type { Provider } from '$lib/types';

/** The two connections that are served by a keyless free pool, which is what a
 *  Free chip on a card says. Everything else that costs nothing (local
 *  endpoints, custom keys) is not a "Free plan". */
const FREE_TEMPLATES = new Set(['kilo-free', 'opencode-free']);

/** Reports whether one connection is one of the keyless free lanes. */
export function isFreeConnection(provider: Pick<Provider, 'template_id'>): boolean {
	return FREE_TEMPLATES.has(provider.template_id);
}