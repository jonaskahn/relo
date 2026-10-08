import { describe, expect, it } from 'vitest';

import { defaultAccountName, NAME_WORDS } from './account-name';

describe('defaultAccountName', () => {
	it('joins one animal and one personality', () => {
		expect(defaultAccountName(() => 0)).toBe('Aardvark Adaptable');
		expect(defaultAccountName(() => 0.999999)).toBe('Sparrow Witty');
	});

	it('picks each word on its own draw', () => {
		const draws = [0, 0.999999];
		expect(defaultAccountName(() => draws.shift() ?? 0)).toBe('Aardvark Witty');
	});

	it('has 50 animals and 50 personalities to draw from', () => {
		expect(NAME_WORDS.animals).toHaveLength(50);
		expect(NAME_WORDS.personalities).toHaveLength(50);
	});

	it('varies over many draws', () => {
		const names = new Set(Array.from({ length: 50 }, () => defaultAccountName()));
		expect(names.size).toBeGreaterThan(40);
	});
});
