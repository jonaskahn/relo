import { describe, expect, it } from 'vitest';

import { isFreeConnection, isOpenCodeFreeConnection } from './provider-free';

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

describe('isOpenCodeFreeConnection', () => {
	it('names only the signed-out OpenCode lane', () => {
		expect(isOpenCodeFreeConnection({ template_id: 'opencode-free' })).toBe(true);
		expect(isOpenCodeFreeConnection({ template_id: 'kilo-free' })).toBe(false);
		expect(isOpenCodeFreeConnection({ template_id: 'openai' })).toBe(false);
	});
});
