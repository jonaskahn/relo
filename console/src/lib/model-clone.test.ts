import { describe, expect, it } from 'vitest';

import {
	clonePayload,
	formatPrice,
	parsePrice,
	suggestedCloneId,
	validateCloneId
} from './model-clone';

describe('suggestedCloneId', () => {
	it('names the first copy -copy', () => {
		expect(suggestedCloneId('gpt-5', [])).toBe('gpt-5-copy');
	});
	it('counts up while the name is taken', () => {
		expect(suggestedCloneId('gpt-5', ['gpt-5-copy'])).toBe('gpt-5-copy-2');
		expect(suggestedCloneId('gpt-5', ['gpt-5-copy', 'gpt-5-copy-2'])).toBe('gpt-5-copy-3');
	});
	it('keeps counting past every name in reach', () => {
		// An operator who has copied this model many times still gets a name
		// that is free rather than one already taken.
		const taken = Array.from({ length: 1000 }, (_, index) => `gpt-5-copy-${index + 1}`);
		expect(suggestedCloneId('gpt-5', taken)).not.toBe('gpt-5-copy-1');
	});
});

describe('formatPrice', () => {
	it('writes stored micros back as dollars per million', () => {
		expect(formatPrice(3_000_000)).toBe('3');
		expect(formatPrice(280_000)).toBe('0.28');
	});
	it('states nothing for a price that was never set', () => {
		expect(formatPrice(null)).toBe('');
		expect(formatPrice(undefined)).toBe('');
	});
});

describe('validateCloneId', () => {
	it('accepts a free identifier', () => {
		expect(validateCloneId('gpt-5-copy', [])).toBeNull();
	});
	it('names what is wrong', () => {
		expect(validateCloneId('', [])).toBe('empty');
		expect(validateCloneId('has space', [])).toBe('whitespace');
		expect(validateCloneId('taken', ['taken'])).toBe('taken');
	});
});

describe('parsePrice', () => {
	it('converts dollars per million to the stored micros', () => {
		expect(parsePrice('3')).toBe(3_000_000);
		expect(parsePrice('$0.28')).toBe(280_000);
	});
	it('states nothing for an empty field', () => {
		expect(parsePrice('')).toBeNull();
	});
});

describe('clonePayload', () => {
	const source = {
		upstreamId: 'gpt-5',
		name: 'GPT-5',
		contextWindow: 400_000,
		maxOutput: 64_000,
		inputPrice: 1_000_000,
		outputPrice: 2_000_000
	};
	const untouched = {
		upstreamId: 'gpt-5',
		name: 'GPT-5',
		contextWindow: '400000',
		maxOutput: '64000',
		inputPrice: '1',
		outputPrice: '2'
	};

	it('sends nothing when the operator changed nothing', () => {
		expect(clonePayload(untouched, source)).toEqual({});
	});
	it('sends the upstream id when it differs', () => {
		expect(clonePayload({ ...untouched, upstreamId: 'gpt-5.1' }, source)).toEqual({
			upstream_model_id: 'gpt-5.1'
		});
	});
	it('sends only the override fields that changed', () => {
		const payload = clonePayload(
			{ ...untouched, contextWindow: '500k', name: 'GPT-5 Preview' },
			source
		);
		expect(payload.override).toEqual({ name: 'GPT-5 Preview', context_window: 500_000 });
	});
	it('sends a price only when it changed', () => {
		const payload = clonePayload({ ...untouched, inputPrice: '3' }, source);
		expect(payload.override?.prices).toEqual({ input: 3_000_000 });
	});
});
