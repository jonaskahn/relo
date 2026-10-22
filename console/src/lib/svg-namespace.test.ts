import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// Svelte reads the namespace of a snippet from the element the snippet starts
// with. Four tags live in HTML as well as in a drawing, and a snippet that
// starts with one of them is drawn as HTML: the shapes and the text of a map
// never appear, and nothing in the server-rendered markup says so.
const SHARED_TAGS = ['a', 'script', 'style', 'title'];

function files(dir: string): string[] {
	const out: string[] = [];
	for (const name of readdirSync(dir)) {
		const path = join(dir, name);
		if (statSync(path).isDirectory()) {
			out.push(...files(path));
			continue;
		}
		if (name.endsWith('.svelte')) out.push(path);
	}
	return out;
}

describe('the namespace of every snippet', () => {
	it('never starts a snippet with a tag a drawing and a page share', () => {
		const seen = new Set<string>();

		for (const path of files('src')) {
			const text = readFileSync(path, 'utf8');
			for (const match of text.matchAll(/\{#snippet\s[^}]*\}([\s\S]*?)\{\/snippet\}/g)) {
				const first = /^\s*<([a-zA-Z-]+)/.exec(match[1]);
				if (!first) continue;
				seen.add(first[1]);
				expect(
					SHARED_TAGS.includes(first[1]),
					path + ' starts a snippet with <' + first[1] + '>, which draws it as HTML'
				).toBe(false);
			}
		}

		expect(seen.size).toBeGreaterThan(5);
	});
});
