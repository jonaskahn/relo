import { describe, expect, test } from 'vitest';

import {
	addConnectionHref,
	connectionsViewQuery,
	linkedProviderId,
	readConnectionsView,
	type ConnectionsView
} from './connections-view';

describe('the connections view a query string describes', () => {
	const cases: { name: string; search: string; want: ConnectionsView }[] = [
		{
			name: 'opens the list when the query names nothing',
			search: '',
			want: { view: 'provider', tab: 'models', selectedId: '', paneOpen: false, addOpen: false }
		},
		{
			name: 'keeps a remembered tab on the list',
			search: '?tab=accounts',
			want: { view: 'provider', tab: 'accounts', selectedId: '', paneOpen: false, addOpen: false }
		},
		{
			name: 'opens the pane a provider names',
			search: '?provider=openai',
			want: {
				view: 'provider',
				tab: 'models',
				selectedId: 'openai',
				paneOpen: true,
				addOpen: false
			}
		},
		{
			name: 'opens the pane on the tab a provider names',
			search: '?provider=openai&tab=accounts',
			want: {
				view: 'provider',
				tab: 'accounts',
				selectedId: 'openai',
				paneOpen: true,
				addOpen: false
			}
		},
		{
			name: 'opens the pane the all-models view names',
			search: '?view=models',
			want: { view: 'models', tab: 'models', selectedId: '', paneOpen: true, addOpen: false }
		},
		{
			name: 'reads the add flag a quick add sets',
			search: '?provider=openai&add=1',
			want: { view: 'provider', tab: 'models', selectedId: 'openai', paneOpen: true, addOpen: true }
		},
		{
			name: 'decodes a provider id that carries a slash',
			search: '?provider=a%2Fb',
			want: { view: 'provider', tab: 'models', selectedId: 'a/b', paneOpen: true, addOpen: false }
		}
	];

	for (const tt of cases) {
		test(tt.name, () => {
			expect(readConnectionsView(tt.search)).toEqual(tt.want);
		});
	}
});

describe('the query string one state renders', () => {
	const cases: { name: string; state: ConnectionsView; query: string }[] = [
		{
			name: 'names nothing on the plain list',
			state: { view: 'provider', tab: 'models', selectedId: '', paneOpen: false, addOpen: false },
			query: ''
		},
		{
			name: 'keeps the tab of a list an operator left',
			state: { view: 'provider', tab: 'accounts', selectedId: '', paneOpen: false, addOpen: false },
			query: 'tab=accounts'
		},
		{
			name: 'names the open provider',
			state: {
				view: 'provider',
				tab: 'models',
				selectedId: 'openai',
				paneOpen: true,
				addOpen: false
			},
			query: 'provider=openai'
		},
		{
			name: 'names a provider and its tab',
			state: {
				view: 'provider',
				tab: 'settings',
				selectedId: 'openai',
				paneOpen: true,
				addOpen: false
			},
			query: 'provider=openai&tab=settings'
		},
		{
			name: 'names the all-models view',
			state: { view: 'models', tab: 'models', selectedId: '', paneOpen: true, addOpen: false },
			query: 'view=models'
		},
		{
			name: 'names the add flow over an open provider',
			state: {
				view: 'provider',
				tab: 'accounts',
				selectedId: 'openai',
				paneOpen: true,
				addOpen: true
			},
			query: 'provider=openai&tab=accounts&add=1'
		}
	];

	for (const tt of cases) {
		test(tt.name, () => {
			expect(connectionsViewQuery(tt.state)).toBe(tt.query);
			expect(readConnectionsView('?' + tt.query)).toEqual(tt.state);
		});
	}

	test('drops the provider of a closed pane', () => {
		const query = connectionsViewQuery({
			view: 'provider',
			tab: 'accounts',
			selectedId: 'openai',
			paneOpen: false,
			addOpen: false
		});
		expect(query).toBe('tab=accounts');
		expect(readConnectionsView('?' + query).paneOpen).toBe(false);
	});
});

describe('the provider a deep link names', () => {
	test('keeps the linked id until the connection list has loaded', () => {
		expect(linkedProviderId('anthropic', [], false)).toBe('anthropic');
	});

	test('keeps a known id once the list has loaded', () => {
		expect(linkedProviderId('anthropic', ['openai', 'anthropic'], true)).toBe('anthropic');
	});

	test('falls back to the first connection when the linked id is absent', () => {
		expect(linkedProviderId('missing', ['openai', 'anthropic'], true)).toBe('openai');
		expect(linkedProviderId('missing', [], true)).toBe('');
	});

	test('leaves an unnamed visit unnamed', () => {
		expect(linkedProviderId('', ['openai'], false)).toBe('');
		expect(linkedProviderId('', ['openai'], true)).toBe('');
	});
});

describe('the quick add href', () => {
	test('lands on the connections page when triggered from elsewhere', () => {
		expect(addConnectionHref('/', '')).toBe('/connections?add=1');
		expect(addConnectionHref('/logs', '?page=2')).toBe('/connections?add=1');
	});

	test('keeps the view already open on the connections page', () => {
		expect(addConnectionHref('/connections', '')).toBe('/connections?add=1');
		expect(addConnectionHref('/connections', '?provider=openai&tab=accounts')).toBe(
			'/connections?provider=openai&tab=accounts&add=1'
		);
	});
});
