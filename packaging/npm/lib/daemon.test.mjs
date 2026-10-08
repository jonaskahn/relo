import assert from 'node:assert/strict';
import { mkdtempSync, writeFileSync } from 'node:fs';
import { createServer } from 'node:net';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { after, test } from 'node:test';

import {
	isReloImage,
	parseLsof,
	parseNetstat,
	parseSs,
	quiesce,
	runtimePid,
	servesHome,
	servicePorts
} from './daemon.mjs';

test('service ports fall back to the daemon defaults', () => {
	assert.deepEqual(servicePorts(''), [10101, 10201, 10202, 10203]);
});

test('service ports follow config.toml and skip a zeroed protocol', () => {
	const config = ['[server]', 'port = 19101', '[server.data_plane]', 'openai = 19201', 'anthropic = 0'].join(
		'\n'
	);
	assert.deepEqual(servicePorts(config), [19101, 19201, 10203]);
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

// A home on ports of its own, so a quiesce never looks at the ports a developer's
// own daemon serves.
function scratchHome(ports) {
	const home = mkdtempSync(join(tmpdir(), 'relo-quiesce-'));
	writeFileSync(
		join(home, 'config.toml'),
		['[server]', `port = ${ports[0]}`, '[server.data_plane]', `openai = ${ports[1]}`, `anthropic = ${ports[2]}`, `gemini = ${ports[3]}`].join('\n')
	);
	return home;
}

test('a quiesce of a home with nothing running is not a failure', () => {
	const home = scratchHome([49101, 49201, 49202, 49203]);
	assert.deepEqual(quiesce(home), { stopped: false });
});

test('a quiesce fails when another program still listens on a configured port', async () => {
	const home = scratchHome([49111, 49211, 49212, 49213]);
	const holder = createServer();
	await new Promise((resolve) => holder.listen(49111, '127.0.0.1', resolve));
	after(() => holder.close());
	assert.throws(() => quiesce(home), /something else is listening on port 49111/);
});
