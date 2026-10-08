import { describe, expect, it } from 'vitest';

import { cn } from './utils';

// Every component's class prop goes through here, so what is under test is
// that a caller's own utility wins the conflict the base class would have set.

describe('cn', () => {
	it('joins the class names it is handed', () => {
		expect(cn('flex', 'items-center')).toBe('flex items-center');
	});

	it('drops the empty ones a conditional leaves behind', () => {
		const hidden: string | false = false;
		expect(cn('flex', hidden && 'hidden', undefined, 'gap-2')).toBe('flex gap-2');
	});

	it('resolves two utilities that set the same property', () => {
		expect(cn('p-2', 'p-4')).toBe('p-4');
		expect(cn('text-sm', 'text-lg')).toBe('text-lg');
	});

	it('keeps two utilities that set different properties', () => {
		expect(cn('rounded-card border', 'border-line')).toBe('rounded-card border border-line');
	});
});
