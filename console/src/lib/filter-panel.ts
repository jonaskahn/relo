// A page's filter panel opens and closes on its own, and which way it stood is
// remembered in the browser, so a visit lands on the layout the operator last
// chose. Nothing here is daemon state: two browsers on one machine keep their
// own choice.

/** The slice of localStorage this module uses, so a test can hand it a fake that throws the way
 *  a browser with storage denied does. */
export interface StorageLike {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
}

/** Names the page each settled state belongs to. */
export const FILTER_PANEL_KEYS = {
	usage: 'relo.filters.usage',
	logs: 'relo.filters.logs',
	logsDaemon: 'relo.filters.logs.daemon'
} as const;

/** LocalStorage where the console runs in a browser.
 *  A browser that blocks storage can throw on the property itself, which is the same condition
 *  as a refused write. */
export function browserStorage(): StorageLike | undefined {
	try {
		return typeof localStorage === 'undefined' ? undefined : localStorage;
	} catch {
		return undefined;
	}
}

/** Reports whether one page's filter panel is open.
 *  A page with no settled state opens closed, which is what keeps a panel from taking the screen
 *  on a first visit. */
export function readPanelOpen(storage: StorageLike | undefined, key: string): boolean {
	if (!storage) return false;
	try {
		return storage.getItem(key) === 'open';
	} catch {
		return false;
	}
}

/** Remembers one panel's state. A refused write is not worth reporting: the panel works either
 *  way, and the next visit opens closed. */
export function writePanelOpen(storage: StorageLike | undefined, key: string, open: boolean): void {
	if (!storage) return;
	try {
		storage.setItem(key, open ? 'open' : 'closed');
	} catch {
		// A browser that refuses storage keeps the panel working for this visit.
	}
}

/** Names the fields grid one toggle controls, so a split toolbar can wire its own toggle to a
 *  FilterPanel rendered with toolbar={false}. */
export function filterPanelId(storageKey: string): string {
	return 'filter-panel-' + storageKey.replace(/[^a-z0-9]+/gi, '-');
}
