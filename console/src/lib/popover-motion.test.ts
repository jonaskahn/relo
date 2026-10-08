import { describe, expect, test } from 'vitest';

import { popoverMotionConfig } from './popover-motion';

describe('popover motion', () => {
	test('drops the panel in from four pixels above', () => {
		const config = popoverMotionConfig(false);

		expect(config.duration).toBe(160);
		expect(config.css?.(0, 1)).toContain('translateY(-4px)');
		expect(config.css?.(1, 0)).toContain('translateY(0px)');
	});

	test('removes travel when reduced motion is requested', () => {
		const config = popoverMotionConfig(true);

		expect(config.duration).toBe(140);
		expect(config.css?.(0.5, 0.5)).toBe('opacity: 0.5');
	});
});
