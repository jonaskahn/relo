import assert from 'node:assert/strict';
import { test } from 'node:test';

import { digestOf, expect, parse } from './checksums.mjs';

const body = Buffer.from('relo');
const digest = digestOf(body);
const manifest = [`${digest}  relo-linux-arm64`, `${digest} *relo-darwin-amd64`].join('\n') + '\n';

test('parses the two-column shasum format', () => {
	const entries = parse(manifest);
	assert.equal(entries.get('relo-linux-arm64'), digest);
	assert.equal(entries.get('relo-darwin-amd64'), digest);
});

test('ignores blank lines and entries that are not sha256', () => {
	const entries = parse(['', `${digest}  relo-linux-amd64`, 'not-a-digest  relo-darwin-amd64'].join('\n'));
	assert.equal(entries.size, 1);
	assert.equal(entries.has('relo-darwin-amd64'), false);
});

test('accepts the matching body and rejects a changed one', () => {
	const entries = parse(manifest);
	assert.doesNotThrow(() => expect(entries, 'relo-linux-arm64', body));
	assert.throws(() => expect(entries, 'relo-linux-arm64', Buffer.from('other')), /does not match/);
	assert.throws(() => expect(entries, 'relo-windows-amd64.exe', body), /not listed/);
});
