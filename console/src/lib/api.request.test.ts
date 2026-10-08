import { addMessages, init, locale } from 'svelte-i18n';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

import en from './i18n/locales/en.json';
import { api, ApiError, acceptLanguage, apiStream, csrfToken } from './api';

// The request path reads the console's translations and the CSRF cookie, so
// both have to be in place before a request is made.
beforeAll(async () => {
	addMessages('en', en);
	await init({ fallbackLocale: 'en', initialLocale: 'en' });
});

/** Narrows a caught value to the error the request path throws, failing the test when the request
 *  threw something else. */
function asApiError(value: unknown): ApiError {
	if (!(value instanceof ApiError)) {
		throw new Error('expected an ApiError, got ' + String(value));
	}
	return value;
}

// These tests run without a DOM, so the two globals the request path touches
// are stood in for: the cookie the CSRF token is read from, and the window the
// agent-catalog event is announced on.
let cookie = '';
let announced = 0;

beforeEach(() => {
	cookie = '';
	announced = 0;
	vi.stubGlobal('document', {
		get cookie() {
			return cookie;
		},
		// svelte-i18n mirrors the active language onto the document element.
		documentElement: { setAttribute: () => undefined }
	});
	vi.stubGlobal('window', {
		dispatchEvent: () => {
			announced++;
			return true;
		}
	});
});

const requests: Array<{ url: string; init: RequestInit }> = [];

function respond(body: unknown, status = 200) {
	requests.length = 0;
	vi.stubGlobal(
		'fetch',
		vi.fn((url: string, init: RequestInit) => {
			requests.push({ url, init });
			return Promise.resolve({
				ok: status >= 200 && status < 300,
				status,
				json: () => Promise.resolve(body)
			});
		})
	);
}

function headersOf(index = 0): Headers {
	return new Headers(requests[index].init.headers);
}

