import { describe, expect, it } from 'vitest';

import { latestBetaVersion, updateAvailable, type UpdateNotice } from './updates';

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

describe('latestBetaVersion', () => {
	const beta = (current: string, latest: string, available = false): UpdateNotice => ({
		current,
		latest,
		available,
		url: 'https://example.com/relo'
	});

	it('names the newest beta beside a beta build', () => {
		expect(
			latestBetaVersion(
				'0.1.7-beta.36.1ea2bdb4',
				beta('0.1.7-beta.36.1ea2bdb4', '0.1.7-beta.37.aaaaaaaa')
			)
		).toBe('0.1.7-beta.37.aaaaaaaa');
	});

	it('leaves a newer beta to the update control', () => {
		expect(
			latestBetaVersion(
				'0.1.7-beta.36.1ea2bdb4',
				beta('0.1.7-beta.36.1ea2bdb4', '0.1.7-beta.37.aaaaaaaa', true)
			)
		).toBe('');
	});

	it('names nothing on a stable build', () => {
		expect(latestBetaVersion('0.1.7', beta('0.1.7', '0.1.8-beta.1.aaaaaaaa'))).toBe('');
	});

	it('names nothing before the check answers', () => {
		expect(latestBetaVersion('0.1.7-beta.36.1ea2bdb4', null)).toBe('');
	});
});
