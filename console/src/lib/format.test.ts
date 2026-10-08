import { addMessages, init } from 'svelte-i18n';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';

import en from './i18n/locales/en.json';
import {
	ageOf,
	formatBytes,
	formatCost,
	formatDate,
	formatDateTime,
	formatDollars,
	formatDuration,
	formatMicroPrice,
	formatTokens,
	formatTokensExact,
	formatUnitPrice,
	formatUptime,
	parseDollars,
	relativeTime
} from './format';

// ageOf and relativeTime read the console's translations, so the catalogue has
// to be loaded before anything that renders one.
beforeAll(async () => {
	addMessages('en', en);
	await init({ fallbackLocale: 'en', initialLocale: 'en' });
});

afterAll(() => {
	vi.useRealTimers();
});

describe('formatDuration', () => {
	it('states milliseconds below a second', () => {
		expect(formatDuration(0)).toBe('0 ms');
		expect(formatDuration(1)).toBe('1 ms');
		expect(formatDuration(999)).toBe('999 ms');
	});

	it('keeps a decimal below ten seconds and drops it above', () => {
		expect(formatDuration(1_500)).toBe('1.5 s');
		expect(formatDuration(9_400)).toBe('9.4 s');
		expect(formatDuration(10_000)).toBe('10 s');
		expect(formatDuration(59_000)).toBe('59 s');
	});

	it('states minutes and the seconds beside them', () => {
		expect(formatDuration(60_000)).toBe('1m 0s');
		expect(formatDuration(90_000)).toBe('1m 30s');
		expect(formatDuration(59 * 60_000 + 59_000)).toBe('59m 59s');
	});

	it('states hours with the fraction of an hour the minutes are', () => {
		expect(formatDuration(3_600_000)).toBe('1.0 h');
		expect(formatDuration(3_600_000 + 30 * 60_000)).toBe('1.5 h');
		expect(formatDuration(25 * 3_600_000)).toBe('25.0 h');
	});
});

describe('formatUptime', () => {
	it('states seconds, minutes, hours, and days as the largest that fits', () => {
		expect(formatUptime(0)).toBe('0s');
		expect(formatUptime(45)).toBe('45s');
		expect(formatUptime(600)).toBe('10m');
		expect(formatUptime(3600 + 120)).toBe('1h 2m');
		expect(formatUptime(86_400 + 3600 * 5)).toBe('1d 5h');
	});

	it('keeps the hours beside the days and drops the minutes', () => {
		expect(formatUptime(86_400 + 3600 * 5 + 120)).toBe('1d 5h');
	});
});

describe('formatBytes', () => {
	it('states bytes below a kibibyte', () => {
		expect(formatBytes(0)).toBe('0 B');
		expect(formatBytes(1023)).toBe('1023 B');
	});

	it('scales to the largest unit that fits and stops there', () => {
		expect(formatBytes(1024)).toBe('1.0 KiB');
		expect(formatBytes(1536)).toBe('1.5 KiB');
		expect(formatBytes(1024 ** 2)).toBe('1.0 MiB');
		expect(formatBytes(1024 ** 3)).toBe('1.0 GiB');
		expect(formatBytes(1024 ** 4)).toBe('1.0 TiB');
		// Past the largest unit the figure keeps growing rather than inventing
		// a unit nothing knows.
		expect(formatBytes(1024 ** 5)).toBe('1024 TiB');
	});

	it('drops the decimal once the figure is two digits or more', () => {
		expect(formatBytes(15 * 1024)).toBe('15 KiB');
	});
});

describe('formatTokens', () => {
	it('states a count below a thousand as it is', () => {
		expect(formatTokens(0)).toBe('0');
		expect(formatTokens(999)).toBe('999');
	});

	it('abbreviates thousands, keeping a decimal below ten thousand', () => {
		expect(formatTokens(1000)).toBe('1.0k');
		expect(formatTokens(9999)).toBe('10.0k');
		expect(formatTokens(10_000)).toBe('10k');
		expect(formatTokens(999_000)).toBe('999k');
		expect(formatTokens(999_999)).toBe('1000k');
	});

	it('abbreviates millions with more decimals for the smaller ones', () => {
		expect(formatTokens(1_000_000)).toBe('1.00M');
		expect(formatTokens(9_999_999)).toBe('10.00M');
		expect(formatTokens(10_000_000)).toBe('10.0M');
		expect(formatTokens(1_400_000)).toBe('1.40M');
	});

	it('abbreviates billions and above with one decimal', () => {
		expect(formatTokens(1_000_000_000)).toBe('1.0B');
		expect(formatTokens(1_234_000_000)).toBe('1.2B');
		expect(formatTokens(12_340_000_000)).toBe('12.3B');
		expect(formatTokens(999_999_999)).toBe('1000.0M');
		expect(formatTokens(1_000_000_000_000)).toBe('1.0T');
		expect(formatTokens(1_234_000_000_000)).toBe('1.2T');
		expect(formatTokens(1_234_000_000_000_000)).toBe('1.2Qa');
		expect(formatTokens(1_234_000_000_000_000_000)).toBe('1.2Qi');
	});
});

