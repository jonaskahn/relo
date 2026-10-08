import { describe, expect, it } from 'vitest';

import { radioStep } from './appearance-keyboard';

describe('radioStep', () => {
	it('moves through a radio group and wraps at the ends', () => {
		expect(radioStep(0, 3, 'ArrowRight')).toBe(1);
		expect(radioStep(2, 3, 'ArrowDown')).toBe(0);
		expect(radioStep(0, 3, 'ArrowLeft')).toBe(2);
		expect(radioStep(1, 3, 'ArrowUp')).toBe(0);
		expect(radioStep(4, 10, 'Home')).toBe(0);
		expect(radioStep(4, 10, 'End')).toBe(9);
		expect(radioStep(1, 3, 'Enter')).toBeNull();
		expect(radioStep(0, 0, 'ArrowRight')).toBeNull();
	});
});
