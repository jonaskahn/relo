import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, test } from 'node:test';

import {
	decodeKeys,
	isReloImage,
	isReloProcess,
	move,
	parseLsof,
	parseNetstat,
	parseSs,
	quiesce,
	runtimePid,
	servesHome,
	servicePorts
} from './daemon.mjs';

test('service ports fall back to the daemon default', () => {
	assert.deepEqual(servicePorts(''), [10101]);
});

test('service ports follow config.toml, and a zero port names no scan', () => {
	const config = ['[server]', 'port = 19101'].join('\n');
	assert.deepEqual(servicePorts(config), [19101]);
	assert.deepEqual(servicePorts('[server]\nport = 0'), []);
});

test('runtime pid reads the published pid, or null', () => {
	assert.equal(runtimePid('{"address":"127.0.0.1:10101","instance_id":"x","pid":77329}'), 77329);
	assert.equal(runtimePid('not json'), null);
	assert.equal(runtimePid('{}'), null);
});

test('lsof output reads as one pid per line', () => {
	assert.deepEqual(parseLsof('65631\n65700\n'), [65631, 65700]);
	assert.deepEqual(parseLsof(''), []);
});

test('ss output collects every pid on the named port only', () => {
	const ss = [
		'LISTEN 0 128 127.0.0.1:19101 0.0.0.0:* users:(("relo",pid=123,fd=9))',
		'LISTEN 0 128 127.0.0.1:19201 0.0.0.0:* users:(("node",pid=456,fd=9))',
		'LISTEN 0 128 127.0.0.1:191011 0.0.0.0:* users:(("other",pid=789,fd=9))'
	].join('\n');
	assert.deepEqual(parseSs(ss, 19101), [123]);
});

test('netstat output pairs a listening port with its owner', () => {
	const netstat = [
		'  TCP    127.0.0.1:10101        0.0.0.0:0              LISTENING       1111',
		'  TCP    [::]:10201            [::]:0                 LISTENING       2222',
		'  TCP    127.0.0.1:10101        127.0.0.1:50000        ESTABLISHED     3333'
	].join('\n');
	assert.deepEqual(parseNetstat(netstat, 10101), [1111]);
	assert.deepEqual(parseNetstat(netstat, 10201), [2222]);
	assert.deepEqual(parseNetstat(netstat, 9999), []);
});

test('the relo image matches by basename, with an exe on windows', () => {
	assert.equal(isReloImage('relo'), true);
	assert.equal(isReloImage('/Applications/Relo.app/Contents/MacOS/relo'), true);
	assert.equal(isReloImage('C:\\Relo\\relo.exe'), true);
	assert.equal(isReloImage('python3'), false);
	assert.equal(isReloImage('relocation'), false);
});

test('the relo image also matches the platform-named release binaries', () => {
	assert.equal(isReloImage('/opt/relo/bin/relo-darwin-arm64'), true);
	assert.equal(isReloImage('/Volumes/WS/relo/dist/.pkgcache/relo-darwin-arm64-0.1.1'), true);
	assert.equal(isReloImage('relo-linux-amd64'), true);
	assert.equal(isReloImage('relo-windows-amd64.exe'), true);
	assert.equal(isReloImage('relo-windows-amd64-0.1.1.exe'), true);
	assert.equal(isReloImage('relo-helper'), false);
	assert.equal(isReloImage('relo-darwin-arm64-notes.txt'), false);
});

test('a truncated command name falls back to the command line', () => {
	// Linux truncates comm to fifteen characters.
	assert.equal(isReloProcess({ comm: 'relo-linux-arm6', args: '/opt/relo/relo-linux-arm64 daemon run' }), true);
	assert.equal(isReloProcess({ comm: 'python3', args: '/usr/bin/python3 /opt/relo/relo-linux-amd64' }), false);
	assert.equal(isReloProcess({ comm: 'relo-linux-amd6', args: '' }), false);
});

