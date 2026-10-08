import { describe, expect, it } from 'vitest';

import { csrfFromCookie, mutationAffectsAgentCatalog } from './api';

describe('csrfFromCookie', () => {
	it('reads the token out of a cookie header', () => {
		expect(csrfFromCookie('relo_session=abc; relo_csrf=token-123')).toBe('token-123');
	});

	it('reads a token that is the only cookie', () => {
		expect(csrfFromCookie('relo_csrf=only-token')).toBe('only-token');
	});

	it('answers an empty string when the cookie is absent', () => {
		expect(csrfFromCookie('relo_session=abc')).toBe('');
		expect(csrfFromCookie('')).toBe('');
	});

	it('does not match a lookalike cookie name', () => {
		expect(csrfFromCookie('not_relo_csrf=nope')).toBe('');
	});
});

describe('mutationAffectsAgentCatalog', () => {
	it('tracks writes that can make configured agents stale', () => {
		expect(mutationAffectsAgentCatalog('/routes/fast', 'PUT')).toBe(true);
		expect(mutationAffectsAgentCatalog('/models/openai/gpt-5', 'PATCH')).toBe(true);
		expect(mutationAffectsAgentCatalog('/connections/openai/models/refresh', 'POST')).toBe(true);
		expect(mutationAffectsAgentCatalog('/catalog/refresh', 'POST')).toBe(true);
		expect(mutationAffectsAgentCatalog('/settings/providers', 'PATCH')).toBe(true);
	});

	it('ignores reads and unrelated writes', () => {
		expect(mutationAffectsAgentCatalog('/routes/fast', 'GET')).toBe(false);
		expect(mutationAffectsAgentCatalog('/settings/language', 'PATCH')).toBe(false);
		expect(mutationAffectsAgentCatalog('/settings/providers', 'GET')).toBe(false);
		expect(mutationAffectsAgentCatalog('/auth/logout', 'POST')).toBe(false);
	});
});
