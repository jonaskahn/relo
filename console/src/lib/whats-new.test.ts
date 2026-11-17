import { describe, expect, it } from 'vitest';

import { browserStorage } from './filter-panel';
import { markWhatsNewSeen, shouldShowWhatsNew, WHATS_NEW_KEY } from './whats-new';

const seen = (value: string | null) => ({
	getItem: (key: string) => (key === WHATS_NEW_KEY ? value : null),
	setItem: () => undefined
});

const notice = { current: '0.3.0', current_notes: '# v0.3.0\n* New.' };

describe('shouldShowWhatsNew', () => {
	it('shows the notes on a first open and once the version changes', () => {
		expect(shouldShowWhatsNew(seen(null), notice)).toBe(true);
		expect(shouldShowWhatsNew(seen('0.2.13'), notice)).toBe(true);
		expect(shouldShowWhatsNew(seen('0.3.0'), notice)).toBe(false);
	});

	it('never shows a version with no notes', () => {
		expect(shouldShowWhatsNew(seen(null), { current: '0.3.0', current_notes: '  ' })).toBe(false);
		expect(shouldShowWhatsNew(seen(null), null)).toBe(false);
	});

	it('shows once per load when storage refuses reads and writes', () => {
		const refused = {
			getItem: () => {
				throw new Error('denied');
			},
			setItem: () => {
				throw new Error('denied');
			}
		};
		expect(() => markWhatsNewSeen(refused, '0.3.0')).not.toThrow();
		expect(shouldShowWhatsNew(refused, notice)).toBe(true);
		expect(shouldShowWhatsNew(undefined, notice)).toBe(true);
	});

	it('keeps one key across rebuilds of the same release', () => {
		let stored: string | null = null;
		const storage = {
			getItem: (key: string) => (key === WHATS_NEW_KEY ? stored : null),
			setItem: (_key: string, value: string) => (stored = value)
		};
		markWhatsNewSeen(storage, 'v0.3.1+b7df4346');
		expect(stored).toBe('0.3.1');
		expect(shouldShowWhatsNew(storage, { current: '0.3.1', current_notes: '# v' })).toBe(false);
		expect(shouldShowWhatsNew(storage, { current: 'v0.3.1+other', current_notes: '# v' })).toBe(
			false
		);
	});
});

describe('markWhatsNewSeen', () => {
	it('writes the version, so the next open stays quiet', () => {
		let stored: string | null = null;
		const storage = {
			getItem: () => stored,
			setItem: (_key: string, value: string) => (stored = value)
		};
		markWhatsNewSeen(storage, '0.3.0');
		expect(shouldShowWhatsNew(storage, notice)).toBe(false);
	});
});

describe('browserStorage', () => {
	it('is undefined outside a browser', () => {
		expect(browserStorage()).toBeUndefined();
	});
});
