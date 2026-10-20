import assert from 'node:assert/strict';
import { test } from 'node:test';

import { bar, bytes, duration } from './progress.mjs';

test('a bar fills up to the width and never past it', () => {
	assert.equal(bar(0, { width: 10 }), '░'.repeat(10));
	assert.equal(bar(0.5, { width: 10 }), '█'.repeat(5) + '░'.repeat(5));
	assert.equal(bar(1, { width: 10 }), '█'.repeat(10));
});

test('a bar clamps a fraction outside the whole', () => {
	assert.equal(bar(-1, { width: 4 }), '░'.repeat(4));
	assert.equal(bar(2, { width: 4 }), '█'.repeat(4));
});

test('a frame changes only the shade of whole blocks', () => {
	const before = { force: process.env.FORCE_COLOR, colorterm: process.env.COLORTERM };
	process.env.FORCE_COLOR = '1';
	process.env.COLORTERM = 'truecolor';
	try {
		const first = bar(0.5, { width: 8, color: true, frame: 0 });
		const second = bar(0.5, { width: 8, color: true, frame: 1 });
		const plain = (text) => text.replace(/\x1b\[[0-9;]*m/g, '');
		assert.notEqual(first, second);
		assert.equal(plain(first), plain(second));
		assert.equal(plain(first).length, 8);
		assert.match(plain(first), /^█+░*$/);
	} finally {
		if (before.force === undefined) delete process.env.FORCE_COLOR;
		else process.env.FORCE_COLOR = before.force;
		if (before.colorterm === undefined) delete process.env.COLORTERM;
		else process.env.COLORTERM = before.colorterm;
	}
});

test('bytes read the way a person reads them', () => {
	assert.equal(bytes(512), '512 B');
	assert.equal(bytes(2048), '2.0 KB');
	assert.equal(bytes(13_730_548), '13.1 MB');
});

test('durations read the way a person reads them', () => {
	assert.equal(duration(120), '120ms');
	assert.equal(duration(9200), '9.2s');
	assert.equal(duration(65_000), '1m 5s');
});
