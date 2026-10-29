import assert from 'node:assert/strict';
import { chmodSync, mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { delimiter, join, relative, sep } from 'node:path';
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

// npm 10 cmd-shim invocation, including its batch-job termination workaround.
function npmShim(dir, launcher) {
	const command = reloCommand(dir, 'relo.cmd');
	const target = relative(dir, launcher).split(sep).join('\\');
	writeFileSync(command, [
		'@ECHO off',
		'GOTO start',
		':find_dp0',
		'SET dp0=%~dp0',
		'EXIT /b',
		':start',
		'SETLOCAL',
		'CALL :find_dp0',
		'IF EXIST "%dp0%\\node.exe" (',
		'  SET "_prog=%dp0%\\node.exe"',
		') ELSE (',
		'  SET "_prog=node"',
		'  SET PATHEXT=%PATHEXT:;.JS;=;%',
		')',
		'endLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & "%_prog%"  "%dp0%\\' + target + '" %*',
		''
	].join('\r\n'));
	return command;
}

test('Windows skips its npm shim on first install and reinstall with spaces in the prefix', (t) => {
	const prefix = join(tempDir(t, 'npm'), 'Author Software', 'nvm');
	const bin = join(prefix, 'node_modules', '@jonaskahn', 'relo', 'bin');
	npmShim(prefix, reloCommand(bin, 'relo.mjs'));
	const options = { env: { Path: prefix }, nodePlatform: 'win32', selfDir: bin };
	assert.equal(findConflictingRelo(options), null);
	reloCommand(bin, 'relo-windows-arm64.exe');
	assert.equal(findConflictingRelo(options), null);
});

test('Windows rejects npm shims pointing at another package or another launcher', (t) => {
	const prefix = tempDir(t, 'foreign-shim');
	const bin = join(prefix, 'node_modules', '@jonaskahn', 'relo', 'bin');
	reloCommand(bin, 'relo.mjs');
	const options = { env: { PATH: prefix }, nodePlatform: 'win32', selfDir: bin };
	for (const launcher of [
		reloCommand(join(prefix, 'other'), 'relo.mjs'),
		reloCommand(bin, 'other.mjs'),
		join(prefix, 'missing', 'relo.mjs')
	]) {
		const command = npmShim(prefix, launcher);
		assert.equal(findConflictingRelo(options), command);
	}
});

test('Windows rejects unrecognized shims even when they mention its launcher', (t) => {
	const prefix = tempDir(t, 'unknown-shim');
	const bin = join(prefix, 'node_modules', '@jonaskahn', 'relo', 'bin');
	const launcher = reloCommand(bin, 'relo.mjs');
	const command = reloCommand(prefix, 'relo.cmd');
	writeFileSync(command, '@echo off\r\nREM ' + launcher + '\r\nother.exe %*\r\n');
	assert.equal(findConflictingRelo({
		env: { Path: prefix }, nodePlatform: 'win32', selfDir: bin
	}), command);
});

test('Windows still checks foreign commands after its own npm shim', (t) => {
	const prefix = tempDir(t, 'own-shim');
	const bin = join(prefix, 'node_modules', '@jonaskahn', 'relo', 'bin');
	npmShim(prefix, reloCommand(bin, 'relo.mjs'));
	const foreignDir = tempDir(t, 'foreign-exe');
	const foreign = reloCommand(foreignDir, 'relo.exe');
	assert.equal(findConflictingRelo({
		env: { PATH: [prefix, foreignDir].join(delimiter) },
		nodePlatform: 'win32',
		selfDir: bin
	}), foreign);
});

test('findConflictingRelo is null with no foreign command', (t) => {
	const empty = tempDir(t, 'empty');
	assert.equal(findConflictingRelo({ env: { PATH: empty }, nodePlatform: 'linux' }), null);
	assert.equal(findConflictingRelo({ env: {}, nodePlatform: 'linux' }), null);
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
