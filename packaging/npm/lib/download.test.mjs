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

test('a retry is reported to the caller before the next attempt', async () => {
	let calls = 0;
	const fetchImpl = async () => {
		calls += 1;
		if (calls === 1) throw new Error('boom');
		return response(200, 'ok');
	};
	const retries = [];
	const body = await fetchBytes('https://example.invalid/a', 'a', {
		fetchImpl,
		sleep: noSleep,
		onRetry: (message) => retries.push(message)
	});
	assert.equal(body.toString(), 'ok');
	assert.deepEqual(retries, ['a failed (boom), retrying']);
});

test('a streamed body reports the bytes as they arrive', async () => {
	const chunks = [Buffer.from('abc'), Buffer.from('de')];
	const fetchImpl = async () => ({
		ok: true,
		status: 200,
		statusText: 'OK',
		headers: { get: () => '5' },
		body: {
			getReader: () => {
				let index = 0;
				return {
					read: async () =>
						index < chunks.length
							? { done: false, value: chunks[index++] }
							: { done: true, value: undefined }
				};
			}
		}
	});
	const seen = [];
	const body = await fetchBytes('https://example.invalid/a', 'relo.tar.gz', {
		fetchImpl,
		sleep: noSleep,
		onProgress: (received, total) => seen.push([received, total])
	});
	assert.equal(body.toString(), 'abcde');
	assert.deepEqual(seen, [[3, 5], [5, 5]]);
});

test('a body without a stream is read whole and reports nothing', async () => {
	const fetchImpl = async () => response(200, 'ok');
	const seen = [];
	const body = await fetchBytes('https://example.invalid/a', 'a', {
		fetchImpl,
		sleep: noSleep,
		onProgress: (received) => seen.push(received)
	});
	assert.equal(body.toString(), 'ok');
	assert.deepEqual(seen, []);
});
