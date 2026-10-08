import { afterEach, describe, expect, test, vi } from 'vitest';

import { modalMotionMode, originFor, stepSwap } from './modal-motion';

const rect = { left: 100, top: 50, width: 400, height: 300 };

describe('modal motion', () => {
	test('places the desktop transform origin at the pressed point', () => {
		expect(originFor({ x: 220, y: 140 }, rect)).toBe('120px 90px');
	});

	test('clamps a pressed point to the modal bounds', () => {
		expect(originFor({ x: 20, y: 500 }, rect)).toBe('0px 300px');
	});

	test('falls back to the modal centre without a recent press', () => {
		expect(originFor(null, rect)).toBe('center');
	});

	test('uses the phone breakpoint unless reduced motion is requested', () => {
		expect(modalMotionMode(639, false)).toBe('mobile');
		expect(modalMotionMode(640, false)).toBe('desktop');
		expect(modalMotionMode(1200, true)).toBe('reduced');
	});
});

// A step swap is a fade with a short rise, so the operator sees one surface
// change content rather than watch a panel travel.
describe('step swap', () => {
	// stepSwap reads the motion preference off the window, the way the other
	// transitions in the module do.
	function stubReduced(reduced: boolean) {
		vi.stubGlobal('window', { matchMedia: () => ({ matches: reduced }) });
	}

	afterEach(() => vi.unstubAllGlobals());

	test('rises six pixels into a step change', () => {
		stubReduced(false);
		const swap = stepSwap({} as HTMLElement);
		expect(swap.duration).toBe(180);
		const css = swap.css as (t: number, u: number) => string;
		expect(css(0, 1)).toContain('translateY(6px)');
		expect(css(1, 0)).toContain('translateY(0px)');
	});

	test('drops the rise under reduced motion, keeping a fade', () => {
		stubReduced(true);
		const swap = stepSwap({} as HTMLElement);
		expect(swap.duration).toBe(140);
		const css = swap.css as (t: number, u: number) => string;
		expect(css(0, 1)).toBe('opacity: 0');
		expect(css(1, 0)).toBe('opacity: 1');
	});
});
