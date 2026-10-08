import { describe, expect, it } from 'vitest';

import { FILTER_PANEL_KEYS, readPanelOpen, writePanelOpen, type StorageLike } from './filter-panel';

function memoryStorage(seed: Record<string, string> = {}): StorageLike {
	const held = new Map(Object.entries(seed));
	return {
		getItem: (key) => held.get(key) ?? null,
		setItem: (key, value) => void held.set(key, value)
	};
}

// deniedStorage is a browser that refuses every read and write.
const deniedStorage: StorageLike = {
	getItem() {
		throw new Error('denied');
	},
	setItem() {
		throw new Error('denied');
	}
};

describe('readPanelOpen', () => {
	it('opens closed without a store or without a settled state', () => {
		expect(readPanelOpen(undefined, FILTER_PANEL_KEYS.usage)).toBe(false);
		expect(readPanelOpen(memoryStorage(), FILTER_PANEL_KEYS.usage)).toBe(false);
	});

	it('reads back the state a page settled on', () => {
		const storage = memoryStorage({ [FILTER_PANEL_KEYS.usage]: 'open' });
		expect(readPanelOpen(storage, FILTER_PANEL_KEYS.usage)).toBe(true);
		expect(readPanelOpen(storage, FILTER_PANEL_KEYS.logs)).toBe(false);
	});

	it('keeps a page closed when the store refuses to be read', () => {
		expect(readPanelOpen(deniedStorage, FILTER_PANEL_KEYS.usage)).toBe(false);
	});
});

describe('writePanelOpen', () => {
	it('round-trips one page without touching another', () => {
		const storage = memoryStorage();
		writePanelOpen(storage, FILTER_PANEL_KEYS.logs, true);
		expect(readPanelOpen(storage, FILTER_PANEL_KEYS.logs)).toBe(true);
		expect(readPanelOpen(storage, FILTER_PANEL_KEYS.usage)).toBe(false);
		writePanelOpen(storage, FILTER_PANEL_KEYS.logs, false);
		expect(readPanelOpen(storage, FILTER_PANEL_KEYS.logs)).toBe(false);
	});

	it('never throws when the store refuses the write', () => {
		expect(() => writePanelOpen(deniedStorage, FILTER_PANEL_KEYS.usage, true)).not.toThrow();
		expect(() => writePanelOpen(undefined, FILTER_PANEL_KEYS.usage, true)).not.toThrow();
	});
});
