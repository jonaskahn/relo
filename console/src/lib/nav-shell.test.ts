import { afterEach, describe, expect, it, vi } from 'vitest';

import {
	isOverlayOpen,
	isPinned,
	isWorkspaceRoute,
	sidebarCompact,
	toggleTarget
} from './nav-shell';
import { loadSettings } from './theme.svelte';

// The top bar carries one button, and what it does depends on the size of the
// screen and whether the page asked for the overlay. These are those decisions.
describe('toggleTarget', () => {
	it('switches the rail on a wide screen', () => {
		expect(toggleTarget(true, false)).toBe('rail');
	});

	it('does not reach for the rail on a narrow screen', () => {
		expect(toggleTarget(false, false)).toBe('drawer');
	});

	it('opens the drawer when the page asked for the overlay', () => {
		expect(toggleTarget(true, true)).toBe('drawer');
		expect(toggleTarget(false, true)).toBe('drawer');
	});
});

describe('isPinned', () => {
	it('docks the rail on a wide screen', () => {
		expect(isPinned(true, false)).toBe(true);
	});

	it('stays off the page on a narrow screen and when the page asked for the overlay', () => {
		expect(isPinned(false, false)).toBe(false);
		expect(isPinned(true, true)).toBe(false);
		expect(isPinned(false, true)).toBe(false);
	});
});

// The workspace page scrolls its own panes, which is also the page whose rail
// may auto-compact beside it.
describe('isWorkspaceRoute', () => {
	it('marks chat as the workspace page', () => {
		expect(isWorkspaceRoute('/chat')).toBe(true);
	});

	it('leaves every other page to the shell', () => {
		expect(isWorkspaceRoute('/')).toBe(false);
		expect(isWorkspaceRoute('/usage')).toBe(false);
		expect(isWorkspaceRoute('/chatting')).toBe(false);
	});
});

describe('sidebarCompact', () => {
	it('follows the saved compact menu', () => {
		expect(sidebarCompact('compact', false, false)).toBe(true);
		expect(sidebarCompact('compact', true, true)).toBe(true);
	});

	it('auto-compacts a full menu on a workspace page until expanded', () => {
		expect(sidebarCompact('full', true, false)).toBe(true);
		expect(sidebarCompact('full', true, true)).toBe(false);
		expect(sidebarCompact('full', false, false)).toBe(false);
	});
});

describe('isOverlayOpen', () => {
	it('shows the drawer over the page wherever it was opened', () => {
		expect(isOverlayOpen(true, false, false)).toBe(true);
		expect(isOverlayOpen(true, true, true)).toBe(true);
		expect(isOverlayOpen(true, false, true)).toBe(true);
	});

	it('never overlays a docked rail', () => {
		expect(isOverlayOpen(true, true, false)).toBe(false);
	});

	it('stays closed when nothing asked for it', () => {
		expect(isOverlayOpen(false, false, false)).toBe(false);
		expect(isOverlayOpen(false, true, false)).toBe(false);
	});
});

// A browser that stored the console settings before full and compact existed
// lands on the icon column, unless it had asked for the expanded rail.
describe('settings stored before full and compact', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('resolves a missing mode to the icon column', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => JSON.stringify({ sidebar: { visible: false } })
		});
		const settings = loadSettings();
		expect(settings.sidebar.mode).toBe('compact');
		expect(settings.sidebar).not.toHaveProperty('docked');
	});

	it('keeps an expanded beside-the-page rail as full', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => JSON.stringify({ sidebar: { mode: 'always', docked: true } })
		});
		expect(loadSettings().sidebar.mode).toBe('full');
	});

	it('turns a collapsed rail into the icon column', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => JSON.stringify({ sidebar: { mode: 'always', docked: false } })
		});
		expect(loadSettings().sidebar.mode).toBe('compact');
	});

	it('turns the old overlay choice into the icon column', () => {
		vi.stubGlobal('localStorage', {
			getItem: () => JSON.stringify({ sidebar: { mode: 'auto', docked: true } })
		});
		expect(loadSettings().sidebar.mode).toBe('compact');
	});

	it('defaults to compact when nothing is stored', () => {
		vi.stubGlobal('localStorage', { getItem: () => null });
		expect(loadSettings().sidebar.mode).toBe('compact');
	});
});
