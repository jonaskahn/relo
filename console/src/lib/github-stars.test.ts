import { describe, expect, it } from 'vitest';

import { formatStars, loadStars, STAR_URL } from './github-stars';

class Memory {
	data = new Map<string, string>();
	getItem(key: string) {
		return this.data.get(key) ?? null;
	}
	setItem(key: string, value: string) {
		this.data.set(key, value);
	}
}

const answer = (body: unknown, ok = true) =>
	(async () => ({ ok, json: async () => body })) as unknown as typeof fetch;

describe('loadStars', () => {
	it('reads the count and remembers it', async () => {
		const storage = new Memory();
		expect(await loadStars(answer({ stargazers_count: 42 }), storage, 1000)).toBe(42);
		expect(storage.getItem('relo.githubStars')).toBe('{"count":42,"at":1000}');
	});

	it('serves a recent count without asking again', async () => {
		const storage = new Memory();
		await loadStars(answer({ stargazers_count: 42 }), storage, 1000);
		const never = (async () => {
			throw new Error('asked');
		}) as unknown as typeof fetch;
		expect(await loadStars(never, storage, 1000 + 59 * 60 * 1000)).toBe(42);
	});

	it('asks again once the count is an hour old', async () => {
		const storage = new Memory();
		await loadStars(answer({ stargazers_count: 42 }), storage, 1000);
		expect(await loadStars(answer({ stargazers_count: 43 }), storage, 1000 + 61 * 60 * 1000)).toBe(
			43
		);
	});

	it('falls back to the last count when GitHub cannot be reached', async () => {
		const storage = new Memory();
		await loadStars(answer({ stargazers_count: 42 }), storage, 1000);
		const offline = (async () => {
			throw new Error('offline');
		}) as unknown as typeof fetch;
		expect(await loadStars(offline, storage, 1000 + 5 * 60 * 60 * 1000)).toBe(42);
		expect(await loadStars(answer({}, false), storage, 1000 + 5 * 60 * 60 * 1000)).toBe(42);
	});

	it('is null when there is nothing to show', async () => {
		expect(await loadStars(answer({}, false), new Memory(), 1)).toBeNull();
		expect(await loadStars(answer({ stargazers_count: 'many' }), new Memory(), 1)).toBeNull();
		expect(await loadStars(answer({ stargazers_count: -1 }), undefined, 1)).toBeNull();
	});
});

describe('formatStars', () => {
	it('shortens a large count for the language', () => {
		expect(formatStars(950, 'en')).toBe('950');
		expect(formatStars(1234, 'en')).toBe('1.2K');
	});
});

describe('STAR_URL', () => {
	it('points at the repository and says where the visit came from', () => {
		const url = new URL(STAR_URL);
		expect(url.origin + url.pathname).toBe('https://github.com/jonaskahn/relo');
		expect(url.searchParams.get('ref')).toBe('relo-console');
	});
});
