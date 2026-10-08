#!/usr/bin/env node
// Extract monochrome marks from @lobehub/icons for the provider ids models.dev
// publishes no mark for. The package is a React component library, so it is
// never a dependency: this script reads the tarball once and writes plain
// single-path SVGs, which is the same shape the rest of the directory holds.
//
// LobeHub's Mono component is `fill="currentColor"` with one `path`, so the
// mark inherits whatever colour the console paints it with. That is the only
// reason to prefer it here over a brand-coloured mark: the logo tile and the
// universe chart both draw on a themed surface.
import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = fileURLToPath(new URL('.', import.meta.url));
const output = resolve(here, '../static/provider-logos');
const pathsModule = resolve(here, '../src/lib/provider-logo-paths.ts');

// LobeHub is read from a tarball this script unpacks itself, so it is never a
// dependency of the console and never reaches the bundle. Pinned, because the
// path geometry a version ships is what the committed marks are cut from.
const PINNED_VERSION = process.argv[2] ?? '5.21.0';
const cache = resolve(tmpdir(), `lobehub-icons-${PINNED_VERSION}`);

// LobeHub name by provider id. Only ids the console currently offers and
// models.dev does not publish a mark for appear here; adding one is the whole
// fix for that row.
const marks = {
	// Sign-in connections models.dev has no entry for.
	nous: 'NousResearch',
	'command-code': 'CommandCode',
	kiro: 'Kiro',
	qwen: 'Qwen',
	iflow: 'IFlyTekCloud',
	devin: 'Devin',
	// Cloud and local runtimes, which are not model providers at all.
	'azure-openai': 'Azure',
	ollama: 'Ollama',
	vllm: 'Vllm',
	'lm-studio': 'LmStudio',
	// API-key rows, including OpenCode Zen, which is the free tier of the
	// OpenCode connection and wears the same mark.
	'opencode-free': 'OpenCode',
	'alibaba-token-plan-cn': 'Alibaba',
	chutes: 'Chutes',
	'cline-pass': 'Cline',
	gmicloud: 'GmiCloud',
	morph: 'Morph',
	sakana: 'Sakana',
	upstage: 'Upstage',

	// Coding agents the agents page lists. Each id is the client identifier
	// the daemon reports, so a row wears its own agent's mark rather than the
	// mark of whichever model vendor that agent talks to.
	codex: 'Codex',
	'claude-code': 'ClaudeCode',
	'grok-build': 'Grok',
	cline: 'Cline',
	openclaw: 'OpenClaw',
	dsh: 'DeepSeek',
	mcode: 'Minimax'
};

// Ids that stay on the monogram fallback because no icon set publishes them.
// A future icon-set bump that adds one shows up as a failing expectation in
// provider-logo.test.ts rather than a silent change.
const uncovered = [
	// Connections no icon set covers.
	'aki-io',
	'bee',
	'cortecs',
	'crof',
	'hpc-ai',
	'iteracompute',
	'litellm',
	'mixlayer',
	'pareto',
	'regolo-ai',
	'requesty',
	'sap-ai-core',
	'sarvam',
	'stackit',
	'synthetic',
	// Coding agents no icon set covers. Claude Desktop and the generic client
	// fall back to the mark of the vendor they talk to; the rest are private or
	// small tools with no published mark anywhere.
	'claude-desktop',
	'gajae',
	'prime',
	'raycast',
	'aside',
	'zcode'
];

// unpack fetches the pinned tarball once and unpacks it, so the marks can be
// regenerated on a machine that has never installed the package.
function unpack() {
	if (existsSync(resolve(cache, 'package/es'))) return resolve(cache, 'package/es');
	mkdirSync(cache, { recursive: true });
	const tgz = resolve(cache, 'icons.tgz');
	if (!existsSync(tgz)) {
		const url = `https://registry.npmjs.org/@lobehub/icons/-/icons-${PINNED_VERSION}.tgz`;
		execFileSync('curl', ['-sSfL', url, '-o', tgz], { stdio: 'inherit' });
	}
	execFileSync('tar', ['-xzf', tgz, '-C', cache], { stdio: 'inherit' });
	return resolve(cache, 'package/es');
}

const esRoot = unpack();

// lobehubMono reads one monochrome icon. LobeHub names the file Mono.js for
// most marks and BrandMono.js for the ones with a separate wordmark, so both
// are accepted.
function lobehubMono(name) {
	for (const file of ['Mono.js', 'BrandMono.js']) {
		try {
			return readFileSync(resolve(esRoot, name, 'components', file), 'utf8');
		} catch {
			continue;
		}
	}
	return '';
}

// blocks strips a call and everything inside it, matched on balanced braces, so
// a nested call cannot be picked up on its own.
function blocks(source, names) {
	let out = source;
	for (const name of names) {
		const open = new RegExp(`_jsx\\("${name}",\\s*\\{`, 'g');
		for (const start of [...source.matchAll(open)]) {
			let depth = 0;
			let i = start.index + start[0].length - 1;
			for (; i < source.length; i++) {
				if (source[i] === '{') depth++;
				else if (source[i] === '}' && --depth === 0) break;
			}
			out = out.slice(0, start.index) + ' '.repeat(i + 1 - start.index) + out.slice(i + 1);
		}
	}
	return out;
}

