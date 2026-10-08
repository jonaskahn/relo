import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// One control system means one height and one radius. This source scan is
// the review a refactor cannot forget: a raw button or a trigger styled
// with an ad-hoc height or a pill corner fails the build, not the eye.

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

const SRC = 'src';
const SVELTE_FILES = files(SRC);

// Interactive elements whose class strings must not carry ad-hoc heights
// or pill corners. ui/ primitives are exempt because they define the system.
const ELEMENT_CLASS = /<(button|a|input|select|textarea)[^>]*?class="([^"]*)"/g;
const TRIGGER_CLASS = /<(Popover|DropdownMenu|Tooltip|Dialog)[.]Trigger[^>]*?class="([^"]*)"/g;

const FORBIDDEN_TOKENS = [
	'h-7',
	'h-8',
	'h-10',
	'h-11',
	'size-7',
	'size-8',
	'size-10',
	'rounded-full'
];

// A block tag that loses its brace is still valid markup, so neither
// svelte-check nor a type error reports it. Svelte renders it as literal
// text and swallows the branch that follows it, which silently emptied
// every button in the console. This is the scan that catches that class of
// typo: an "{:else}" written as ":else".
const BARE_ELSE = /^\s*:else\b/m;

function forbiddenToken(classes: string): string {
	const tokens = classes.split(/\s+/);
	return FORBIDDEN_TOKENS.find((token) => tokens.includes(token)) ?? '';
}

// A raw <button> outside ui/ has to be a list-pattern control (a radio, an
// option, an expander) or carry data-plain for its full-row hit target.
const RAW_BUTTON = /<button([^>]*)>/g;
const PATTERN_ATTR = /data-plain|role=|aria-current=|aria-checked=|aria-expanded=|aria-pressed=/;

describe('control consistency', () => {
	it('scans the component tree', () => {
		expect(SVELTE_FILES.length).toBeGreaterThan(50);
	});

	it('keeps ad-hoc heights and pill corners off every control', () => {
		for (const path of SVELTE_FILES) {
			const inUi = path.includes(join(SRC, 'lib', 'components', 'ui'));
			if (inUi) continue;
			const text = readFileSync(path, 'utf8');
			for (const regex of [ELEMENT_CLASS, TRIGGER_CLASS]) {
				regex.lastIndex = 0;
				for (const match of text.matchAll(regex)) {
					const classes = match[2];
					const found = forbiddenToken(classes);
					expect(found === '', path + ' uses ' + found + ' on: ' + classes).toBe(true);
				}
			}
		}
	});

	it('keeps raw buttons to list-pattern controls or full-row targets', () => {
		let seen = 0;
		for (const path of SVELTE_FILES) {
			const inUi = path.includes(join(SRC, 'lib', 'components', 'ui'));
			if (inUi) continue;
			const text = readFileSync(path, 'utf8');
			for (const match of text.matchAll(RAW_BUTTON)) {
				seen += 1;
				const attrs = match[1];
				expect(
					PATTERN_ATTR.test(attrs),
					path + ' has a bare <button' + attrs.slice(0, 60) + '>'
				).toBe(true);
			}
		}
		expect(seen).toBeGreaterThan(10);
	});

	it('keeps every block tag braced, so a branch is never read as text', () => {
		for (const path of SVELTE_FILES) {
			const text = readFileSync(path, 'utf8');
			expect(BARE_ELSE.test(text), path + ' has a block tag missing its brace').toBe(false);
		}
	});
});
