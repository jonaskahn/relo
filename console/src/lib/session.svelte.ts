// Whether this console asks an operator to sign in. The daemon reports it
// with the session, and the shell reads it to choose between the sign-in page
// and the console itself.

function createSession() {
	// The daemon is the authority; a console that has not asked yet assumes
	// the guarded answer, so no page flashes for an operator who does need to
	// sign in.
	let loginRequired = $state(true);

	return {
		get loginRequired() {
			return loginRequired;
		},
		setLoginRequired(next: boolean) {
			loginRequired = next;
		}
	};
}

/** The shell's sign-in state. */
export const session = createSession();
