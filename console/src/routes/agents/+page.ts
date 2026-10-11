import { redirect } from '@sveltejs/kit';

/** Agent Harness is what this page is called, so an old link to Agents lands on
 *  the page that reads and writes them. */
export function load({ url }: { url: URL }) {
	throw redirect(308, '/agent-harness' + url.search);
}
