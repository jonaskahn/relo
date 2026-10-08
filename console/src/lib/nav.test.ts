import { describe, expect, it } from 'vitest';

import { NAV_SECTIONS, isNavLinkActive } from './nav';

describe('NAV_SECTIONS', () => {
	it('offers the agents and the chat page inside the integrations section', () => {
		const section = NAV_SECTIONS.find((entry) => entry.id === 'integrations');
		expect(section?.links.map((link) => link.href)).toEqual(['/agents', '/chat']);
		expect(section?.links[0].labelKey).toBe('ui.sidebar.agents');
		expect(section?.links[1].labelKey).toBe('ui.sidebar.chat');
	});
});

describe('isNavLinkActive', () => {
	it('marks only the root for the dashboard', () => {
		expect(isNavLinkActive('/', '/')).toBe(true);
		expect(isNavLinkActive('/', '/logs')).toBe(false);
	});

	it('marks a section and the pages beneath it', () => {
		expect(isNavLinkActive('/connections', '/connections')).toBe(true);
		expect(isNavLinkActive('/connections', '/connections/openai')).toBe(true);
		expect(isNavLinkActive('/groups', '/groups')).toBe(true);
	});

	it('does not mark a page that merely starts with the same letters', () => {
		expect(isNavLinkActive('/groups', '/groupsomething')).toBe(false);
		expect(isNavLinkActive('/keys', '/keys-archive')).toBe(false);
	});

	it('marks nothing on an unrelated page', () => {
		expect(isNavLinkActive('/settings', '/usage')).toBe(false);
	});
});
