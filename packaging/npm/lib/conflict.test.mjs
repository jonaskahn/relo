import assert from 'node:assert/strict';
import { chmodSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { delimiter, join } from 'node:path';
import { test } from 'node:test';

import { candidates, findConflictingRelo, findReloApp, findSetupInstall } from './conflict.mjs';

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

test('Windows treats its npm global bin as this install, even with a linked prefix', (t) => {
	const root = tempDir(t, 'global');
	const real = join(root, 'installs', 'v22.23.4');
	const bin = join(real, 'node_modules', '@jonaskahn', 'relo', 'bin');
	mkdirSync(bin, { recursive: true });
	const link = join(root, 'Author Software', '.nodejs');
	mkdirSync(join(root, 'Author Software'));
	symlinkSync(real, link);
	reloCommand(real, 'relo.exe');
	reloCommand(real, 'relo.cmd');
	for (const env of [{ Path: real }, { Path: link }]) {
		assert.equal(findConflictingRelo({ env, nodePlatform: 'win32', selfDir: bin }), null);
	}
});

test('Windows still rejects a relo beside another node_modules', (t) => {
	const bin = join(tempDir(t, 'owned'), 'node_modules', '@jonaskahn', 'relo', 'bin');
	mkdirSync(bin, { recursive: true });
	const otherDir = tempDir(t, 'other-prefix');
	const other = reloCommand(otherDir, 'relo.exe');
	assert.equal(findConflictingRelo({ env: { Path: otherDir }, nodePlatform: 'win32', selfDir: bin }), other);
});

test('findReloApp reports the bundle in Applications on macOS', (t) => {
	const apps = tempDir(t, 'apps');
	mkdirSync(join(apps, 'Relo.app'), { recursive: true });

	assert.equal(
		findReloApp({ nodePlatform: 'darwin', applications: apps, home: tempDir(t, 'home') }),
		join(apps, 'Relo.app')
	);
});

test('findReloApp reports the bundle in the user Applications on macOS', (t) => {
	const home = tempDir(t, 'userhome');
	mkdirSync(join(home, 'Applications', 'Relo.app'), { recursive: true });

	assert.equal(
		findReloApp({ nodePlatform: 'darwin', applications: tempDir(t, 'empty'), home }),
		join(home, 'Applications', 'Relo.app')
	);
});

test('findReloApp is null without a bundle, or off macOS', (t) => {
	const apps = tempDir(t, 'bundled');
	mkdirSync(join(apps, 'Relo.app'), { recursive: true });

	assert.equal(
		findReloApp({ nodePlatform: 'darwin', applications: tempDir(t, 'empty'), home: tempDir(t, 'home') }),
		null
	);
	assert.equal(findReloApp({ nodePlatform: 'linux', applications: apps }), null);
	assert.equal(findReloApp({ nodePlatform: 'win32', applications: apps }), null);
});

test('findSetupInstall reads the uninstall registry on Windows only', () => {
	assert.equal(findSetupInstall({ nodePlatform: 'win32', query: () => true }), true);
	assert.equal(findSetupInstall({ nodePlatform: 'win32', query: () => false }), false);
	assert.equal(
		findSetupInstall({
			nodePlatform: 'linux',
			query: () => {
				throw new Error('must not query the registry off Windows');
			}
		}),
		false
	);
});
