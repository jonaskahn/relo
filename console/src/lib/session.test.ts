import { describe, expect, it } from 'vitest';

import { session } from './session.svelte';

// The daemon is the authority on whether a console asks for a sign-in. Until
// it answers, the shell has to assume the guarded answer: the other way round
// would flash a page at an operator who does have to sign in.

describe('session', () => {
	it('assumes the daemon asks for a sign-in until it says otherwise', () => {
		expect(session.loginRequired).toBe(true);
	});

	it('takes the daemon at its word', () => {
		session.setLoginRequired(false);
		expect(session.loginRequired).toBe(false);
		session.setLoginRequired(true);
		expect(session.loginRequired).toBe(true);
	});
});
