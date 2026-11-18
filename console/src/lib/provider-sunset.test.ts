import { describe, expect, it } from 'vitest';

import { isSunsetConnection, sunsetNoticeKey } from './provider-sunset';

describe('isSunsetConnection', () => {
	// Google disabled Cloud Code Assist for accounts using a third-party
	// client, so the connection ships off and says why when it is switched on.
	it('marks the Antigravity connection closed at the vendor', () => {
		expect(isSunsetConnection('google-antigravity')).toBe(true);
	});

	// Every other lane still answers, so none of them carries the caveat.
	it('leaves every other connection open', () => {
		expect(isSunsetConnection('claude')).toBe(false);
		expect(isSunsetConnection('openai-codex')).toBe(false);
		expect(isSunsetConnection('grok')).toBe(false);
	});
});

describe('sunsetNoticeKey', () => {
	it('names the notice for a closed connection', () => {
		expect(sunsetNoticeKey('google-antigravity')).toBe('ui.pages.providersPage.banner.sunset');
	});

	it('names no notice for an open one', () => {
		expect(sunsetNoticeKey('claude')).toBe('');
	});
});
