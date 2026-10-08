import { api } from '$lib/api';
import { initI18n } from '$lib/i18n';

/** The console ships as a single-page application: every route renders in the browser, and the
 *  Go server serves the shell for all of them. */
export const ssr = false;
/** prerender stays off: the daemon serves the shell for every route. */
export const prerender = false;

/** The language arrives with the shell the daemon served, so the console renders in it from the
 *  first paint instead of flashing English. */
export async function load(): Promise<void> {
	await initI18n(window.__reloLanguage ?? (await daemonLanguage()));
}

async function daemonLanguage(): Promise<string | undefined> {
	try {
		return (await api<{ language?: string }>('/status')).language;
	} catch {
		return undefined;
	}
}
