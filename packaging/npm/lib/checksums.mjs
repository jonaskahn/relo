// Verifies a downloaded asset against the digest the release publishes beside
// it, so a tampered or truncated download fails the install instead of being
// written into the package as an executable.
import { createHash } from 'node:crypto';

// digestOf is the lowercase hex sha256 of a buffer.
export function digestOf(body) {
	return createHash('sha256').update(body).digest('hex');
}

// parse reads the `sha256  name` lines of a sha256sum file into a lookup. It
// accepts the two-column form both shasum and sha256sum emit, with one or two
// spaces and the optional leading asterisk binary mode adds.
export function parse(manifest) {
	const entries = new Map();
	for (const line of manifest.toString('utf8').split('\n')) {
		const match = /^([0-9a-f]{64})\s+\*?(.+?)\s*$/.exec(line);
		if (match) entries.set(match[2], match[1]);
	}
	return entries;
}

// expect finds the digest for one asset and checks the body against it.
export function expect(entries, name, body) {
	const expected = entries.get(name);
	if (!expected) throw new Error(`${name} is not listed in the release checksums`);
	const actual = digestOf(body);
	if (actual !== expected) {
		throw new Error(`${name} does not match its published sha256. expected ${expected}, got ${actual}`);
	}
}
