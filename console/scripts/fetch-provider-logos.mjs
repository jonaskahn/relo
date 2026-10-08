#!/usr/bin/env node
// Refresh the bundled provider marks from the catalog snapshot. models.dev
// returns its generic logo for unknown IDs, so those IDs use our UI fallback.
import { readFileSync, readdirSync, mkdirSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = fileURLToPath(new URL('.', import.meta.url));
// The catalog snapshot the daemon caches, not a seed file: this is the copy
// whose provider ids the console actually offers.
const snapshot = JSON.parse(readFileSync(resolve(here, '../../.dev/cache/modelsdev.json'), 'utf8'));
const output = resolve(here, '../static/provider-logos');
const manifest = resolve(here, '../src/lib/provider-logo-ids.json');
const source = 'https://models.dev/logos';

async function svg(url) {
	const response = await fetch(url);
	if (!response.ok) throw new Error(`${response.status} ${url}`);
	const body = await response.text();
	if (!body.trimStart().startsWith('<svg')) throw new Error(`Not an SVG: ${url}`);
	return body;
}

const fallback = await svg(`${source}/relo-unknown-provider-logo.svg`);
const ids = snapshot.providers.map((provider) => provider.id);
mkdirSync(output, { recursive: true });

let downloaded = 0;
let missing = 0;
// Marks a vendor publishes and models.dev does not (for example a coding agent
// that is not a model provider) are committed by hand, and the ones
// extract-lobehub-icons.mjs cuts also land here. Scanning the directory after
// the fetch keeps all of them in the manifest, so regenerating the marks never
// drops an asset the console still points at.
const available = readdirSync(output)
	.filter((name) => name.endsWith('.svg'))
	.map((name) => name.slice(0, -'.svg'.length));

// Ids whose committed mark is better than the one models.dev serves. OrcaRouter
// publishes its mark only as a raster, which the console cannot mask and cannot
// theme, so the vector committed by hand wins and the fetch leaves it alone.
const keepCommitted = new Set(['orcarouter']);
for (let start = 0; start < ids.length; start += 12) {
	await Promise.all(
		ids.slice(start, start + 12).map(async (id) => {
			if (keepCommitted.has(id)) return;
			try {
				// Alibaba's supplied lab URL resolves to the same mark as its provider URL.
				const path = id === 'alibaba' ? `labs/${id}` : id;
				const body = await svg(`${source}/${path}.svg`);
				if (body === fallback) {
					missing++;
					return;
				}
				writeFileSync(resolve(output, `${id}.svg`), body);
				available.push(id);
				downloaded++;
			} catch (error) {
				console.error(`Logo ${id}: ${error.message}`);
				missing++;
			}
		})
	);
}
writeFileSync(manifest, JSON.stringify(available.sort()) + '\n');
console.log(`Downloaded ${downloaded} logos; ${missing} providers use the fallback.`);
