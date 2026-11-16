// The repository's star count, shown beside the link that lets an operator add
// one. It is a public number read straight from GitHub, so nothing about the
// operator or their setup is sent. A failed read, an offline machine or a
// blocked store just leaves the count out; the link still works.
import { browserStorage, type StorageLike } from '$lib/filter-panel';

export const REPO = 'jonaskahn/relo';

// GitHub has no address that stars a repository for you: starring needs the
// operator signed in on the repository page. The parameter only tells the page
// the visit came from the console.
export const STAR_URL = `https://github.com/${REPO}?ref=relo-console`;

const API_URL = `https://api.github.com/repos/${REPO}`;
const CACHE_KEY = 'relo.githubStars';
// GitHub allows 60 unauthenticated reads an hour per address, so one read an
// hour is plenty for a number that moves slowly.
const MAX_AGE_MS = 60 * 60 * 1000;

interface Cached {
	count: number;
	at: number;
}

function readCache(storage: StorageLike | undefined): Cached | null {
	if (!storage) return null;
	try {
		const raw = storage.getItem(CACHE_KEY);
		if (!raw) return null;
		const value: unknown = JSON.parse(raw);
		if (
			value &&
			typeof value === 'object' &&
			typeof (value as Cached).count === 'number' &&
			typeof (value as Cached).at === 'number'
		) {
			return value as Cached;
		}
	} catch {
		// An unreadable cache is the same as none.
	}
	return null;
}

function writeCache(storage: StorageLike | undefined, entry: Cached) {
	try {
		storage?.setItem(CACHE_KEY, JSON.stringify(entry));
	} catch {
		// A refused write only costs the next read a request.
	}
}

/** The star count, or null when it cannot be told.
 *  A recent cached count is returned without a request; when the request fails the last
 *  count seen is returned, however old, because a slightly stale number beats none. */
export async function loadStars(
	fetcher: typeof fetch = fetch,
	storage: StorageLike | undefined = browserStorage(),
	now: number = Date.now()
): Promise<number | null> {
	const cached = readCache(storage);
	if (cached && now - cached.at >= 0 && now - cached.at < MAX_AGE_MS) return cached.count;
	try {
		const response = await fetcher(API_URL, { headers: { Accept: 'application/vnd.github+json' } });
		if (!response.ok) return cached?.count ?? null;
		const body: unknown = await response.json();
		const count = (body as { stargazers_count?: unknown })?.stargazers_count;
		if (typeof count !== 'number' || !Number.isFinite(count) || count < 0) {
			return cached?.count ?? null;
		}
		writeCache(storage, { count, at: now });
		return count;
	} catch {
		return cached?.count ?? null;
	}
}

/** A count as a short label for the current language: 1,234 reads 1.2K. */
export function formatStars(count: number, locale: string): string {
	try {
		return new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 }).format(
			count
		);
	} catch {
		return String(count);
	}
}

// Below this many stars the link wears a sad face, which is the nudge to add one.
export const SAD_BELOW = 200;

export function isSad(count: number | null): boolean {
	return count !== null && count < SAD_BELOW;
}
