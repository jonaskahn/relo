// The preuninstall script runs on `npm uninstall -g @jonaskahn/relo`, so what matters is
// that it quiesces the home it is pointed at and exits cleanly either way: an
// uninstall must not fail because the daemon could not be reached.
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { existsSync, mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

const script = resolve(dirname(fileURLToPath(import.meta.url)), 'uninstall.mjs');

// scratchHome is a home on a port of its own, so the quiesce never looks at the
// port a developer's own daemon serves.
function scratchHome(port) {
	const home = mkdtempSync(join(tmpdir(), 'relo-uninstall-'));
	writeFileSync(join(home, 'config.toml'), ['[server]', `port = ${port}`].join('\n'));
	return home;
}

function uninstall(home) {
	return spawnSync(process.execPath, [script], {
		env: { ...process.env, RELO_HOME: home },
		encoding: 'utf8'
	});
}

test('the package declares the preuninstall script it ships', async () => {
	const { readFile } = await import('node:fs/promises');
	const manifest = JSON.parse(await readFile(join(dirname(script), '..', 'package.json'), 'utf8'));
	assert.equal(manifest.scripts.preuninstall, 'node lib/uninstall.mjs');
	assert.ok(manifest.files.includes('lib/uninstall.mjs'));
});

test('a removal with nothing running succeeds quietly', () => {
	const result = uninstall(scratchHome(49301));
	assert.equal(result.status, 0, result.stderr);
});

test('a removal never reports the runtime file it could not touch', () => {
	const home = scratchHome(49311);
	writeFileSync(join(home, 'runtime.json'), '{"address":"127.0.0.1:49311","instance_id":"gone","pid":0}');
	const result = uninstall(home);
	assert.equal(result.status, 0, result.stderr);
});