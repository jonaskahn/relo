import { describe, expect, it } from 'vitest';

import { isFreeConnection } from './provider-free';

describe('isFreeConnection', () => {
	// A keyless lane accepts no account, so there is nothing to bill: the chip
	// says what the connection is, not what the operator saves.
	it('marks the keyless pools free', () => {
		expect(isFreeConnection({ template_id: 'kilo-free', auth: 'none' })).toBe(true);
		expect(isFreeConnection({ template_id: 'opencode-free', auth: 'none' })).toBe(true);
	});

	it('marks a keyless connection free whatever template it came from', () => {
		expect(isFreeConnection({ template_id: 'custom', auth: 'none' })).toBe(true);
	});

	it('leaves a connection that takes a credential unmarked', () => {
		expect(isFreeConnection({ template_id: 'openai', auth: 'api_key' })).toBe(false);
		expect(isFreeConnection({ template_id: 'anthropic', auth: 'oauth' })).toBe(false);
		expect(isFreeConnection({ template_id: 'kilo', auth: 'api_key' })).toBe(false);
	});
});
