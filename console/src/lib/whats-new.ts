import { browserStorage, type StorageLike } from './filter-panel';

/** Browser memory of the last version whose release notes were shown.
 *  Per browser like the filter panels: a second browser or cleared site data
 *  shows the notes once more. */
export const WHATS_NEW_KEY = 'relo.whatsNew.seen';

/** Strips a leading v and build metadata so the same release keeps one key
 *  across rebuilds and across /status vs. catalog spellings. */
export function releaseVersionOf(raw: string): string {
	return raw.trim().replace(/^v/, '').split('+')[0];
}

/** Reports whether the console opened on a version whose notes it has not
 *  shown in this browser. A version with no notes never shows, so a later
 *  open with notes still does. */
export function shouldShowWhatsNew(
	storage: StorageLike | undefined,
	notice: { current: string; current_notes?: string } | null | undefined
): boolean {
	if (!notice?.current || !notice.current_notes?.trim()) return false;
	if (!storage) return true;
	try {
		return storage.getItem(WHATS_NEW_KEY) !== releaseVersionOf(notice.current);
	} catch {
		return true;
	}
}

/** Remembers the version whose notes were just shown. A refused write means
 *  the next open shows them again, which the dialog tolerates. */
export function markWhatsNewSeen(storage: StorageLike | undefined, version: string): void {
	if (!storage || !version) return;
	try {
		storage.setItem(WHATS_NEW_KEY, releaseVersionOf(version));
	} catch {
		// Storage denied: this visit still closes the dialog for good.
	}
}

export { browserStorage };