beforeEach(() => {
	locale.set('en');
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('acceptLanguage', () => {
	it('is the language the console renders in, so prose and page agree', () => {
		locale.set('de');
		expect(acceptLanguage()).toBe('de');
		locale.set('en');
		expect(acceptLanguage()).toBe('en');
	});
});

describe('csrfToken', () => {
	it('reads the token out of the readable cookie', () => {
		cookie = 'relo_csrf=from-document';
		expect(csrfToken()).toBe('from-document');
	});

	it('answers an empty string when no cookie carries one', () => {
		cookie = 'relo_session=abc';
		expect(csrfToken()).toBe('');
	});
});

describe('api', () => {
	it('requests the management path with the cookies the console needs', async () => {
		respond({ ok: true });
		await api('/status');

		expect(requests).toHaveLength(1);
		expect(requests[0].url).toBe('/api/v1/status');
		expect(requests[0].init.method).toBe('GET');
		expect(requests[0].init.credentials).toBe('same-origin');
	});

	it('names the language so the daemon answers in it', async () => {
		respond({ ok: true });
		locale.set('de');
		await api('/status');
		expect(headersOf().get('Accept-Language')).toBe('de');
	});

	it('keeps an Accept-Language the caller chose', async () => {
		respond({ ok: true });
		await api('/status', { headers: { 'Accept-Language': 'fr' } });
		expect(headersOf().get('Accept-Language')).toBe('fr');
	});

	it('echoes the CSRF token on a write and not on a read', async () => {
		cookie = 'relo_csrf=write-token';
		respond({ ok: true });
		await api('/routes/fast', { method: 'PUT', body: '{}' });
		expect(headersOf().get('X-CSRF-Token')).toBe('write-token');

		respond({ ok: true });
		await api('/status');
		expect(headersOf().get('X-CSRF-Token')).toBeNull();
	});

	it('defaults a body to JSON but keeps the type the caller chose', async () => {
		respond({ ok: true });
		await api('/routes/fast', { method: 'POST', body: '{}' });
		expect(headersOf().get('Content-Type')).toBe('application/json');

		respond({ ok: true });
		await api('/catalog/refresh', {
			method: 'POST',
			body: 'raw',
			headers: { 'Content-Type': 'text/plain' }
		});
		expect(headersOf().get('Content-Type')).toBe('text/plain');
	});

	it('adds no content type to a read, which carries no body', async () => {
		respond({ ok: true });
		await api('/status');
		expect(headersOf().get('Content-Type')).toBeNull();
	});

	it('answers nothing for a write that returned no content', async () => {
		respond(undefined, 204);
		await expect(api('/routes/fast', { method: 'DELETE' })).resolves.toBeUndefined();
	});

	it('throws the status and the code the error contract names', async () => {
		respond({ error: { code: 'session_expired', message: 'sign in again' } }, 401);
		const failure = await api('/status').catch((error: unknown) => error);

		const apiError = asApiError(failure);
		expect(apiError.status).toBe(401);
		expect(apiError.code).toBe('session_expired');
		expect(apiError.message).toBe('sign in again');
		expect(apiError.name).toBe('ApiError');
	});

	it('falls back to a generic code and a translated message', async () => {
		respond({ nothing: true }, 500);
		const failure = asApiError(await api('/status').catch((error: unknown) => error));

		expect(failure.code).toBe('error');
		expect(failure.message).toContain('500');

		respond(null, 503);
		const unreadable = asApiError(await api('/status').catch((error: unknown) => error));
		expect(unreadable.code).toBe('error');
		expect(unreadable.message).toContain('503');
	});

	it('carries on when the error body is not the shape the contract names', async () => {
		respond({ error: { code: 42, message: null } }, 400);
		const failure = asApiError(await api('/status').catch((error: unknown) => error));

		expect(failure.status).toBe(400);
		expect(failure.code).toBe('error');
		expect(failure.message).toContain('400');
	});

	it('announces a write that can make configured agents stale', async () => {
		respond({ ok: true });
		await api('/catalog/refresh', { method: 'POST' });
		expect(announced).toBe(1);

		respond({ ok: true });
		await api('/status');
		expect(announced).toBe(1);

		respond({ ok: true });
		await api('/auth/logout', { method: 'POST' });
		expect(announced).toBe(1);
	});

	it('announces a write that returned no content too', async () => {
		respond(undefined, 204);
		await api('/models/openai/gpt-5', { method: 'DELETE' });
		expect(announced).toBe(1);
	});

	it('never announces a failed write', async () => {
		respond({ error: { code: 'refused', message: 'no' } }, 403);
		await api('/routes/fast', { method: 'PUT' }).catch(() => undefined);
		expect(announced).toBe(0);
	});

	it('defaults a missing method to a read', async () => {
		respond({ ok: true });
		await api('/status', { method: 'get' });
		expect(requests[0].init.method).toBe('GET');
	});

	it('arms its own deadline and keeps the signal the caller brought', async () => {
		respond({ ok: true });
		await api('/status');
		expect(requests[0].init.signal).toBeInstanceOf(AbortSignal);

		respond({ ok: true });
		const controller = new AbortController();
		await api('/integrations/chat', { method: 'POST', body: '{}', signal: controller.signal });
		expect(requests[0].init.signal).toBe(controller.signal);
	});
});

describe('apiStream', () => {
	it('sends the same headers a request does, so a stream answers the same contract', async () => {
		respond({ ok: true });
		cookie = 'relo_csrf=from-document';
		locale.set('de');
		await apiStream('/integrations/chat', { method: 'POST', body: '{}' });

		expect(requests[0].url).toBe('/api/v1/integrations/chat');
		expect(requests[0].init.credentials).toBe('same-origin');
		expect(headersOf().get('Accept-Language')).toBe('de');
		expect(headersOf().get('X-CSRF-Token')).toBe('from-document');
		expect(headersOf().get('Content-Type')).toBe('application/json');
	});

	it('keeps a header the caller chose', async () => {
		respond({ ok: true });
		await apiStream('/integrations/chat', {
			method: 'POST',
			headers: { Accept: 'text/event-stream' },
			body: '{}'
		});
		expect(headersOf().get('Accept')).toBe('text/event-stream');
	});

	it('hands the response back unread and un-deadlined, because a stream outlives one request', async () => {
		respond({ ok: true });
		const response = await apiStream('/integrations/chat', { method: 'POST', body: '{}' });
		expect(response.status).toBe(200);
		expect(requests[0].init.signal).toBeUndefined();
	});
});
