import assert from 'node:assert/strict';
import { chmodSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { delimiter, join } from 'node:path';
import { test } from 'node:test';

import { candidates, findConflictingRelo } from './conflict.mjs';

// reloCommand writes a stub relo command into dir and returns its path.
function reloCommand(dir, file = 'relo') {
	mkdirSync(dir, { recursive: true });
	const binary = join(dir, file);
	writeFileSync(binary, '#!/bin/sh\nexit 0\n');
	chmodSync(binary, 0o755);
	return binary;
}

function tempDir(t, name) {
	const dir = mkdtempSync(join(tmpdir(), `relo-conflict-${name}-`));
	t.after(() => rmSync(dir, { recursive: true, force: true }));
	return dir;
}

test('candidates lists the relo commands a PATH resolves, in order', (t) => {
	const first = tempDir(t, 'first');
	const second = tempDir(t, 'second');
	const empty = tempDir(t, 'empty');
	reloCommand(first);
	reloCommand(second);

	const pathValue = [first, empty, second].join(delimiter);
	assert.deepEqual(candidates({ pathValue }), [join(first, 'relo'), join(second, 'relo')]);
});

test('candidates skips a directory entry that does not exist or is not executable', (t) => {
	const dir = tempDir(t, 'plain');
	writeFileSync(join(dir, 'relo'), 'not executable');
	chmodSync(join(dir, 'relo'), 0o644);

	assert.deepEqual(candidates({ pathValue: [dir, join(dir, 'missing')].join(delimiter) }), []);
});

test('candidates on Windows lists the installer commands, relo.exe and relo.cmd', (t) => {
	const exeDir = tempDir(t, 'win-exe');
	const exe = reloCommand(exeDir, 'relo.exe');

	assert.deepEqual(candidates({ pathValue: exeDir, nodePlatform: 'win32' }), [exe]);
	assert.deepEqual(candidates({ pathValue: exeDir, nodePlatform: 'linux' }), [], 'the .exe is not a POSIX command');

	const cmdDir = tempDir(t, 'win-cmd');
	const cmd = reloCommand(cmdDir, 'relo.cmd');

	assert.deepEqual(candidates({ pathValue: cmdDir, nodePlatform: 'win32' }), [cmd], 'the installer command is relo.cmd');

	const command = reloCommand(exeDir);
	assert.deepEqual(candidates({ pathValue: exeDir, nodePlatform: 'linux' }), [command]);
});

test('findConflictingRelo reports the first foreign command', (t) => {
	const first = tempDir(t, 'a');
	const second = tempDir(t, 'b');
	const command = reloCommand(first);
	reloCommand(second);

	assert.equal(
		findConflictingRelo({ env: { PATH: [first, second].join(delimiter) }, nodePlatform: 'linux' }),
		command
	);
});

test('findConflictingRelo reads Path when PATH is absent, as Windows exposes it', (t) => {
	const dir = tempDir(t, 'pathcase');
	const expected = reloCommand(dir, 'relo.exe');

	assert.equal(findConflictingRelo({ env: { Path: dir }, nodePlatform: 'win32' }), expected);
	assert.equal(findConflictingRelo({ env: {}, nodePlatform: 'linux' }), null);
});

test('findConflictingRelo skips this package own install', (t) => {
	const bin = join(tempDir(t, 'pkg'), 'bin');
	reloCommand(bin);
	const foreignDir = tempDir(t, 'other');
	const foreign = reloCommand(foreignDir);

	assert.equal(
		findConflictingRelo({
			env: { PATH: [bin, foreignDir].join(delimiter) },
			nodePlatform: 'linux',
			selfDir: bin
		}),
		foreign
	);
});

test('findConflictingRelo follows a shim link into the package', (t) => {
	const bin = join(tempDir(t, 'linked'), 'bin');
	const own = reloCommand(bin);
	const shimDir = tempDir(t, 'shim');
	symlinkSync(own, join(shimDir, 'relo'));

	assert.equal(
		findConflictingRelo({ env: { PATH: shimDir }, nodePlatform: 'linux', selfDir: bin }),
		null
	);
});

test('findConflictingRelo is null with no foreign command', (t) => {
	const empty = tempDir(t, 'empty');
	assert.equal(findConflictingRelo({ env: { PATH: empty }, nodePlatform: 'linux' }), null);
	assert.equal(findConflictingRelo({ env: {}, nodePlatform: 'linux' }), null);
});
