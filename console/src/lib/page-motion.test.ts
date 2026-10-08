import { describe, expect, test } from 'vitest';

import { pageMotionConfig } from './page-motion';

describe('page motion', () => {
	test('uses a short fade and rise for route changes', () => {
		const config = pageMotionConfig(false);

		expect(config.duration).toBe(180);
		expect(config.css?.(0, 1)).toContain('translateY(6px)');
		expect(config.css?.(1, 0)).toContain('translateY(0px)');
	});

	test('removes travel when reduced motion is requested', () => {
		const config = pageMotionConfig(true);

		expect(config.duration).toBe(120);
		expect(config.css?.(0.5, 0.5)).toBe('opacity: 0.5');
	});
});
