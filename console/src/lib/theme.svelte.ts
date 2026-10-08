// The console's settable state: theme, accent, card layout, and the menu's
// full/compact mode. Layout persists in localStorage; appearance comes
// from the shared daemon config and is injected into the shell before paint.

/** The accents the appearance picker offers, in the order it lists them. */
export const ACCENTS = [
	'red',
	'orange',
	'amber',
	'green',
	'teal',
	'cyan',
	'blue',
	'indigo',
	'purple',
	'rose'
] as const;

/** The theme accent the appearance picker offers. */
export type Accent = (typeof ACCENTS)[number];

/** Reports whether an accent name is one the picker still offers, so a choice saved before a
 *  rename is dropped instead of applied as a colour the stylesheet does not define. */
export function isAccent(value: string): value is Accent {
	return ACCENTS.some((accent) => accent === value);
}

/** How the shell resolves light and dark. */
export type ThemeChoice = 'system' | 'light' | 'dark';

/** Reports whether a theme name the daemon injected or a stored blob carried is one the shell
 *  still knows how to resolve. */
export function isThemeChoice(value: string): value is ThemeChoice {
	return value === 'system' || value === 'light' || value === 'dark';
}

/** The rail docked beside the page on a wide screen.
 *  Full shows icons and names; compact keeps the icon column. Neither mode hides the rail. */
export type SidebarMode = 'full' | 'compact';

/** How the connection, account, key, agent, and group grids pack: vertical is four cards on a
 *  wide screen, horizontal keeps three wider cards.
 *  A page shares one card size; vertical may differ from horizontal. */
export type CardLayout = 'vertical' | 'horizontal';

/** Reports whether a density the control handed back is one the grids pack by. */
export function isCardLayout(value: string): value is CardLayout {
	return value === 'vertical' || value === 'horizontal';
}

/** The grid class one card density lays out with. */
export function cardLayoutGrid(layout: CardLayout): string {
	return layout === 'horizontal'
		? 'grid gap-3 sm:grid-cols-2 xl:grid-cols-3'
		: 'grid gap-3 sm:grid-cols-2 xl:grid-cols-4';
}

/** Sizes a toolbar group to one card column of cardLayoutGrid: gap 0.75rem between tracks, 2
 *  tracks at sm, 3 or 4 at xl.
 *  100% resolves against the same container as the grid, so the group always matches one card's
 *  width. */
export function cardLayoutSlot(layout: CardLayout): string {
	return layout === 'horizontal'
		? 'sm:w-[calc((100%-0.75rem)/2)] xl:w-[calc((100%-2*0.75rem)/3)]'
		: 'sm:w-[calc((100%-0.75rem)/2)] xl:w-[calc((100%-3*0.75rem)/4)]';
}

/** Cards fill the grid cell so a row matches the tallest card on the page. */
export function cardLayoutCard(_layout: CardLayout): string {
	return 'flex h-full min-h-0 flex-col';
}

/** Stretches a divider across every column the grid is using. */
export function cardLayoutSpan(layout: CardLayout): string {
	return layout === 'horizontal' ? 'sm:col-span-2 xl:col-span-3' : 'sm:col-span-2 xl:col-span-4';
}

/** The appearance and console preferences the shell stores. */
export interface ConsoleSettings {
	theme: ThemeChoice;
	accent: Accent;
	cardLayout: CardLayout;
	sidebar: {
		mode: SidebarMode;
	};
}

const STORAGE_KEY = 'relo.console.v1';

/** What a first visit starts with. */
export const DEFAULT_SETTINGS: ConsoleSettings = {
	theme: 'system',
	accent: 'red',
	cardLayout: 'vertical',
	sidebar: {
		mode: 'compact'
	}
};

/** Reads the stored preferences, falling back to the defaults. */
export function loadSettings(): ConsoleSettings {
	const defaults = structuredClone(DEFAULT_SETTINGS);
	if (typeof window !== 'undefined' && window.__reloAppearance) {
		const injected = window.__reloAppearance.theme;
		// The daemon's own pre-paint script reads a theme it does not know as
		// light, so the console follows it rather than the OS: the two must not
		// paint the same page differently.
		defaults.theme = isThemeChoice(injected) ? injected : 'light';
		const accent = storedAccent(window.__reloAppearance.accent);
		if (accent) defaults.accent = accent;
	}
	if (typeof localStorage === 'undefined') {
		return defaults;
	}
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) {
			return defaults;
		}
		const stored: unknown = JSON.parse(raw);
		if (typeof stored !== 'object' || stored === null) {
			return defaults;
		}
		if ('theme' in stored || 'accent' in stored) {
			try {
				localStorage.setItem(STORAGE_KEY, JSON.stringify(storedLayout(stored)));
			} catch {
				// A blocked storage write does not prevent reading the layout.
			}
		}
		return mergeSettings(defaults, stored);
	} catch {
		return defaults;
	}
}

