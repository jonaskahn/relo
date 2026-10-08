// The one reactive holder for the console's settable state. Components
// read consoleState.settings and write through its methods. Layout persists
// locally; appearance is saved to the daemon's shared config.

import { toast } from 'svelte-sonner';
import { api } from './api';
import { CARD_MORPH_MS, captureCardBoxes, playCardMorph } from './card-layout-motion';
import type { QuotaDisplay } from './types';

import {
	applySettings,
	loadSettings,
	saveSettings,
	watchSystemTheme,
	DEFAULT_SETTINGS,
	type Accent,
	type CardLayout,
	type ConsoleSettings,
	type ThemeChoice
} from './theme.svelte';

function createConsoleState() {
	const initial = loadSettings();
	let settings = $state<ConsoleSettings>(initial);
	// Appearance is what the daemon stores in config.toml: theme and accent,
	// which the shell also caches locally so it paints without a flash, and
	// how a quota chart reads, which only the daemon owns.
	type Appearance = { theme: ThemeChoice; accent: Accent; quota_display: QuotaDisplay };
	let quotaDisplay = $state<QuotaDisplay>('used');
	let appearanceQueue = Promise.resolve();
	let appearanceRevision = 0;
	// cardLayoutAnimating marks the moment the layout control re-solves the card
	// grid, so the toolbar pair eases to the new card width only then: a window
	// resize has to track its container exactly, with no transition to lag it.
	let cardLayoutAnimating = $state(false);
	let cardLayoutAnimation: ReturnType<typeof setTimeout> | undefined;

	applySettings(initial);
	watchSystemTheme(() => {
		if (settings.theme === 'system') {
			applySettings(settings);
		}
	});

	function commit(next: ConsoleSettings) {
		settings = next;
		saveSettings(next);
		applySettings(next);
	}

	function showAppearance(next: Partial<Appearance>) {
		if (next.theme !== undefined || next.accent !== undefined) {
			settings = {
				...settings,
				theme: next.theme ?? settings.theme,
				accent: next.accent ?? settings.accent
			};
			applySettings(settings);
		}
		if (next.quota_display !== undefined) quotaDisplay = next.quota_display;
	}

	function saveAppearance(patch: Partial<Appearance>) {
		appearanceRevision++;
		// Every field of the patch is put back from where it was when the write
		// failed, unless a later choice already replaced it.
		const previous: Appearance = {
			theme: settings.theme,
			accent: settings.accent,
			quota_display: quotaDisplay
		};
		showAppearance(patch);
		appearanceQueue = appearanceQueue.then(async () => {
			try {
				// The console keeps what the operator chose rather than reading
				// it back, so a second choice made while this write is in flight
				// is not undone by this one's answer.
				await api('/settings/appearance', {
					method: 'PATCH',
					body: JSON.stringify(patch)
				});
			} catch (error) {
				if (patch.theme !== undefined && settings.theme === patch.theme) {
					showAppearance({ theme: previous.theme });
				}
				if (patch.accent !== undefined && settings.accent === patch.accent) {
					showAppearance({ accent: previous.accent });
				}
				if (patch.quota_display !== undefined && quotaDisplay === patch.quota_display) {
					showAppearance({ quota_display: previous.quota_display });
				}
				toast.error(error instanceof Error ? error.message : 'Could not save appearance');
			}
		});
		return appearanceQueue;
	}

	return {
		get settings() {
			return settings;
		},
		get appearanceRevision() {
			return appearanceRevision;
		},
		setTheme(theme: ThemeChoice) {
			return saveAppearance({ theme });
		},
		setAccent(accent: Accent) {
			return saveAppearance({ accent });
		},
		setQuotaDisplay(quota_display: QuotaDisplay) {
			return saveAppearance({ quota_display });
		},
		useAppearance(appearance: Appearance, revision = appearanceRevision) {
			if (revision !== appearanceRevision) return;
			showAppearance(appearance);
		},
		async loadAppearance() {
			const revision = appearanceRevision;
			const saved = await api<{ appearance: Appearance }>('/settings');
			if (revision !== appearanceRevision) return;
			showAppearance(saved.appearance);
		},
		get quotaDisplay() {
			return quotaDisplay;
		},
		setSidebar(sidebar: ConsoleSettings['sidebar']) {
			commit({ ...settings, sidebar });
		},
		setCardLayout(cardLayout: CardLayout) {
			// The cards are measured before the class flips, so every surface
			// with the control morphs its grid without a page knowing about it.
			const before = captureCardBoxes();
			commit({ ...settings, cardLayout });
			cardLayoutAnimating = true;
			clearTimeout(cardLayoutAnimation);
			cardLayoutAnimation = setTimeout(() => (cardLayoutAnimating = false), CARD_MORPH_MS + 20);
			void playCardMorph(before);
		},
		get cardLayoutAnimating() {
			return cardLayoutAnimating;
		},
		reset() {
			commit({
				...structuredClone(DEFAULT_SETTINGS),
				theme: settings.theme,
				accent: settings.accent
			});
		}
	};
}

/** The appearance and console settings the shell shares. */
export const consoleState = createConsoleState();