test('a process serves its home by runtime pid or command line', () => {
	const home = '/Users/jonas/.relo';
	const args = `/Applications/Relo.app/Contents/MacOS/relo daemon run --home ${home} --port 10101`;
	assert.equal(servesHome({ pid: 1, owner: 1, args: 'other', home, defaultHome: home }), true);
	assert.equal(servesHome({ pid: 1, owner: 2, args, home, defaultHome: home }), true);
	assert.equal(
		servesHome({ pid: 1, owner: 0, args: 'relo daemon run --home /tmp/other', home, defaultHome: home }),
		false
	);
	assert.equal(servesHome({ pid: 1, owner: 0, args: 'relo daemon run', home, defaultHome: home }), true);
	assert.equal(
		servesHome({ pid: 1, owner: 0, args: 'relo daemon run', home: '/tmp/other', defaultHome: home }),
		false
	);
});

// A home on a port of its own, so a quiesce never looks at the port a developer's
// own daemon serves.
function scratchHome(port) {
	const home = mkdtempSync(join(tmpdir(), 'relo-quiesce-'));
	writeFileSync(join(home, 'config.toml'), ['[server]', `port = ${port}`].join('\n'));
	return home;
}

test('a quiesce of a home with nothing running is not a failure', () => {
	const home = scratchHome(49101);
	assert.deepEqual(quiesce(home), { stopped: false });
});

test('a quiesce fails when another program still listens on a configured port', async () => {
	const home = scratchHome(49111);
	const holder = createServer();
	await new Promise((resolve) => holder.listen(49111, '127.0.0.1', resolve));
	after(() => holder.close());
	assert.throws(() => quiesce(home), /something else is listening on port 49111/);
});

test('arrow keys decode in both forms a terminal sends them', () => {
	assert.deepEqual(decodeKeys('', '\x1b[C\x1b[D'), { keys: ['right', 'left'], pending: '' });
	assert.deepEqual(decodeKeys('', '\x1bOC\x1bOD'), { keys: ['right', 'left'], pending: '' });
});

test('a split escape sequence is carried to the next chunk', () => {
	assert.deepEqual(decodeKeys('', '\x1b'), { keys: [], pending: '\x1b' });
	assert.deepEqual(decodeKeys('\x1b', '['), { keys: [], pending: '\x1b[' });
	assert.deepEqual(decodeKeys('\x1b[', 'C'), { keys: ['right'], pending: '' });
	assert.deepEqual(decodeKeys('\x1b', 'OD'), { keys: ['left'], pending: '' });
	assert.deepEqual(decodeKeys('', '\x1b[C\x1b['), { keys: ['right'], pending: '\x1b[' });
});

test('enter and interrupt are the keys that answer', () => {
	assert.deepEqual(decodeKeys('', '\r').keys, ['enter']);
	assert.deepEqual(decodeKeys('', '\n').keys, ['enter']);
	assert.deepEqual(decodeKeys('', '\x03').keys, ['interrupt']);
});

test('unrelated keys and modified arrows are ignored', () => {
	for (const chunk of [
		'\x1b[A',
		'\x1b[B',
		'\x1b[H',
		'\x1b[F',
		'\x1b[3~',
		'\x1b[1;2D',
		'\x1b[1;5C',
		'\x1bx',
		'x',
		' '
	]) {
		assert.deepEqual(decodeKeys('', chunk), { keys: [], pending: '' }, chunk);
	}
});

test('an escape prefix never swallows the control after it', () => {
	assert.deepEqual(decodeKeys('\x1b', '\x1b[C'), { keys: ['right'], pending: '' });
	assert.deepEqual(decodeKeys('\x1b', '\r'), { keys: ['enter'], pending: '' });
	assert.deepEqual(decodeKeys('', '\x1b[\r'), { keys: ['enter'], pending: '' });
	assert.deepEqual(decodeKeys('', '\x1bO\r'), { keys: ['enter'], pending: '' });
});

test('a menu selection wraps at both ends', () => {
	assert.equal(move(0, 2, -1), 1);
	assert.equal(move(1, 2, 1), 0);
	assert.equal(move(0, 3, -1), 2);
	assert.equal(move(2, 3, 1), 0);
	assert.equal(move(0, 0, 1), 0);
});
