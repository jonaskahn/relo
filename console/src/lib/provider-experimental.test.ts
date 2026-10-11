import { describe, expect, it } from 'vitest';

import { isExperimentalConnection } from './provider-experimental';

describe('isExperimentalConnection', () => {
	// The keyless Kilo pool logs every prompt at the vendor, so the chip says
	// the lane is still settling rather than let the Free chip stand alone.
	it('marks the keyless Kilo lane experimental', () => {
		expect(isExperimentalConnection('kilo-free')).toBe(true);
	});

	// The sibling keyless lane logs nothing, so it carries no such caveat.
	it('leaves every other lane unchipped', () => {
		expect(isExperimentalConnection('opencode-free')).toBe(false);
		expect(isExperimentalConnection('openai')).toBe(false);
		expect(isExperimentalConnection('anthropic')).toBe(false);
	});

	// The metered Kilo connection is the same vendor on a paid key, so the id
	// the connection was given cannot decide the chip.
	it('leaves the metered Kilo connection unchipped', () => {
		expect(isExperimentalConnection('kilo')).toBe(false);
	});
});
