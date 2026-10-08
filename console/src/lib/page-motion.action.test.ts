import { afterEach, describe, expect, test, vi } from 'vitest';

import { pageMotion } from './page-motion';

// The action reads the motion preference off the window, so it is stood in
// for; what is under test is that it hands the preference to the config.
afterEach(() => {
	vi.unstubAllGlobals();
});

function stubMotionPreference(reduced: boolean) {
	vi.stubGlobal('window', {
		matchMedia: (query: string) => ({ matches: reduced && query.includes('reduce') })
	});
}

describe('pageMotion action', () => {
	test('uses the full motion when none is reduced', () => {
		stubMotionPreference(false);
		const config = pageMotion(null as unknown as HTMLElement);

		expect(config.duration).toBe(180);
		expect(config.css?.(0, 1)).toContain('translateY(6px)');
	});

	test('uses the reduced motion when the system asks for it', () => {
		stubMotionPreference(true);
		const config = pageMotion(null as unknown as HTMLElement);

		expect(config.duration).toBe(120);
		expect(config.css?.(0.5, 0.5)).toBe('opacity: 0.5');
	});
});
