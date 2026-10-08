import { describe, expect, it } from 'vitest';

import { providerLogoPaths } from './provider-logo-paths';

// The geometry is generated from @lobehub/icons by scripts/extract-lobehub-icons.mjs.
// These are the invariants a bad regeneration would break, and the ones the
// universe chart relies on to scale a mark without parsing the viewBox back.

describe('provider logo paths', () => {
	it('draws every mark from at least one path', () => {
		const entries = Object.entries(providerLogoPaths);
		expect(entries.length).toBeGreaterThan(0);
		for (const [id, logo] of entries) {
			expect(logo.d.length, id).toBeGreaterThan(0);
			expect(
				logo.d.every((d) => /^[Mm]/.test(d)),
				id
			).toBe(true);
		}
	});

	it('anchors every viewBox at the origin and declares the width a caller scales against', () => {
		for (const [id, logo] of Object.entries(providerLogoPaths)) {
			const parts = logo.viewBox.split(' ').map(Number);
			expect(parts, id).toHaveLength(4);
			expect(parts.slice(0, 2), id).toEqual([0, 0]);
			expect(parts[2], id).toBe(logo.width);
		}
	});

	// One mark ships a 64-unit viewBox rather than the set's 24, and the
	// generator kept the source height beside it. Nothing reads the height —
	// the chart scales off the width — so this records the mismatch instead of
	// leaving it for a reader to find.
	it('keeps the declared height to the viewBox everywhere but the mark that ships scaled', () => {
		const disagreeing = Object.entries(providerLogoPaths)
			.filter(([, logo]) => Number(logo.viewBox.split(' ')[3]) !== logo.height)
			.map(([id]) => id);
		expect(disagreeing).toEqual(['chutes']);
	});
});
