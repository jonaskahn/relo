import { describe, expect, it } from 'vitest';

import { isFreeConnection } from './provider-free';

describe('isFreeConnection', () => {
	it('treats the two keyless lanes as free', () => {
		expect(isFreeConnection({ template_id: 'kilo-free' })).toBe(true);
		expect(isFreeConnection({ template_id: 'opencode-free' })).toBe(true);
	});

	it('does not treat local or custom endpoints as free', () => {
		expect(isFreeConnection({ template_id: 'custom' })).toBe(false);
		expect(isFreeConnection({ template_id: 'ollama' })).toBe(false);
	});

	it('does not treat metered providers as free', () => {
		expect(isFreeConnection({ template_id: 'openai' })).toBe(false);
		expect(isFreeConnection({ template_id: 'anthropic' })).toBe(false);
		expect(isFreeConnection({ template_id: 'kilo' })).toBe(false);
	});
});