// glyph pulls the viewBox and every painted path out of a LobeHub component. A
// Mono component is one or more _jsx('path') calls, and they are separate
// elements rather than one path with subpaths, so they are kept separate here:
// merging them would let one shape punch a hole in another under the even-odd
// rule.
//
// defs, clipPath and mask are removed first. They carry paths of their own —
// usually a full-canvas `M0 0h24v24H0z` — which are geometry the mark is
// clipped *by*, not ink it draws. Left in, they turn the mark into a solid
// block, because a mask keeps alpha and a full-canvas fill is fully opaque.
//
// The d is read from inside each call's own braces, because the component's
// stylesheet also carries bare `d: "b"` keys that a looser match picks up.
function glyph(source) {
	const box = source.match(/viewBox:\s*"([^"]+)"/);
	if (!box) return null;
	const fillRule = source.match(/fillRule:\s*"([^"]+)"/)?.[1] ?? null;

	const painted = blocks(source, ['defs', 'clipPath', 'mask', 'pattern']);
	const paths = [];
	for (const call of painted.matchAll(/_jsx\("path",\s*\{/g)) {
		let depth = 0;
		let i = call.index + call[0].length - 1;
		for (; i < painted.length; i++) {
			if (painted[i] === '{') depth++;
			else if (painted[i] === '}' && --depth === 0) break;
		}
		const body = painted.slice(call.index, i + 1);
		// An explicit fill of none is a cut-out, which stays out of the mark.
		if (/fill:\s*"none"/.test(body)) continue;
		const d = body.match(/\bd:\s*"((?:[^"\\]|\\.)*)"/);
		if (d) paths.push({ d: d[1].replace(/\\"/g, '"').replace(/\\\\/g, '\\'), body });
	}
	if (paths.length === 0) return null;
	return { viewBox: box[1], fillRule, paths };
}

// svgFor writes the glyph with no width or height, so the console sizes it and
// the mark carries no intrinsic size of its own.
function svgFor(g) {
	const rule = g.fillRule ? ` fill-rule="${g.fillRule}"` : '';
	const body = g.paths
		.map(({ d, body: attrs }) => {
			const clip = attrs.match(/\bclipRule:\s*"([^"]+)"/);
			return `<path fill="currentColor"${rule}${clip ? ` clip-rule="${clip[1]}"` : ''} d="${d}"/>`;
		})
		.join('');
	return `<svg viewBox="${g.viewBox}" xmlns="http://www.w3.org/2000/svg">${body}</svg>\n`;
}

mkdirSync(output, { recursive: true });

const paths = {};
let written = 0;
for (const [id, mark] of Object.entries(marks)) {
	const g = glyph(lobehubMono(mark));
	if (g === null) {
		console.error(`${id} (${mark}): no monochrome path`);
		continue;
	}
	writeFileSync(resolve(output, `${id}.svg`), svgFor(g));
	const [, , , w, h] = g.viewBox.split(' ').map(Number);
	paths[id] = {
		viewBox: g.viewBox,
		d: g.paths.map((p) => p.d),
		width: w || 24,
		height: h || 24
	};
	written++;
}

// The universe chart cannot mask an SVG child, so it draws the geometry inline
// and needs it in JS as well as on disk. This module is generated: edit the
// script, not the file.
writeFileSync(
	pathsModule,
	`// Generated by scripts/extract-lobehub-icons.mjs from @lobehub/icons ${PINNED_VERSION}. ` +
		'Do not edit by hand.\n' +
		'//\n' +
		'// The universe chart draws a mark as inline <path> elements rather than an\n' +
		'// <image>, because a CSS mask cannot be applied to an SVG child and\n' +
		'// currentColor does not resolve inside one. Everything else uses the .svg\n' +
		'// files.\n' +
		'//\n' +
		'// d is an array because the icon set draws some marks as several separate\n' +
		'// paths. They stay separate: merged into one path, even-odd filling would\n' +
		'// let an overlapping shape punch a hole in the one beneath it.\n\n' +
		"/** One mark's geometry: a viewBox, the paths that draw it, and the size it declares. */\n" +
		'export interface LogoPath {\n\tviewBox: string;\n\td: string[];\n' +
		'\t// The viewBox width and height, kept beside the string so a caller can scale\n' +
		'\t// the path without parsing it back out.\n\twidth: number;\n\theight: number;\n}\n\n' +
		'export const providerLogoPaths: Record<string, LogoPath> = ' +
		JSON.stringify(paths, null, '\t') +
		';\n'
);

console.log(`Wrote ${written} marks from @lobehub/icons ${PINNED_VERSION}.`);
console.log(`${uncovered.length} ids stay on the monogram fallback: ${uncovered.join(' ')}`);
