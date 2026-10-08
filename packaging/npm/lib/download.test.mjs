import assert from 'node:assert/strict';
import { test } from 'node:test';

import { fetchBytes } from './download.mjs';

function response(status, body = 'x') {
	return {
		ok: status >= 200 && status < 300,
		status,
		statusText: status === 404 ? 'Not Found' : 'Error',
		arrayBuffer: async () => Buffer.from(body)
	};
}

const noSleep = async () => {};

test('a 404 fails fast with guidance, without retrying', async () => {
	let calls = 0;
	const fetchImpl = async () => {
		calls += 1;
		return response(404);
	};
	await assert.rejects(
		() => fetchBytes('https://example.invalid/a', 'relo-darwin-arm64', { fetchImpl, sleep: noSleep }),
		/no relo-darwin-arm64 in the release \(HTTP 404\)/
	);
	assert.equal(calls, 1);
});

test('a 429 is retried, then reported like any other transient failure', async () => {
	let calls = 0;
	const fetchImpl = async () => {
		calls += 1;
		return response(429);
	};
	await assert.rejects(
		() => fetchBytes('https://example.invalid/a', 'checksums.txt', { fetchImpl, sleep: noSleep }),
		/could not download checksums.txt from https:\/\/example\.invalid\/a/
	);
	assert.equal(calls, 3);
});

test('a 503 is retried and a later attempt wins', async () => {
	let calls = 0;
	const fetchImpl = async () => {
		calls += 1;
		return calls < 3 ? response(503) : response(200, 'ok');
	};
	const body = await fetchBytes('https://example.invalid/a', 'a', { fetchImpl, sleep: noSleep });
	assert.equal(body.toString(), 'ok');
	assert.equal(calls, 3);
});

test('a network error is retried, then reported with the url', async () => {
	let calls = 0;
	const fetchImpl = async () => {
		calls += 1;
		throw new Error('boom');
	};
	await assert.rejects(
		() => fetchBytes('https://example.invalid/a', 'checksums.txt', { fetchImpl, sleep: noSleep }),
		/could not download checksums\.txt from https:\/\/example\.invalid\/a: boom/
	);
	assert.equal(calls, 3);
});