describe('formatTokensExact', () => {
	it('states the whole count with its unit behind an abbreviation', () => {
		expect(formatTokensExact(999)).toBe('999 tokens');
		expect(formatTokensExact(1_234_567_890)).toBe((1_234_567_890).toLocaleString() + ' tokens');
	});
});

describe('formatCost', () => {
	it('states an unknown cost as a dash', () => {
		expect(formatCost(null)).toBe('—');
		expect(formatCost(undefined)).toBe('—');
	});

	it('states no cost as a round zero', () => {
		expect(formatCost(0)).toBe('$0.00');
	});

	it('keeps enough decimals that a non-zero cost never reads as free', () => {
		// Under a cent keeps six decimals, so a spend of 9999 micros reads as
		// the fraction of a cent it is rather than as nothing.
		expect(formatCost(1)).toBe('$0.000001');
		expect(formatCost(9999)).toBe('$0.009999');
		// A cent and over keeps four until it reaches a dollar.
		expect(formatCost(10_000)).toBe('$0.0100');
		expect(formatCost(100_000)).toBe('$0.1000');
		expect(formatCost(990_000)).toBe('$0.9900');
		expect(formatCost(1_000_000)).toBe('$1.00');
		expect(formatCost(4_200_000)).toBe('$4.20');
	});
});

describe('formatMicroPrice', () => {
	it('renders a rate the way a cost is rendered', () => {
		expect(formatMicroPrice(1_500_000)).toBe(formatCost(1_500_000));
		expect(formatMicroPrice(1)).toBe(formatCost(1));
	});
});

describe('formatUnitPrice', () => {
	it('states an unknown rate as a dash and no rate without decimals', () => {
		expect(formatUnitPrice(null)).toBe('—');
		expect(formatUnitPrice(undefined)).toBe('—');
		expect(formatUnitPrice(0)).toBe('$0');
	});

	it('keeps a cheap model from being shown as free', () => {
		expect(formatUnitPrice(1)).toBe('$0.0000');
		expect(formatUnitPrice(9999)).toBe('$0.0100');
		expect(formatUnitPrice(100_000)).toBe('$0.100');
		expect(formatUnitPrice(999_999)).toBe('$1.000');
		expect(formatUnitPrice(2_500_000)).toBe('$2.50');
	});
});

describe('formatDollars', () => {
	it('reads back the figure an operator would type', () => {
		expect(formatDollars(null)).toBe('');
		expect(formatDollars(undefined)).toBe('');
		expect(formatDollars(0)).toBe('0');
		expect(formatDollars(1_250_000)).toBe('1.25');
	});
});

describe('parseDollars', () => {
	it('states no rate for an empty or unusable entry', () => {
		expect(parseDollars('')).toBeNull();
		expect(parseDollars('   ')).toBeNull();
		expect(parseDollars('free')).toBeNull();
		expect(parseDollars('-1')).toBeNull();
	});

	it('reads dollars into integer micros, and zero as a rate of zero', () => {
		expect(parseDollars('1.25')).toBe(1_250_000);
		expect(parseDollars(' 2 ')).toBe(2_000_000);
		expect(parseDollars('0')).toBe(0);
	});
});

describe('ageOf', () => {
	it('states a fetch that just happened, or never happened, as just now', () => {
		expect(ageOf(undefined)).toContain('just');
		expect(ageOf(null)).toContain('just');
		expect(ageOf(0)).toContain('just');
		expect(ageOf(Date.now())).toContain('just');
	});

	it('ages a fetch in minutes, hours, and days', () => {
		expect(ageOf(Date.now() - 5 * 60_000)).toContain('5');
		expect(ageOf(Date.now() - 3 * 3_600_000)).toContain('3');
		expect(ageOf(Date.now() - 2 * 86_400_000)).toContain('2');
	});

	it('states a fetch from the future as just now rather than a negative age', () => {
		expect(ageOf(Date.now() + 60_000)).toContain('just');
	});
});

describe('formatDateTime and formatDate', () => {
	it('state an absent timestamp as a dash', () => {
		expect(formatDateTime(undefined)).toBe('—');
		expect(formatDateTime(null)).toBe('—');
		expect(formatDate(undefined)).toBe('—');
		expect(formatDate(null)).toBe('—');
	});

	it('render a timestamp the locale can read', () => {
		const ms = Date.UTC(2026, 0, 15, 12, 30);
		expect(formatDateTime(ms)).not.toBe('—');
		expect(formatDate(ms)).not.toBe('—');
		expect(formatDate(ms)).toContain('2026');
	});
});

describe('relativeTime', () => {
	it('states an absent timestamp as a dash', () => {
		expect(relativeTime(undefined)).toBe('—');
		expect(relativeTime(null)).toBe('—');
	});

	it('states the span since as a compact age', () => {
		expect(relativeTime(Date.now())).toBe('<1m');
		expect(relativeTime(Date.now() - 5 * 60_000)).toContain('5m');
		expect(relativeTime(Date.now() - 3 * 3_600_000)).toContain('3h');
		expect(relativeTime(Date.now() - 2 * 86_400_000)).toContain('2d');
	});
});
