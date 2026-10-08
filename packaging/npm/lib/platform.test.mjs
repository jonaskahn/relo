import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';

import { target } from './platform.mjs';

test('names the release asset for this host', () => {
	const { os, arch, name } = target();
	assert.ok(['darwin', 'linux', 'windows'].includes(os));
	assert.ok(['amd64', 'arm64'].includes(arch));
	assert.equal(name, `relo-${os}-${arch}${os === 'windows' ? '.exe' : ''}`);
});

test('resolves every asset the release publishes, from one machine', () => {
	const expected = {
		'darwin-arm64': 'relo-darwin-arm64',
		'darwin-x64': 'relo-darwin-amd64',
		'linux-arm64': 'relo-linux-arm64',
		'linux-x64': 'relo-linux-amd64',
		'win32-arm64': 'relo-windows-arm64.exe',
		'win32-x64': 'relo-windows-amd64.exe'
	};
	for (const [host, name] of Object.entries(expected)) {
		const [nodePlatform, nodeArch] = host.split('-');
		assert.equal(target({ platform: nodePlatform, arch: nodeArch }).name, name);
	}
});

test('names the host it cannot serve instead of only the status code', () => {
	assert.throws(() => target({ platform: 'freebsd', arch: 'x64' }), /freebsd-x64/);
	assert.throws(() => target({ platform: 'linux', arch: 'ia32' }), /linux-ia32/);
});

test('package.json os and cpu stay inside what the release ships', () => {
	const manifest = JSON.parse(
		readFileSync(fileURLToPath(new URL('../package.json', import.meta.url)), 'utf8')
	);
	const makefile = readFileSync(fileURLToPath(new URL('../../../Makefile', import.meta.url)), 'utf8');
	const built = makefile.match(/^PLATFORMS := (.+)$/m)[1].trim().split(/\s+/);
	const arch = { arm64: 'arm64', amd64: 'x64' };
	const os = { windows: 'win32' };
	const shipped = new Set(built.map((entry) => `${os[entry.split('/')[0]] ?? entry.split('/')[0]}-${arch[entry.split('/')[1]]}`));

	assert.ok(built.length > 0, 'the Makefile declares no PLATFORMS');
	for (const entry of built) {
		const [goos, goarch] = entry.split('/');
		assert.ok(manifest.os.includes(os[goos] ?? goos), `package.json os misses ${goos}`);
		assert.ok(manifest.cpu.includes(arch[goarch]), `package.json cpu misses ${goarch}`);
	}
	for (const platform of manifest.os) {
		for (const cpu of manifest.cpu) {
			assert.ok(shipped.has(`${platform}-${cpu}`), `nothing is released for ${platform}-${cpu}`);
		}
	}
});
