import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import logoIds from './provider-logo-ids.json';
import { providerLogoPaths } from './provider-logo-paths';
import {
	providerLogoIsColored,
	providerLogoPath,
	providerLogoSrc,
	providerMonogram
} from './provider-logo';

const directory = resolve(import.meta.dirname, '../../static/provider-logos');

describe('providerLogoSrc', () => {
	it('resolves a sign-in id through its logo alias', () => {
		expect(providerLogoSrc('claude')).toBe('/provider-logos/anthropic.svg');
		expect(providerLogoSrc('grok')).toBe('/provider-logos/xai.svg');
		expect(providerLogoSrc('openai-codex')).toBe('/provider-logos/openai.svg');
	});

	it('resolves a hand-committed mark by its provider id', () => {
		expect(providerLogoSrc('teamorouter')).toBe('/provider-logos/teamorouter.svg');
	});

	// The keyless lane is the same vendor with no account, so it wears that
	// vendor's mark rather than one of its own.
	it('resolves the keyless Kilo lane through the gateway mark', () => {
		expect(providerLogoSrc('kilo-free')).toBe('/provider-logos/kilo.svg');
	});

	it('returns null when that id has no logo file', () => {
		expect(providerLogoSrc('not-a-provider')).toBeNull();
	});

	// A manifest entry with no file behind it would ask the console for a mark
	// that 404s, which shows as an empty tile rather than the monogram.
	it('names a mark that is actually on disk', () => {
		for (const id of logoIds) {
			expect(readFileSync(resolve(directory, `${id}.svg`), 'utf8')).toBeTruthy();
		}
	});

	// Every mark is drawn as a mask so it can take the console's ink, and a
	// mask only sees alpha. A mark that paints its own plates is the way this
	// silently turns into a filled blob, so the one exception is named.
	it('leaves every mark maskable apart from the declared coloured ones', () => {
		expect(logoIds.filter((id) => providerLogoIsColored(id)).sort()).toEqual([
			'302ai',
			'atomic-chat'
		]);
	});

	// models.dev serves OrcaRouter as a raster, which the console cannot mask
	// and cannot theme. The committed vector is the one that has to survive.
	it('draws OrcaRouter from a maskable vector, not the models.dev raster', () => {
		const body = readFileSync(resolve(directory, 'orcarouter.svg'), 'utf8');
		expect(body).toContain('viewBox="0 0 98 98"');
		expect(body).not.toContain('data:image');
		expect(providerLogoIsColored('orcarouter')).toBe(false);
	});

	// Every maskable mark has to be one ink, and drawing it as a mask is what
	// lets it take the theme's colour. A mark that arrives with its own fills
	// is either wrong or has to be declared in the colored set.
	it('paints every generated mark in one ink', () => {
		for (const id of Object.keys(providerLogoPaths)) {
			if (providerLogoIsColored(id)) continue;
			const body = readFileSync(resolve(directory, `${id}.svg`), 'utf8');
			const inks = body.match(/(?:fill|stroke)="(?!none|currentColor|url\()[^"]*"/g) ?? [];
			expect(inks, id).toEqual([]);
		}
	});

	// A mask keeps only alpha, so a plate behind the mark fills the whole tile
	// and the logo disappears into a solid block. The icon set carries such a
	// plate as a clip path, which is geometry the mark is clipped by and never
	// ink; if one is ever written out as a path this catches it. The same
	// hazard is why zenmux's decorative disc was dropped by hand.
	it('leaves no full-canvas plate behind a maskable mark', () => {
		for (const id of logoIds) {
			// Only a mask is at risk: a mark drawn as an image shows whatever the
			// vendor drew, plate included.
			if (providerLogoIsColored(id)) continue;
			const body = readFileSync(resolve(directory, `${id}.svg`), 'utf8');
			const box = body
				.match(/viewBox="([^"]+)"/)?.[1]
				.trim()
				.split(/\s+/)
				.map(Number);
			if (!box) continue;
			const [, , w, h] = box;
			// A plate from the icon set is a full-canvas rect written as one closed
			// subpath starting at the origin. Matching that shape exactly is the
			// point: a looser "spans the viewBox" test cannot tell a plate from a
			// real glyph, because path data mixes absolute coordinates with
			// relative deltas and reading them as pairs is meaningless.
			const norm = (d: string) => d.replace(/[\s,]/g, '').toUpperCase();
			const plate = new RegExp(`^M0\\.?0*H${w}\\.?0*V${h}\\.?0*(?:H0\\.?0*)?Z$`);
			const plates = [...body.matchAll(/ d="([^"]+)"/g)]
				.map((m) => m[1])
				.filter((d) => plate.test(norm(d)))
				.map((d) => norm(d));
			expect(plates, id).toEqual([]);
		}
	});

	// Monochrome is what makes one file serve both themes, so a mark must not
	// carry an ink of its own. Only painted geometry counts: a clip or mask
	// plate is never drawn, and a fill that defers to a pattern resolves to
	// whatever that pattern paints.
	it('paints every maskable mark in currentColor', () => {
		for (const id of logoIds) {
			if (providerLogoIsColored(id)) continue;
			const body = readFileSync(resolve(directory, `${id}.svg`), 'utf8').replace(
				/<defs>[\s\S]*?<\/defs>/g,
				''
			);
			const inks = body.match(/(?:fill|stroke)="(?!none|currentColor|url\()[^"]*"/g) ?? [];
			expect(inks, id).toEqual([]);
		}
	});
});

