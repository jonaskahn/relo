import { describe, expect, it } from 'vitest';

import { accountCaption, namedCaption } from './account-label';

describe('namedCaption', () => {
	it('joins a name and a provider', () => {
		expect(namedCaption('cred-1', 'Work', 'ChatGPT')).toBe('Work (ChatGPT)');
		expect(namedCaption('smart', 'Smart', 'OpenAI')).toBe('Smart (OpenAI)');
	});

	it('keeps a name without a provider', () => {
		expect(namedCaption('cred-1', 'Work')).toBe('Work');
		expect(namedCaption('cred-1', 'Work', '   ')).toBe('Work');
	});

	it('keeps the full id when there is no name', () => {
		expect(namedCaption('a1b2c3d4e5f67890aabbccddeeff0011')).toBe(
			'a1b2c3d4e5f67890aabbccddeeff0011'
		);
		expect(namedCaption('a1b2c3d4e5f67890aabbccddeeff0011', '', 'ChatGPT')).toBe(
			'a1b2c3d4e5f67890aabbccddeeff0011'
		);
	});

	it('renders an empty key as a dash', () => {
		expect(namedCaption('')).toBe('—');
		expect(namedCaption('', 'Work', 'ChatGPT')).toBe('—');
	});
});

describe('accountCaption', () => {
	it('is the named caption for an account', () => {
		expect(accountCaption('cred-1', 'Work', 'ChatGPT')).toBe('Work (ChatGPT)');
	});
});
