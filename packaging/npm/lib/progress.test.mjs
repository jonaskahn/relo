import assert from 'node:assert/strict';
import { test } from 'node:test';

import { bar, buttons, bytes, duration, errorBox } from './progress.mjs';

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

// withColor runs render under the given color environment, then restores it.
function withColor({ force, colorterm, noColor }, render) {
	const before = {
		force: process.env.FORCE_COLOR,
		colorterm: process.env.COLORTERM,
		noColor: process.env.NO_COLOR
	};
	const set = (name, value) => {
		if (value === undefined) delete process.env[name];
		else process.env[name] = value;
	};
	set('FORCE_COLOR', force);
	set('COLORTERM', colorterm);
	set('NO_COLOR', noColor);
	try {
		return render();
	} finally {
		set('FORCE_COLOR', before.force);
		set('COLORTERM', before.colorterm);
		set('NO_COLOR', before.noColor);
	}
}

test('buttons mark the selected option without moving the row', () => {
	withColor({ noColor: '1' }, () => {
		assert.equal(buttons(['YES', 'NO'], 0), '▶ YES ◀  [ NO ]');
		assert.equal(buttons(['YES', 'NO'], 1), '[ YES ]  ▶ NO ◀');
	});
});

test('a painted button keeps the same text and width', () => {
	const plain = withColor({ noColor: '1' }, () => buttons(['YES', 'NO'], 0));
	const truecolor = withColor({ force: '1', colorterm: 'truecolor' }, () =>
		buttons(['YES', 'NO'], 0)
	);
	const strip = (text) => text.replace(/\x1b\[[0-9;]*m/g, '');
	assert.match(truecolor, /\x1b\[48;2;155;188;15m/);
	assert.equal(strip(truecolor), plain);

	const inverse = withColor({ force: '1' }, () => buttons(['YES', 'NO'], 1));
	assert.match(inverse, /\x1b\[7m/);
	assert.equal(strip(inverse), '[ YES ]  ▶ NO ◀');
});

test('a failed install is a block below the screen', () => {
	const box = withColor({ noColor: '1' }, () =>
		errorBox(
			'INSTALL FAILED',
			'something else is listening on port 10101\n' +
				'  free the port, or point relo at another one, and install again'
		)
	);
	const lines = box.split('\n');
	assert.equal(lines[0], `┌${'─'.repeat(76)}┐`);
	assert.equal(lines.at(-1), `└${'─'.repeat(76)}┘`);
	assert.match(lines[1], /^│ ✗ INSTALL FAILED /);
	assert.ok(lines.some((line) => line.includes('something else is listening')));
	for (const line of lines) assert.equal(line.length, 78, line);
});

test('a long failure message wraps inside the block', () => {
	const path = `/very/long/path${'/that/keeps/going'.repeat(5)}/relo`;
	const box = withColor({ noColor: '1' }, () =>
		errorBox(
			'INSTALL FAILED',
			`another Relo install provides the relo command at ${path}\n` +
				'  remove that install first to install this package, or keep using that install instead'
		)
	);
	for (const line of box.split('\n')) assert.equal(line.length, 78, line);
	assert.ok(box.includes('remove that install first'));
	const content = box
		.split('\n')
		.slice(1, -1)
		.map((line) => line.slice(2, -2))
		.join('')
		.replace(/\s+/g, '');
	assert.ok(content.includes(path), content);
});

test('the failure block is red when color is on', () => {
	const plain = withColor({ noColor: '1' }, () => errorBox('INSTALL FAILED', 'boom'));
	const colored = withColor({ force: '1' }, () => errorBox('INSTALL FAILED', 'boom'));
	const strip = (text) => text.replace(/\x1b\[[0-9;]*m/g, '');
	assert.match(colored, /\x1b\[31m/);
	assert.match(colored, /\x1b\[1;31m/);
	assert.equal(strip(colored), plain);
});
