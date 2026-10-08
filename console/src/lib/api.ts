import { get } from 'svelte/store';
import { locale, t } from 'svelte-i18n';

const base = '/api/v1';

// A request that never answers would hold one of the six browser connections
// the console shares with its live streams, so every request that does not
// bring its own signal gives up on its own.
const requestTimeoutMs = 15_000;

const writeMethods = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);
/** The window event a write fires when what agents see has changed. */
export const AGENT_CATALOG_CHANGED_EVENT = 'relo:agent-catalog-changed';

/** These writes can change the model names or capabilities Relo publishes to coding agents.
 *  Successful writes notify the shell so its warning does not wait for the operator to navigate. */
export function mutationAffectsAgentCatalog(path: string, method: string): boolean {
	if (!writeMethods.has(method.toUpperCase())) return false;
	return (
		path === '/catalog/refresh' ||
		// A new model directory or proxy changes what agents see at the next
		// refresh, so the shell re-reads the catalog warning.
		path === '/settings/providers' ||
		path.startsWith('/routes/') ||
		path.startsWith('/models/') ||
		path.startsWith('/connections/')
	);
}

function notifyAgentCatalogChanged(path: string, method: string) {
	if (typeof window !== 'undefined' && mutationAffectsAgentCatalog(path, method)) {
		window.dispatchEvent(new Event(AGENT_CATALOG_CHANGED_EVENT));
	}
}

/** A management request that answered with a failure the Relo error contract names. */
export class ApiError extends Error {
	status: number;
	code: string;

	constructor(status: number, code: string, message: string) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.code = code;
	}
}

/** Returns the CSRF token a cookie header carries.
 *  The console reads it from the readable cookie and echoes it on every write, which is what the
 *  server's double-submit check compares. */
export function csrfFromCookie(cookie: string): string {
	const match = cookie.match(/(?:^|;\s*)relo_csrf=([^;]*)/);
	return match ? decodeURIComponent(match[1]) : '';
}

/** Reads the double-submit token the cookie holds. */
export function csrfToken(): string {
	return csrfFromCookie(document.cookie);
}

/** The language the console renders in, which the daemon answers its own messages in: prose and
 *  page agree. */
export function acceptLanguage(): string {
	return get(locale) ?? 'en';
}

function requestMethod(init: RequestInit): string {
	return (init.method ?? 'GET').toUpperCase();
}

function requestHeaders(init: RequestInit, method: string): Headers {
	const headers = new Headers(init.headers);
	if (init.body != null && !headers.has('Content-Type')) {
		headers.set('Content-Type', 'application/json');
	}
	if (!headers.has('Accept-Language')) {
		headers.set('Accept-Language', acceptLanguage());
	}
	if (writeMethods.has(method)) {
		headers.set('X-CSRF-Token', csrfToken());
	}
	return headers;
}

/** Performs one management request and returns its JSON body.
 *  A failure carries the status and the code the Relo error contract names, so a page can tell an
 *  expired session from a refused write.
 *
 *  The signature is an overload so the body can be handed back without asserting it: the caller's
 *  own `T` is what names the shape the daemon promised, and the implementation reads a body it
 *  does not pretend to know. */
export function api<T>(path: string, init?: RequestInit): Promise<T>;
export async function api(path: string, init: RequestInit = {}): Promise<unknown> {
	const method = requestMethod(init);
	const response = await fetch(base + path, {
		...init,
		method,
		headers: requestHeaders(init, method),
		credentials: 'same-origin',
		signal: init.signal ?? AbortSignal.timeout(requestTimeoutMs)
	});
	if (response.status === 204) {
		notifyAgentCatalogChanged(path, method);
		// A write the daemon acknowledges answers with nothing, which is what a
		// caller awaiting a void result asked for.
		return undefined;
	}
	const payload = await response.json().catch(() => null);
	if (!response.ok) {
		const code = typeof payload?.error?.code === 'string' ? payload.error.code : 'error';
		const message =
			typeof payload?.error?.message === 'string'
				? payload.error.message
				: get(t)('ui.common.requestFailed', { values: { status: response.status } });
		throw new ApiError(response.status, code, message);
	}
	notifyAgentCatalogChanged(path, method);
	return payload;
}

/** Starts one management request whose answer arrives as a stream.
 *  The method and headers are the ones `api` sends, because the daemon answers the same contract
 *  either way; the response comes back unread, because a stream carries frames rather than the JSON
 *  body `api` is for. No timeout is set: a stream is meant to outlive one. */
export async function apiStream(path: string, init: RequestInit = {}): Promise<Response> {
	const method = requestMethod(init);
	return fetch(base + path, {
		...init,
		method,
		headers: requestHeaders(init, method),
		credentials: 'same-origin'
	});
}
