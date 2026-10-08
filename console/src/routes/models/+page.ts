import { redirect } from '@sveltejs/kit';

import { modelsRedirectTarget } from '$lib/provider-routes';

/** Models live inside the provider that serves them, so an old link to the Models page lands
 *  where the model list moved to. */
export function load({ url }: { url: URL }) {
	throw redirect(308, modelsRedirectTarget(url.search));
}
