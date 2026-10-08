import { beforeEach, describe, expect, it } from 'vitest';

import { consoleState } from './console-state.svelte';
import { navDrawer } from './nav-drawer.svelte';

// The drawer is one piece of shell state the rail, the top bar and the page
// frame all read, so it is checked here rather than through a component.

beforeEach(() => {
	navDrawer.setWide(true);
	navDrawer.setDrawerOnly(false);
	navDrawer.close();
	consoleState.setSidebar({ ...consoleState.settings.sidebar, mode: 'full' });
});

describe('the docked rail', () => {
	it('is pinned beside a wide page and named as the button target', () => {
		navDrawer.setWide(true);
		expect(navDrawer.pinned).toBe(true);
		expect(navDrawer.target).toBe('rail');
		expect(navDrawer.overlay).toBe(false);
	});

	it('gives way to the drawer below the breakpoint', () => {
		navDrawer.setWide(false);
		expect(navDrawer.pinned).toBe(false);
		expect(navDrawer.target).toBe('drawer');
		expect(navDrawer.compact).toBe(false);
	});

	it('crossing into the wide layout closes the overlay it replaces', () => {
		navDrawer.setWide(false);
		navDrawer.toggle();
		expect(navDrawer.overlay).toBe(true);
		navDrawer.setWide(true);
		expect(navDrawer.overlay).toBe(false);
	});

	it('is never pinned when the page asked for the overlay and nothing else', () => {
		navDrawer.setDrawerOnly(true);
		expect(navDrawer.pinned).toBe(false);
		expect(navDrawer.target).toBe('drawer');
	});
});

describe('the workspace rail', () => {
	it('starts compact on a page that asks for it, and expands for that page', () => {
		navDrawer.setWorkspaceAutoCompact(() => true);
		expect(navDrawer.compact).toBe(true);

		navDrawer.toggle();
		expect(navDrawer.compact).toBe(false);

		navDrawer.toggle();
		expect(navDrawer.compact).toBe(true);
	});

	it('reads the page flag live rather than a copy taken when it was set', () => {
		let workspace = true;
		navDrawer.setWorkspaceAutoCompact(() => workspace);
		expect(navDrawer.compact).toBe(true);

		workspace = false;
		expect(navDrawer.compact).toBe(false);
	});

	it('leaves a page that does not ask for it alone', () => {
		navDrawer.setWorkspaceAutoCompact(() => false);
		expect(navDrawer.compact).toBe(false);
	});

	it('keeps the saved compact menu compact whichever page asks', () => {
		consoleState.setSidebar({ ...consoleState.settings.sidebar, mode: 'compact' });
		navDrawer.setWorkspaceAutoCompact(() => false);
		expect(navDrawer.compact).toBe(true);
	});
});
