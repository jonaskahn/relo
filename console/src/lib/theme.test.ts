import { afterEach, describe, expect, it, vi } from 'vitest';
import {
	applyTheme,
	cardLayoutCard,
	cardLayoutGrid,
	cardLayoutSlot,
	cardLayoutSpan,
	isAccent,
	isCardLayout,
	isThemeChoice,
	loadSettings,
	saveSettings
} from './theme.svelte';

function stubThemeBrowser({
	dark = false,
	ready = true,
	reducedMotion = false,
	systemDark = false,
	startViewTransition
}: {
	dark?: boolean;
	ready?: boolean;
	reducedMotion?: boolean;
	systemDark?: boolean;
	startViewTransition?: (update: () => void) => { finished: Promise<void> };
}) {
	let isDark = dark;
	const root = {
		classList: {
			contains: (name: string) => name === 'dark' && isDark,
			toggle: (_name: string, force: boolean) => (isDark = force)
		},
		dataset: ready ? { themeReady: '' } : {}
	};
	vi.stubGlobal('document', { documentElement: root, startViewTransition });
	vi.stubGlobal('window', {
		matchMedia: (query: string) =>
			query === '(prefers-reduced-motion: reduce)'
				? { matches: reducedMotion }
				: { matches: systemDark }
	});
	return { root, isDark: () => isDark };
}

describe('shared appearance', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('uses the shell choice and removes old browser appearance values', () => {
		const setItem = vi.fn();
		vi.stubGlobal('window', { __reloAppearance: { theme: 'dark', accent: 'blue' } });
		vi.stubGlobal('localStorage', {
			getItem: () =>
				JSON.stringify({
					theme: 'light',
					accent: 'red',
					sidebar: { mode: 'always', docked: false }
				}),
			setItem
		});
		const settings = loadSettings();
		expect(settings.theme).toBe('dark');
		expect(settings.accent).toBe('blue');
		expect(settings.sidebar.mode).toBe('compact');
		expect(settings.sidebar).not.toHaveProperty('docked');
		expect(JSON.parse(setItem.mock.calls[0][1])).toEqual({
			sidebar: { mode: 'always', docked: false }
		});

		saveSettings(settings);
		const saved = JSON.parse(setItem.mock.calls[1][1]);
		expect(saved).not.toHaveProperty('theme');
		expect(saved).not.toHaveProperty('accent');
		expect(saved.cardLayout).toBe('vertical');
		expect(saved.sidebar.mode).toBe('compact');
		expect(saved.sidebar).not.toHaveProperty('docked');
	});

	it('folds a 2.4 accent id onto the 3.0 set', () => {
		vi.stubGlobal('localStorage', undefined);
		vi.stubGlobal('window', { __reloAppearance: { theme: 'light', accent: 'emerald' } });
		expect(loadSettings().accent).toBe('green');
		vi.stubGlobal('window', { __reloAppearance: { theme: 'light', accent: 'violet' } });
		expect(loadSettings().accent).toBe('purple');
	});

	it('defaults card layout to vertical when the stored blob has none', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => JSON.stringify({ sidebar: { mode: 'always', docked: false } }),
			setItem: vi.fn()
		});
		expect(loadSettings().cardLayout).toBe('vertical');
	});

	it('lets a page share one card size while vertical and horizontal stay distinct', () => {
		expect(cardLayoutGrid('vertical')).toContain('xl:grid-cols-4');
		expect(cardLayoutGrid('horizontal')).toContain('xl:grid-cols-3');
		expect(cardLayoutGrid('vertical')).not.toContain('items-start');
		expect(cardLayoutCard('vertical')).toBe('flex h-full min-h-0 flex-col');
		expect(cardLayoutCard('horizontal')).toBe('flex h-full min-h-0 flex-col');
		expect(cardLayoutSpan('vertical')).toContain('xl:col-span-4');
		expect(cardLayoutSpan('horizontal')).toContain('xl:col-span-3');
	});

	it('sizes a toolbar group to one card column of the same grid', () => {
		expect(cardLayoutSlot('vertical')).toContain('sm:w-[calc((100%-0.75rem)/2)]');
		expect(cardLayoutSlot('vertical')).toContain('xl:w-[calc((100%-3*0.75rem)/4)]');
		expect(cardLayoutSlot('horizontal')).toContain('xl:w-[calc((100%-2*0.75rem)/3)]');
	});

	it.each([
		{ fromDark: false, theme: 'dark' as const, direction: 'sunset' },
		{ fromDark: true, theme: 'light' as const, direction: 'sunrise' }
	])(
		'animates a $direction when the resolved theme changes',
		async ({ fromDark, theme, direction }) => {
			const finished = Promise.resolve();
			const startViewTransition = vi.fn((update: () => void) => {
				update();
				return { finished };
			});
			const { root, isDark } = stubThemeBrowser({ dark: fromDark, startViewTransition });

			applyTheme(theme);

			expect(startViewTransition).toHaveBeenCalledOnce();
			expect(root.dataset).toHaveProperty('themeDir', direction);
			expect(isDark()).toBe(theme === 'dark');
			await finished;
			expect(root.dataset).not.toHaveProperty('themeDir');
		}
	);

	it('follows the OS preference while the choice is system', () => {
		const { isDark } = stubThemeBrowser({ systemDark: true });

		applyTheme('system');

		expect(isDark()).toBe(true);
	});

	it('does not animate the first theme sync or an unchanged theme', () => {
		const startViewTransition = vi.fn();
		const { root } = stubThemeBrowser({ ready: false, startViewTransition });

		applyTheme('light');
		applyTheme('light');

		expect(startViewTransition).not.toHaveBeenCalled();
		expect(root.dataset).toHaveProperty('themeReady');
	});

	it('applies reduced-motion theme changes without a view transition', () => {
		const startViewTransition = vi.fn();
		const { isDark } = stubThemeBrowser({ reducedMotion: true, startViewTransition });

		applyTheme('dark');

		expect(startViewTransition).not.toHaveBeenCalled();
		expect(isDark()).toBe(true);
	});

	it('falls back to an instant theme change without View Transitions support', () => {
		const { isDark } = stubThemeBrowser({});

		applyTheme('dark');

		expect(isDark()).toBe(true);
	});
});

describe('the guards over an appearance value', () => {
	it('accepts only the themes the shell resolves', () => {
		expect(isThemeChoice('system')).toBe(true);
		expect(isThemeChoice('light')).toBe(true);
		expect(isThemeChoice('dark')).toBe(true);
		expect(isThemeChoice('solarized')).toBe(false);
		expect(isThemeChoice('')).toBe(false);
	});

	it('accepts only the accents the picker offers', () => {
		expect(isAccent('rose')).toBe(true);
		expect(isAccent('emerald')).toBe(false);
		expect(isAccent('blue-ish')).toBe(false);
	});

	it('accepts only the two densities the grids pack by', () => {
		expect(isCardLayout('vertical')).toBe(true);
		expect(isCardLayout('horizontal')).toBe(true);
		expect(isCardLayout('compact')).toBe(false);
	});
});
