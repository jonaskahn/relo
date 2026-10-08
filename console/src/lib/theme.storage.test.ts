import { afterEach, describe, expect, it, vi } from 'vitest';

import {
	applyAccent,
	applySettings,
	loadSettings,
	saveSettings,
	watchSystemTheme
} from './theme.svelte';

// These cover the parts of the appearance module the theme suite does not
// reach: a browser with no storage, one that refuses to read or write, the
// accent attribute, and the live system-theme watcher.
afterEach(() => {
	vi.unstubAllGlobals();
});

function stubStorage(getItem: () => string | null, setItem = vi.fn()) {
	vi.stubGlobal('localStorage', { getItem, setItem });
	return setItem;
}

describe('loadSettings without storage', () => {
	it('answers the defaults where there is no local storage', () => {
		// A server render has no localStorage, so the console paints its
		// defaults rather than failing to read a preference.
		vi.stubGlobal('localStorage', undefined);
		const settings = loadSettings();

		expect(settings.theme).toBe('system');
		expect(settings.accent).toBe('red');
		expect(settings.cardLayout).toBe('vertical');
		expect(settings.sidebar).toEqual({ mode: 'compact' });
	});

	it('answers the defaults for a store that has nothing saved', () => {
		stubStorage(() => null);
		expect(loadSettings().theme).toBe('system');
	});

	it('answers the defaults for a store holding something unreadable', () => {
		stubStorage(() => 'not json');
		expect(loadSettings().theme).toBe('system');
	});

	it('answers the defaults when the store refuses to be read', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => {
				throw new Error('denied');
			},
			setItem: vi.fn()
		});
		expect(loadSettings().theme).toBe('system');
	});

	it('still reads the layout when the cleanup write is refused', () => {
		// Appearance moved to the shared config, so an older stored blob is
		// rewritten without it. A browser that refuses that write still keeps
		// the layout it already had.
		const setItem = vi.fn(() => {
			throw new Error('denied');
		});
		stubStorage(() => JSON.stringify({ theme: 'light', cardLayout: 'horizontal' }), setItem);

		const settings = loadSettings();
		expect(settings.cardLayout).toBe('horizontal');
		expect(settings.theme).toBe('system');
	});
});

describe('loadSettings merging', () => {
	it('takes a stored card layout over the defaults', () => {
		stubStorage(() => JSON.stringify({ cardLayout: 'horizontal' }));
		expect(loadSettings().cardLayout).toBe('horizontal');
	});

	it('ignores stored menu sections and top bar settings', () => {
		stubStorage(() =>
			JSON.stringify({
				sidebar: {
					mode: 'full',
					sections: [{ id: 'activity', visible: false, order: 0, links: [] }]
				},
				topbar: { showHealth: false, showQuickAdd: false }
			})
		);
		const settings = loadSettings();

		expect(settings.sidebar).toEqual({ mode: 'full' });
		expect(settings).not.toHaveProperty('topbar');
	});

	it('ignores a stored card layout it does not know', () => {
		stubStorage(() => JSON.stringify({ cardLayout: 'diagonal' }));
		expect(loadSettings().cardLayout).toBe('vertical');
	});

	it('reads an expanded rail beside the page as full', () => {
		stubStorage(() => JSON.stringify({ sidebar: { mode: 'always' } }));
		expect(loadSettings().sidebar.mode).toBe('full');
	});

	it('reads a full rail that was stored as expanded but not docked as compact', () => {
		stubStorage(() => JSON.stringify({ sidebar: { mode: 'always', docked: false } }));
		expect(loadSettings().sidebar.mode).toBe('compact');
	});

	it('reads an old overlay rail as the icon column', () => {
		stubStorage(() => JSON.stringify({ sidebar: { mode: 'overlay' } }));
		expect(loadSettings().sidebar.mode).toBe('compact');
	});

	it('reads a rail with no stored mode as the icon column', () => {
		stubStorage(() => JSON.stringify({ sidebar: {} }));
		expect(loadSettings().sidebar.mode).toBe('compact');
	});

	it('reads a stored mode it knows as itself', () => {
		stubStorage(() => JSON.stringify({ sidebar: { mode: 'full' } }));
		expect(loadSettings().sidebar.mode).toBe('full');

		stubStorage(() => JSON.stringify({ sidebar: { mode: 'compact' } }));
		expect(loadSettings().sidebar.mode).toBe('compact');
	});
});

describe('saveSettings', () => {
	it('stores the layout and not the appearance the config owns', () => {
		const setItem = stubStorage(() => null);
		const settings = loadSettings();
		settings.accent = 'blue';
		settings.theme = 'dark';

		saveSettings(settings);

		const saved = JSON.parse(setItem.mock.calls[0][1] as string);
		expect(saved).not.toHaveProperty('accent');
		expect(saved).not.toHaveProperty('theme');
		expect(saved.cardLayout).toBe('vertical');
	});

	it('stores nothing where there is no local storage', () => {
		vi.stubGlobal('localStorage', undefined);
		expect(() => saveSettings(loadSettings())).not.toThrow();
	});
});

describe('applyAccent', () => {
	it('names the accent on the document element', () => {
		const setAttribute = vi.fn();
		vi.stubGlobal('document', { documentElement: { setAttribute } });

		applyAccent('green');

		expect(setAttribute).toHaveBeenCalledWith('data-accent', 'green');
	});

	it('does nothing where there is no document', () => {
		vi.stubGlobal('document', undefined);
		expect(() => applyAccent('blue')).not.toThrow();
	});
});

describe('applySettings', () => {
	it('applies the theme and the accent together', () => {
		const classes = new Set<string>();
		const attributes: Record<string, string> = {};
		vi.stubGlobal('document', {
			documentElement: {
				dataset: {},
				classList: {
					contains: (name: string) => classes.has(name),
					toggle: (name: string, force: boolean) => {
						if (force) classes.add(name);
						else classes.delete(name);
					}
				},
				setAttribute: (name: string, value: string) => {
					attributes[name] = value;
				}
			}
		});
		vi.stubGlobal('window', { matchMedia: () => ({ matches: false }) });

		applySettings({
			theme: 'dark',
			accent: 'teal',
			cardLayout: 'vertical',
			sidebar: loadSettings().sidebar
		});

		expect(classes.has('dark')).toBe(true);
		expect(attributes['data-accent']).toBe('teal');
	});
});

describe('watchSystemTheme', () => {
	it('follows the OS preference while the operator chose the system', () => {
		const handlers: Array<() => void> = [];
		const removeEventListener = vi.fn();
		vi.stubGlobal('window', {
			matchMedia: () => ({
				matches: true,
				addEventListener: (_name: string, handler: () => void) => handlers.push(handler),
				removeEventListener
			})
		});

		const onChange = vi.fn();
		const stop = watchSystemTheme(onChange);

		handlers.forEach((handler) => handler());
		expect(onChange).toHaveBeenCalled();

		stop();
		expect(removeEventListener).toHaveBeenCalledWith('change', onChange);
	});

	it('answers a no-op where there is no window', () => {
		vi.stubGlobal('window', undefined);
		const stop = watchSystemTheme(vi.fn());
		expect(() => stop()).not.toThrow();
	});
});
