import { describe, expect, it } from 'vitest';

import { updateAvailable, type UpdateNotice } from './updates';

const notice = (available: boolean, url = 'https://example.com/relo'): UpdateNotice => ({
	current: '0.1.0',
	latest: '0.2.0',
	available,
	url
});

describe('updateAvailable', () => {
	it('is true when a newer build has a URL', () => {
		expect(updateAvailable(notice(true))).toBe(true);
	});

	it('is false when the feed has no bump', () => {
		expect(updateAvailable(notice(false))).toBe(false);
	});

	it('is false without a URL', () => {
		expect(updateAvailable(notice(true, ''))).toBe(false);
	});

	it('is false when the check has not run', () => {
		expect(updateAvailable(null)).toBe(false);
	});
});
