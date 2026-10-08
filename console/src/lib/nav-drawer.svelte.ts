// The shell shows its navigation as a rail docked beside the page or as a
// drawer opened over it, and the top bar is the one control that switches
// between them. That state lives here rather than being threaded through the
// shell, because the sidebar, the top bar, and the page frame all read it.
// Closing the drawer hands focus back to the button that opened it, which is
// what keeps a keyboard from being dropped on the page behind the scrim.

import { consoleState } from './console-state.svelte';
import { isOverlayOpen, isPinned, sidebarCompact, toggleTarget } from './nav-shell';

function browserWide(): boolean {
	return typeof window !== 'undefined' && window.matchMedia('(min-width: 1024px)').matches;
}

class NavDrawer {
	/** The control that opened the drawer, which is where focus goes back. */
	trigger = $state<HTMLElement | null>(null);
	/** True while the overlay drawer is over the page. */
	drawerOpen = $state(false);
	/** True when a page asked for the overlay drawer and nothing else. */
	drawerOnly = $state(false);
	/** The dock breakpoint: 1024px, where a rail has room to sit beside the page
	 *  instead of over it. */
	wide = $state(browserWide());
	/** True while the operator has expanded the rail of an auto-compacting page
	 *  for that page. */
	workspaceMenuExpanded = $state(false);

	// A workspace page, such as Chat, keeps the console rail docked but starts
	// in the icon column when the saved menu is full. The page hands over a
	// reader rather than a copy, so the rail follows the page it is on without
	// a second value to keep in step.
	#workspaceAutoCompact: () => boolean = () => false;

	get pinned() {
		return isPinned(this.wide, this.drawerOnly);
	}

	/** compact is the icon column. Full and compact are both docked; the rail
	 *  stays beside the page either way. */
	get compact() {
		if (!this.pinned) return false;
		return sidebarCompact(
			consoleState.settings.sidebar.mode,
			this.#workspaceAutoCompact(),
			this.workspaceMenuExpanded
		);
	}

	/** target names the surface the top bar button controls right now, which is
	 *  its label as much as its behaviour. */
	get target() {
		return toggleTarget(this.wide, this.drawerOnly);
	}

	get overlay() {
		return isOverlayOpen(this.drawerOpen, this.wide, this.drawerOnly);
	}

	/** setDrawerOnly is the page telling the shell what it wants. A drawer that
	 *  was open when the page changed is closed with it, because the next page
	 *  never asked for it. */
	setDrawerOnly(next: boolean) {
		if (next === this.drawerOnly) return;
		this.drawerOnly = next;
		this.drawerOpen = false;
	}

	/** setWorkspaceAutoCompact hands over the page's own flag, read live. */
	setWorkspaceAutoCompact(read: () => boolean) {
		this.#workspaceAutoCompact = read;
	}

	/** setWide follows the dock breakpoint. Crossing into the wide layout closes
	 *  the overlay, which the docked rail replaces. */
	setWide(next: boolean) {
		if (next === this.wide) return;
		this.wide = next;
		if (next) this.drawerOpen = false;
	}

	close() {
		this.drawerOpen = false;
	}

	/** toggle acts on whichever surface the button controls at this size. On a
	 *  wide screen it switches the docked rail between full and compact. */
	toggle() {
		if (toggleTarget(this.wide, this.drawerOnly) === 'drawer') {
			this.drawerOpen = !this.drawerOpen;
			return;
		}
		if (this.#workspaceAutoCompact() && consoleState.settings.sidebar.mode === 'full') {
			this.workspaceMenuExpanded = !this.workspaceMenuExpanded;
			return;
		}
		consoleState.setSidebar({
			...consoleState.settings.sidebar,
			mode: consoleState.settings.sidebar.mode === 'full' ? 'compact' : 'full'
		});
	}
}

/** The rail's dock state. */
export const navDrawer = new NavDrawer();