describe('providerLogoPath', () => {
	it('carries the geometry for a mark the chart draws itself', () => {
		expect(providerLogoPath('ollama')?.d.join('')).toContain('M');
		expect(providerLogoPath('opencode-free')?.viewBox).toBe('0 0 24 24');
	});

	// The icon set draws some marks as several separate paths, and OpenClaw is
	// the clearest case: its first path on its own is a dot the size of a
	// pixel, so a mark written from the first path alone looks like nothing is
	// there. Every path has to survive extraction.
	it('keeps every path of a mark the icon set draws in pieces', () => {
		expect(providerLogoPath('openclaw')?.d.length).toBe(5);
		expect(providerLogoPath('upstage')?.d.length).toBe(11);
		for (const id of ['openclaw', 'upstage', 'devin', 'azure-openai']) {
			const mark = providerLogoPath(id);
			expect(mark, id).not.toBeNull();
			for (const segment of mark!.d) expect(segment, id).toMatch(/^M/);
		}
	});

	it('resolves an aliased id', () => {
		expect(providerLogoPath('claude')).toBeNull();
		expect(providerLogoSrc('claude')).toBe('/provider-logos/anthropic.svg');
	});

	it('is null for an id with no mark', () => {
		expect(providerLogoPath('litellm')).toBeNull();
	});

	it('carries no geometry for a mark that is only a file', () => {
		expect(providerLogoPath('anthropic')).toBeNull();
	});

	// Only the marks extract-lobehub-icons.mjs cut have geometry here; the rest
	// are files. A caller falls back for both, so this is a bound rather than a
	// promise of coverage.
	it('covers the ids models.dev publishes no mark for', () => {
		for (const id of [
			'nous',
			'command-code',
			'kiro',
			'qwen',
			'iflow',
			'devin',
			'azure-openai',
			'ollama',
			'vllm',
			'lm-studio',
			'opencode-free',
			'alibaba-token-plan-cn',
			'chutes',
			'cline-pass',
			'gmicloud',
			'morph',
			'sakana',
			'upstage',
			// Coding agents wear their own mark, not their vendor's.
			'codex',
			'claude-code',
			'grok-build',
			'cline',
			'openclaw',
			'dsh',
			'mcode'
		]) {
			expect(providerLogoSrc(id), id).not.toBeNull();
			expect(providerLogoPath(id), id).not.toBeNull();
		}
	});

	// Marks that arrived from the project rather than the icon set are files
	// only: they have no geometry for the chart to draw, which is the same
	// position every models.dev mark is in and the chart falls back for it.
	it('leaves a project-supplied mark as a file the chart cannot inline', () => {
		for (const id of [
			'hermes',
			'pi',
			'omp',
			'orcarouter',
			'other',
			'shared',
			'google-antigravity'
		]) {
			expect(providerLogoSrc(id), id).not.toBeNull();
			expect(providerLogoPath(id), id).toBeNull();
		}
	});

	// No icon set publishes these, so the monogram is the intended answer. A
	// future bump that adds one shows up here instead of changing the console
	// without anyone deciding to.
	it('leaves the ids no icon set publishes on the monogram', () => {
		for (const id of [
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
			// Agents with no published mark. Claude Desktop is not here: it wears
			// Anthropic's mark, which is the point of the alias.
			'gajae',
			'prime',
			'raycast',
			'aside',
			'zcode'
		]) {
			expect(providerLogoSrc(id), id).toBeNull();
			expect(providerLogoPath(id), id).toBeNull();
		}
	});

	// Google Antigravity wears its own mark rather than Google's: the
	// connection signs into a different product with its own glyph.
	it('gives Google Antigravity its own mark rather than the vendor mark', () => {
		expect(providerLogoSrc('google-antigravity')).toBe('/provider-logos/google-antigravity.svg');
	});

	// An agent row is chosen by the operator as an agent, so it wears the
	// agent's own mark. Only the clients that genuinely have no mark of their
	// own fall back to the vendor they talk to.
	it('gives a coding agent its own mark rather than its vendor mark', () => {
		expect(providerLogoSrc('codex')).toBe('/provider-logos/codex.svg');
		expect(providerLogoSrc('claude-code')).toBe('/provider-logos/claude-code.svg');
		expect(providerLogoSrc('grok-build')).toBe('/provider-logos/grok-build.svg');
		expect(providerLogoSrc('claude-desktop')).toBe('/provider-logos/anthropic.svg');
	});

	// Oh My Pi is Pi's mark inside a ring: the ring is the O. The Pi glyph has
	// to sit inside it, which is the one thing worth pinning, because the two
	// marks are composed rather than drawn and nothing else would catch a
	// mis-scaled reuse.
	it('sets the Pi glyph inside the ring for Oh My Pi', () => {
		const body = readFileSync(resolve(directory, 'omp.svg'), 'utf8');
		expect(body).toContain('<circle');
		expect(body).toContain('scale(0.6)');
		const pi = readFileSync(resolve(directory, 'pi.svg'), 'utf8');
		expect(body).toContain(pi.match(/d="([^"]+)"/)![1]);
	});
});

describe('providerMonogram', () => {
	it('uses the first letters of two words, else the first two characters', () => {
		expect(providerMonogram('Claude API')).toBe('CA');
		expect(providerMonogram('Grok')).toBe('GR');
	});
});
