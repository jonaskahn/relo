import { redirect } from '@sveltejs/kit';

/** Agents is what this page configures, so an old link to Integrations lands on the page that
 *  reads and writes them. */
export function load({ url }: { url: URL }) {
	throw redirect(308, '/agents' + url.search);
}
