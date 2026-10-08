// The Connections page keeps its view in the URL, so a link to one provider is
// a link to its models and a reload lands where the operator left. The narrow
// layout reads the same state to decide which of its two panes is on screen,
// which is what paneOpen reports.

/** Which of a connections page's two views a query string names. */
export type ConnectionsViewName = 'provider' | 'models';

/** The state one connections query string describes. */
export interface ConnectionsView {
	view: ConnectionsViewName;
	tab: string;
	selectedId: string;
	// paneOpen is whether the detail pane is the one an operator is looking at.
	// A query that names a provider or the all-models view opens it; anything
	// else opens the list.
	paneOpen: boolean;
	addOpen: boolean;
}

/** Reads the state one query string describes. */
export function readConnectionsView(search: string): ConnectionsView {
	const params = new URLSearchParams(search);
	const view: ConnectionsViewName = params.get('view') === 'models' ? 'models' : 'provider';
	const selectedId = params.get('provider') ?? '';
	return {
		view,
		tab: params.get('tab') ?? 'models',
		selectedId,
		paneOpen: view === 'models' || selectedId !== '',
		addOpen: params.get('add') === '1'
	};
}

/** Renders the query string for one state, without the question mark and without the parameters
 *  the state does not carry. */
export function connectionsViewQuery(state: ConnectionsView): string {
	const params = new URLSearchParams();
	if (state.paneOpen) {
		if (state.view === 'models') {
			params.set('view', 'models');
		} else if (state.selectedId !== '') {
			params.set('provider', state.selectedId);
		}
	}
	if (state.tab !== 'models') params.set('tab', state.tab);
	if (state.addOpen) params.set('add', '1');
	return params.toString();
}

/** Keeps a deep link until the connection list is known.
 *  An unloaded list is not a missing connection, so the id stays.
 *  Once the list has loaded, a named connection that is absent falls back to the first one. A
 *  visit that names none stays unnamed. */
export function linkedProviderId(
	selectedId: string,
	providerIds: readonly string[],
	listLoaded: boolean
): string {
	if (selectedId === '' || !listLoaded) return selectedId;
	if (providerIds.includes(selectedId)) return selectedId;
	return providerIds[0] ?? '';
}

/** Where a quick add lands. It keeps the view already open when it is triggered from the
 *  connections page itself, so the flow opens over the provider an operator was reading. */
export function addConnectionHref(pathname: string, search: string): string {
	const params = new URLSearchParams(pathname === '/connections' ? search : '');
	params.set('add', '1');
	return '/connections?' + params.toString();
}
