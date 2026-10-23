import { describe, expect, it } from 'vitest';

import { latestVersion, onBetaChannel, updateAvailable, type UpdateNotice } from './updates';

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

describe('latestVersion', () => {
	const channel = (current: string, latest: string, available = false): UpdateNotice => ({
		current,
		latest,
		available,
		url: 'https://example.com/relo'
	});

	it('names the newest beta beside a current beta build', () => {
		expect(latestVersion(channel('0.1.7-beta.36.1ea2bdb4', '0.1.7-beta.36.1ea2bdb4'))).toBe(
			'0.1.7-beta.36.1ea2bdb4'
		);
	});

	it('names the newest release beside a current stable build', () => {
		expect(latestVersion(channel('0.1.7', '0.1.7'))).toBe('0.1.7');
	});

	it('leaves a newer build to the update control', () => {
		expect(latestVersion(channel('0.1.7', '0.1.8', true))).toBe('');
		expect(latestVersion(channel('0.1.7-beta.36.1ea2bdb4', '0.1.7-beta.37.aaaaaaaa', true))).toBe(
			''
		);
	});

	it('names nothing before the check answers', () => {
		expect(latestVersion(null)).toBe('');
		expect(latestVersion(channel('0.1.7', ''))).toBe('');
	});
});

describe('onBetaChannel', () => {
	it('follows the version suffix', () => {
		expect(onBetaChannel('0.1.7-beta.36.1ea2bdb4')).toBe(true);
		expect(onBetaChannel('0.1.7')).toBe(false);
	});
});
