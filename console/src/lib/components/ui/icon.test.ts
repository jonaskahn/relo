import { describe, expect, it } from 'vitest';

import { isIconName } from './icon.svelte';

// The guard is what keeps a name that drifted out of a state value from
// reaching a path lookup with nothing behind it.

describe('isIconName', () => {
	it('accepts every name the vendored set draws', () => {
		expect(isIconName('check')).toBe(true);
		expect(isIconName('layout-sidebar')).toBe(true);
		expect(isIconName('x')).toBe(true);
	});

	it('refuses a name the set does not carry', () => {
		expect(isIconName('check-circle')).toBe(false);
		expect(isIconName('')).toBe(false);
		expect(isIconName('Check')).toBe(false);
	});
});
