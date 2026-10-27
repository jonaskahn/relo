import { describe, expect, it } from 'vitest';

import { hexToHsv, hueColour, hsvToHex, isHexColour, onAccent, parseHex } from './color';

describe('the colour the radar writes', () => {
	it('accepts only the shape the daemon stores', () => {
		expect(isHexColour('#c8102e')).toBe(true);
		expect(isHexColour('#C8102E')).toBe(true);
		expect(isHexColour('c8102e')).toBe(false);
		expect(isHexColour('#c8102')).toBe(false);
		expect(isHexColour('#c8102ee')).toBe(false);
		expect(isHexColour('')).toBe(false);
	});

	it('reads what an operator types, with or without the hash', () => {
		expect(parseHex('#C0FFEE')).toBe('#c0ffee');
		expect(parseHex(' c0ffee ')).toBe('#c0ffee');
		expect(parseHex('coffee')).toBeUndefined();
		expect(parseHex('#c0ff')).toBeUndefined();
	});

	it('round-trips every corner of the board', () => {
		for (const hex of [
			'#000000',
			'#ffffff',
			'#808080',
			'#ff0000',
			'#00ff00',
			'#0000ff',
			'#c8102e',
			'#2f5fd0',
			'#d9961f',
			'#268e8e'
		]) {
			expect(hsvToHex(hexToHsv(hex))).toBe(hex);
		}
	});

	it('keeps the cursor in range whatever it is handed', () => {
		expect(hsvToHex({ hue: -30, saturation: 4, value: -2 })).toBe('#000000');
		expect(hsvToHex({ hue: 720, saturation: 0, value: 0.5 })).toBe('#808080');
		expect(hexToHsv('#000000')).toEqual({ hue: 0, saturation: 0, value: 0 });
	});

	it('names the hue the rail shows, in either direction', () => {
		expect(hueColour(0)).toBe('hsl(0 100% 50%)');
		expect(hueColour(359.6)).toBe('hsl(360 100% 50%)');
		expect(hueColour(-30)).toBe('hsl(330 100% 50%)');
	});
});

describe('the ink an accent carries', () => {
	it('picks whichever colour reads better on the base', () => {
		expect(onAccent('#ffffff')).toBe('#17171c');
		expect(onAccent('#000000')).toBe('#ffffff');
		expect(onAccent('#c8102e')).toBe('#ffffff');
		expect(onAccent('#d9961f')).toBe('#17171c');
	});

	it('never answers with a colour that is not an ink', () => {
		for (const base of ['#c86343', '#e8553a', '#8c1c2c', '#2b3a67', '#5b6270']) {
			expect(['#ffffff', '#17171c']).toContain(onAccent(base));
		}
		expect(onAccent('not a colour')).toBe('#ffffff');
	});
});
