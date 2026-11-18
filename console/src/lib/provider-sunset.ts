/** Lists the connections the vendor has closed to a third-party client. Google disabled Cloud
 *  Code Assist for accounts it finds behind one, so an Antigravity connection is refused with a
 *  terms-of-service violation whatever the account's quota is. The connection ships switched off;
 *  this names why for an operator who switches it back on. */
const SUNSET_IDS = new Set(['google-antigravity']);

/** Reports whether one connection is closed at the vendor. */
export function isSunsetConnection(id: string): boolean {
	return SUNSET_IDS.has(id);
}

/** The banner a closed connection wears while it is switched on, empty for every other
 *  connection. */
export function sunsetNoticeKey(id: string): string {
	return isSunsetConnection(id) ? 'ui.pages.providersPage.banner.sunset' : '';
}
