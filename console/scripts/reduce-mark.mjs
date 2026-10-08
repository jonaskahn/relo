#!/usr/bin/env node
// Reduce a vendor-supplied SVG to the shape this directory keeps: no editor
// metadata, no intrinsic size, and one ink so the console can mask it.
//
// Usage: node scripts/reduce-mark.mjs <raw.svg> <id> [--colour]
//
// The path data is never retyped or rewritten. It is lifted out of the source
// verbatim, which is the only way a mark this long survives the trip: the
// geometry is opaque, so a dropped digit is invisible until it renders wrong.
import { readFileSync, writeFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = fileURLToPath(new URL('.', import.meta.url));
const output = resolve(here, '../static/provider-logos');

const [, , rawArg, idArg, ...flags] = process.argv;
if (!rawArg || !idArg) {
	console.error('usage: node scripts/reduce-mark.mjs <raw.svg> <id> [--colour]');
	process.exit(1);
}
const keepColour = flags.includes('--colour');
const raw = readFileSync(rawArg, 'utf8');

// viewBox is the only size worth keeping: the tile sizes the mark itself.
const box = raw.match(/viewBox="([^"]+)"/);
if (!box) throw new Error(`${idArg}: no viewBox, cannot scale it`);
const [, , w, h] = box[1].trim().split(/\s+/).map(Number);

// The style block is what carries an Inkscape export's fill-rule, so a
// monochrome mark takes it onto the paths rather than losing it.
const styleRule = raw.match(/fill-rule:\s*(\w+)/)?.[1];
const rule = styleRule && styleRule !== 'nonzero' ? ` fill-rule="${styleRule}"` : '';

// Gradients and the CSS that references them are the drawing, not metadata, so
// a colour mark keeps its defs and stylesheet verbatim.
const defs = raw.match(/<defs>[\s\S]*?<\/defs>/)?.[0] ?? '';
const style = keepColour ? (raw.match(/<style>[\s\S]*?<\/style>/)?.[0] ?? '') : '';

// A path can sit inside a transformed group, and the transform is geometry, not
// metadata: an Inkscape export scales a 24-unit glyph by 8.33 to fill a 200-unit
// viewBox, so dropping the wrapper would draw the mark at an eighth size in the
// corner. It is kept when every path shares one.
const groups = [...raw.matchAll(/<g\b([^>]*)>\s*(<path\b[\s\S]*?<\/g>)/g)];
const shared = new Set(groups.map((m) => m[1].trim()));
const wrap = groups.length > 0 && shared.size === 1 ? [...shared][0] : '';

const paths = [...raw.matchAll(/<path\b([^>]*?)\/>/g)].map((m) => m[1]);
if (paths.length === 0) throw new Error(`${idArg}: no <path> found`);

// Keep only what draws: the geometry and any fill a colour mark depends on.
// Everything else an exporter emits (class hooks, opacity, data-original) is
// dropped, because the mask reads alpha and those only fade it.
const kept = paths.map((attrs) => {
	const d = attrs.match(/\sd="([^"]+)"/)?.[1];
	if (!d) throw new Error(`${idArg}: a <path> has no d`);
	if (keepColour) {
		const cls = attrs.match(/\sclass="([^"]+)"/)?.[1];
		const fill = attrs.match(/\sfill="([^"]+)"/)?.[1];
		const named = cls ? ` class="${cls}"` : fill ? ` fill="${fill}"` : '';
		return `<path${named} d="${d}"/>`;
	}
	return `<path fill="currentColor"${rule} d="${d}"/>`;
});

const body =
	`<svg viewBox="${box[1].trim()}" xmlns="http://www.w3.org/2000/svg">` +
	(keepColour ? defs + style : '') +
	(wrap ? `<g ${wrap}>` : '') +
	kept.join('') +
	(wrap ? `</g>` : '') +
	`</svg>\n`;
writeFileSync(resolve(output, `${idArg}.svg`), body);

console.log(
	`${idArg}: ${paths.length} path(s), viewBox "${box[1].trim()}" (${w} x ${h}), ` +
		`${wrap ? 'shared group kept, ' : ''}` +
		`${keepColour ? 'colour as supplied' : `one ink${rule ? `, ${styleRule}` : ''}`}`
);
