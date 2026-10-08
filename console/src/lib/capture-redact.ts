// The console masks a credential in a captured message before it draws it.
// Captures stored before the daemon masked them, or by an older daemon, would
// otherwise print a live key into a log an operator copies and shares.

// SENSITIVE_HEADERS names the headers that always carry a credential; the
// daemon masks the same list, so a capture reads the same whichever side
// redacted it. The names are compared without dashes, underscores, or case.
const SENSITIVE_HEADERS = new Set([
	'authorization',
	'proxyauthorization',
	'apikey',
	'xapikey',
	'xgoogapikey',
	'xamzsecuritytoken',
	'xapitoken',
	'xauthtoken',
	'xxaitokenauth',
	'authentication',
	'password',
	'cookie',
	'setcookie'
]);

// SENSITIVE_SUFFIXES catch a credential a vendor sent under a name of its own.
// They are matched against the end of the normalized name, so a header about
// how many tokens are left is not a token.
const SENSITIVE_SUFFIXES = ['apikey', 'secret', 'password', 'token'];

/** Reports whether one header name carries a credential. */
export function sensitiveHeader(name: string): boolean {
	const normalized = name.toLowerCase().replace(/[-_\s]/g, '');
	if (SENSITIVE_HEADERS.has(normalized)) return true;
	return SENSITIVE_SUFFIXES.some((suffix) => normalized.endsWith(suffix));
}

// SCHEME is the leading word of a credential value, which says how the
// credential was presented and is not itself a secret.
const SCHEME = /^([A-Za-z]{2,12})\s+\S/;

// MASK is what the secret behind a scheme becomes.
const MASK = '••••';

/** Hides the secret of one credential value. A value sent with a scheme keeps its scheme;
 *  anything else becomes the mask alone. */
export function maskCredential(value: string): string {
	const trimmed = value.trim();
	if (trimmed === '') return value;
	const scheme = SCHEME.exec(trimmed);
	if (!scheme) return MASK;
	return scheme[1] + ' ' + MASK;
}

/** Returns the headers a capture may show: every value of a header that names a credential is
 *  masked, and the rest are untouched. */
export function redactHeaders(headers: Record<string, string[]>): Record<string, string[]> {
	const redacted: Record<string, string[]> = {};
	for (const [name, values] of Object.entries(headers ?? {})) {
		redacted[name] = sensitiveHeader(name) ? values.map(maskCredential) : values;
	}
	return redacted;
}
