import { redirect } from '@sveltejs/kit';

/** A connection is what this page configures, so an old link to the Providers page lands on the
 *  page that reads and writes them. */
export function load({ url }: { url: URL }) {
	throw redirect(308, '/connections' + url.search);
}
