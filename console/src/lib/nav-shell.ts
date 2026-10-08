// The console shows its navigation as a rail docked beside the page on a wide
// screen, or as a drawer opened over the page. These are the decisions that
// choose between the two, kept out of the components so they can be checked
// directly. Full and compact are both docked; the rail is not hidden.

/** The surface the top bar button controls at one size. */
export type NavSurface = 'rail' | 'drawer';

/** Names that surface: a wide screen switches the docked rail between full and compact, and
 *  every other case opens the overlay drawer. */
export function toggleTarget(wide: boolean, drawerOnly: boolean): NavSurface {
	return wide && !drawerOnly ? 'rail' : 'drawer';
}

/** Whether the rail is docked beside the page, taking its width from the content rather than
 *  floating over it. */
export function isPinned(wide: boolean, drawerOnly: boolean): boolean {
	return wide && !drawerOnly;
}

/** Whether the drawer is showing over the page. A docked rail never overlays. */
export function isOverlayOpen(drawerOpen: boolean, wide: boolean, drawerOnly: boolean): boolean {
	return drawerOpen && !isPinned(wide, drawerOnly);
}

/** Marks the page that fills the viewport and scrolls its own panes.
 *  Chat is the one such page: the thread rail and the conversation scroll apart from each other
 *  rather than with the shell. */
export function isWorkspaceRoute(pathname: string): boolean {
	return pathname === '/chat';
}

/** The docked rail width on a wide screen. Chat keeps the rail visible but starts in the icon
 *  column when the saved menu is full; the operator can expand it for that visit without
 *  changing settings. */
export function sidebarCompact(
	mode: 'full' | 'compact',
	workspaceAutoCompact: boolean,
	workspaceMenuExpanded: boolean
): boolean {
	if (mode === 'compact') return true;
	return workspaceAutoCompact && !workspaceMenuExpanded;
}
