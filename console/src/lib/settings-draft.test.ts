import { describe, expect, it } from 'vitest';

import { cloneDraft, draftDirty } from './settings-draft';

describe('cloneDraft', () => {
	it('copies nested windows so an edit does not touch the original', () => {
		const original = { url: 'https://example.test', windows: [[1, 3] as [number, number]] };
		const copy = cloneDraft(original);
		copy.url = 'https://other.test';
		copy.windows[0][0] = 9;
		expect(original.url).toBe('https://example.test');
		expect(original.windows[0][0]).toBe(1);
	});

	it('copies values out of a state proxy', () => {
		// Svelte 5 state is a proxy; structuredClone would refuse it.
		const proxy = new Proxy(
			{ hide: false, window: true, presence: true },
			{
				get(target, key) {
					return Reflect.get(target, key);
				},
				set(target, key, value) {
					Reflect.set(target, key, value);
					return true;
				}
			}
		);
		expect(() => cloneDraft(proxy)).not.toThrow();
		expect(cloneDraft(proxy)).toEqual({ hide: false, window: true, presence: true });
	});
});

describe('draftDirty', () => {
	it('is false for a copy and true once a field or a window changes', () => {
		const saved = { hide: false, window: true, presence: true };
		const draft = cloneDraft(saved);
		expect(draftDirty(draft, saved)).toBe(false);
		draft.window = false;
		expect(draftDirty(draft, saved)).toBe(true);

		const windows = {
			retry_backoff: [
				[1, 3],
				[3, 5],
				[5, 10]
			]
		};
		const edited = cloneDraft(windows);
		edited.retry_backoff[1][0] = 4;
		expect(draftDirty(edited, windows)).toBe(true);
	});
});
