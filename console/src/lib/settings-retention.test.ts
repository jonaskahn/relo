import { describe, expect, it } from 'vitest';

import { MIN_USAGE_DAYS, validUsageDays } from './settings-retention';

describe('validUsageDays', () => {
	it('keeps everything at zero', () => {
		expect(validUsageDays(0)).toBe(true);
	});

	it('accepts the floor and any window above it', () => {
		expect(validUsageDays(MIN_USAGE_DAYS)).toBe(true);
		expect(validUsageDays(30)).toBe(true);
	});

	it('refuses a window below the floor', () => {
		expect(validUsageDays(MIN_USAGE_DAYS - 1)).toBe(false);
		expect(validUsageDays(1)).toBe(false);
		expect(validUsageDays(-1)).toBe(false);
	});

	it('refuses a value that is not a whole number', () => {
		expect(validUsageDays(2.5)).toBe(false);
		expect(validUsageDays(Number.NaN)).toBe(false);
	});
});