/** The two layout fields a stored blob carries, read without trusting the rest of its shape. */
function storedLayout(patch: object): { sidebar: unknown; cardLayout: unknown } {
	return {
		sidebar: 'sidebar' in patch ? patch.sidebar : undefined,
		cardLayout: 'cardLayout' in patch ? patch.cardLayout : undefined
	};
}

function storedSidebarMode(mode: string | undefined, docked: boolean | undefined): SidebarMode {
	if (mode === 'full' || mode === 'compact') return mode;
	if (mode === 'always' && docked !== false) return 'full';
	return 'compact';
}

function storedAccent(accent: string | undefined): Accent | undefined {
	if (accent === 'emerald') return 'green';
	if (accent === 'violet') return 'purple';
	if (accent === undefined) return undefined;
	return isAccent(accent) ? accent : undefined;
}

/** Folds a stored blob over the defaults, keeping only the values the console still applies. A
 *  field it no longer reads, or one that never held a value of its own kind, falls back. */
function mergeSettings(base: ConsoleSettings, patch: object): ConsoleSettings {
	const merged = structuredClone(base);
	if ('cardLayout' in patch) {
		const layout: unknown = patch.cardLayout;
		if (layout === 'vertical' || layout === 'horizontal') {
			merged.cardLayout = layout;
		}
	}
	if ('sidebar' in patch) {
		// mode is read on its own: a stored blob from before full and compact
		// existed carries the old beside/over choice, or a visible flag, and
		// neither of those is a shape the console applies any more.
		const sidebar: unknown = patch.sidebar;
		if (typeof sidebar === 'object' && sidebar !== null) {
			const mode = 'mode' in sidebar && typeof sidebar.mode === 'string' ? sidebar.mode : undefined;
			const docked =
				'docked' in sidebar && typeof sidebar.docked === 'boolean' ? sidebar.docked : undefined;
			merged.sidebar = { mode: storedSidebarMode(mode, docked) };
		}
	}
	return merged;
}

/** Stores the preferences. A refused write is not worth reporting. */
export function saveSettings(settings: ConsoleSettings): void {
	if (typeof localStorage === 'undefined') return;
	localStorage.setItem(
		STORAGE_KEY,
		JSON.stringify({
			sidebar: settings.sidebar,
			cardLayout: settings.cardLayout
		})
	);
}

/** Writes the resolved theme onto the document. */
export function applyTheme(theme: ThemeChoice): void {
	if (typeof document === 'undefined') return;
	const dark =
		theme === 'dark' ||
		(theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
	const root = document.documentElement;
	const apply = () => {
		root.classList.toggle('dark', dark);
		root.dataset.theme = dark ? 'dark' : 'light';
	};

	// The shell has already painted its saved theme. Mark the first sync as
	// ready without animating it, even if the shell and client briefly differ.
	if (!('themeReady' in root.dataset)) {
		apply();
		root.dataset.themeReady = '';
		return;
	}
	if (root.classList.contains('dark') === dark) return;

	if (
		typeof document.startViewTransition !== 'function' ||
		window.matchMedia('(prefers-reduced-motion: reduce)').matches
	) {
		apply();
		return;
	}

	const direction = dark ? 'sunset' : 'sunrise';
	root.dataset.themeDir = direction;
	const transition = document.startViewTransition(apply);
	const cleanup = () => {
		if (root.dataset.themeDir === direction) delete root.dataset.themeDir;
	};
	void transition.finished.then(cleanup, cleanup);
}

/** Writes the accent onto the document. */
export function applyAccent(accent: Accent): void {
	if (typeof document === 'undefined') return;
	document.documentElement.setAttribute('data-accent', accent);
}

/** Writes every preference that has a document effect. */
export function applySettings(settings: ConsoleSettings): void {
	applyTheme(settings.theme);
	applyAccent(settings.accent);
}

/** resolveSystemTheme watches the OS preference while the operator's choice is "system", so a
 *  change outside the console is followed live. */
export function watchSystemTheme(onChange: () => void): () => void {
	if (typeof window === 'undefined') return () => {};
	const query = window.matchMedia('(prefers-color-scheme: dark)');
	query.addEventListener('change', onChange);
	return () => query.removeEventListener('change', onChange);
}
