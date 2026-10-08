// The navigation catalog: one source of truth for the sidebar, the
// command palette, and the top bar's quick-add actions.

import type { IconName } from '$lib/components/ui/icon.svelte';

/** One entry the rail, the palette and the top bar share. */
export interface NavLink {
	id: string;
	href: string;
	labelKey: string;
	// icon is the glyph the sidebar and the command palette show beside the
	// label, so a page is recognised by shape before its name is read.
	icon: IconName;
}

/** A labelled group of links in the rail. */
export interface NavSection {
	id: string;
	labelKey: string;
	links: NavLink[];
}

/** Names the current page the way a menu marks it: the root is only ever the root, and every
 *  other page owns the paths beneath it. */
export function isNavLinkActive(href: string, pathname: string): boolean {
	if (href === '/') return pathname === '/';
	return pathname === href || pathname.startsWith(href + '/');
}

/** The navigation catalog, in the order the rail shows it. */
export const NAV_SECTIONS: NavSection[] = [
	{
		id: 'connectivity',
		labelKey: 'ui.sidebar.connectivity',
		links: [
			{
				id: 'connections',
				href: '/connections',
				labelKey: 'ui.sidebar.connections',
				icon: 'plug-connected'
			},
			{ id: 'groups', href: '/groups', labelKey: 'ui.sidebar.groups', icon: 'route' },
			{ id: 'keys', href: '/keys', labelKey: 'ui.sidebar.keys', icon: 'key' }
		]
	},
	{
		id: 'integrations',
		labelKey: 'ui.sidebar.integrations',
		links: [
			{ id: 'integrations', href: '/agents', labelKey: 'ui.sidebar.agents', icon: 'layers' },
			{ id: 'chat', href: '/chat', labelKey: 'ui.sidebar.chat', icon: 'message-square' }
		]
	},
	{
		id: 'activity',
		labelKey: 'ui.sidebar.activity',
		links: [
			{ id: 'dashboard', href: '/', labelKey: 'ui.sidebar.dashboard', icon: 'layout-dashboard' },
			{ id: 'logs', href: '/logs', labelKey: 'ui.sidebar.logs', icon: 'list-details' },
			{ id: 'usage', href: '/usage', labelKey: 'ui.sidebar.usage', icon: 'chart-bar' }
		]
	},
	{
		id: 'settings',
		labelKey: 'ui.sidebar.console',
		links: [
			{ id: 'settings', href: '/settings', labelKey: 'ui.sidebar.settings', icon: 'device-desktop' }
		]
	}
];
