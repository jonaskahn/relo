import type { Provider } from '$lib/types';

/** Lists the connections whose upstream needs no credential. A keyless lane is
 *  free by construction: it accepts no account, so there is nothing to bill. */
const KEYLESS_TEMPLATES = new Set(['kilo-free', 'opencode-free']);

/** Reports whether one connection is served by a keyless free pool, which is
 *  what a Free chip on its card says. */
export function isFreeConnection(provider: Pick<Provider, 'template_id' | 'auth'>): boolean {
	return provider.auth === 'none' || KEYLESS_TEMPLATES.has(provider.template_id);
}
