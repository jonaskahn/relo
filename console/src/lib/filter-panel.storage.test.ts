import { afterEach, describe, expect, it, vi } from 'vitest';

import { browserStorage, readPanelOpen, FILTER_PANEL_KEYS, type StorageLike } from './filter-panel';

// browserStorage reaches for localStorage, which the console runs with in a
// browser and a test does not. These cover the three shapes it can find there:
// a working store, a browser with storage denied, and no localStorage at all.
afterEach(() => {
	vi.unstubAllGlobals();
});

describe('browserStorage', () => {
	it('is the local storage where the console runs', () => {
		const store: StorageLike = { getItem: () => null, setItem: () => undefined };
		vi.stubGlobal('localStorage', store);

		expect(browserStorage()).toBe(store);
	});

	it('is undefined where there is no local storage', () => {
		// A server render has no localStorage at all.
		vi.stubGlobal('localStorage', undefined);
		expect(browserStorage()).toBeUndefined();
	});

	it('is undefined where the browser refuses storage', () => {
		// Private browsing can throw on the property itself rather than on a
		// read, which is the same condition as a refused write.
		Object.defineProperty(globalThis, 'localStorage', {
			configurable: true,
			get() {
				throw new Error('denied');
			}
		});

		expect(browserStorage()).toBeUndefined();

		delete (globalThis as { localStorage?: unknown }).localStorage;
	});

	it('reads and writes a panel through the storage it found', () => {
		const held = new Map<string, string>();
		const store: StorageLike = {
			getItem: (key: string) => held.get(key) ?? null,
			setItem: (key: string, value: string) => void held.set(key, value)
		};
		vi.stubGlobal('localStorage', store);

		const storage = browserStorage();
		expect(storage).toBeDefined();
		expect(readPanelOpen(storage, FILTER_PANEL_KEYS.usage)).toBe(false);
	});
});
