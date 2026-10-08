import { describe, expect, it } from 'vitest';

import { TAB_SPRING, tabDirection, tabPaneIn, tabPaneOut, thumbTarget } from './tab-motion';

describe('tabDirection', () => {
	it('travels forward to a later tab and backward to an earlier one', () => {
		const order = ['overview', 'day', 'provider', 'model'];
		expect(tabDirection(order, 'overview', 'provider')).toBe(1);
		expect(tabDirection(order, 'model', 'day')).toBe(-1);
	});

	it('travels forward when a value is unknown or unchanged', () => {
		const order = ['agent', 'daemon', 'startup'];
		expect(tabDirection(order, 'agent', 'agent')).toBe(1);
		expect(tabDirection(order, 'agent', 'unknown')).toBe(1);
		expect(tabDirection(order, 'unknown', 'agent')).toBe(1);
	});
});

describe('thumbTarget', () => {
	const rects = [
		{ left: 3, width: 60 },
		{ left: 67, width: 80 }
	];

	it('places the thumb under the active segment', () => {
		expect(thumbTarget(rects, 1)).toEqual({ left: 67, width: 80 });
	});

	it('parks a zero-width thumb when the segment is missing', () => {
		expect(thumbTarget(rects, -1)).toEqual({ left: 0, width: 0 });
		expect(thumbTarget(rects, 9)).toEqual({ left: 0, width: 0 });
		expect(thumbTarget([], 0)).toEqual({ left: 0, width: 0 });
	});
});

describe('tab spring', () => {
	it('settles quickly with a small overshoot', () => {
		expect(TAB_SPRING.stiffness).toBeGreaterThan(0.1);
		expect(TAB_SPRING.stiffness).toBeLessThanOrEqual(0.5);
		expect(TAB_SPRING.damping).toBeGreaterThanOrEqual(0.7);
	});
});

describe('tab pane transitions', () => {
	function enter(direction: 1 | -1, reducedMotion: boolean) {
		return tabPaneIn({} as HTMLElement, { direction, reducedMotion });
	}

	function leave(direction: 1 | -1, reducedMotion: boolean) {
		return tabPaneOut({} as HTMLElement, { direction, reducedMotion });
	}

	it('slides from the travel direction and fades', () => {
		const entering = enter(1, false);
		expect(entering.duration).toBe(200);
		expect(entering.css?.(0, 1)).toContain('translateX(16px)');
		expect(entering.css?.(1, 0)).toContain('translateX(0px)');
		const leaving = leave(1, false);
		expect(leaving.duration).toBe(160);
		expect(leaving.css?.(0, 1)).toContain('translateX(-16px)');
	});

	it('mirrors the travel for a backward swap', () => {
		expect(enter(-1, false).css?.(0, 1)).toContain('translateX(-16px)');
		expect(leave(-1, false).css?.(0, 1)).toContain('translateX(16px)');
	});

	it('keeps a fade with no travel under reduced motion', () => {
		const entering = enter(1, true);
		expect(entering.duration).toBe(120);
		expect(entering.css?.(0.5, 0.5)).not.toContain('translateX');
		const leaving = leave(-1, true);
		expect(leaving.duration).toBe(120);
		expect(leaving.css?.(0.5, 0.5)).not.toContain('translateX');
	});
});
