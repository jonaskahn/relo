import assert from 'node:assert/strict';
import { test } from 'node:test';

import { REPO, assets, checksumsUrl, tagFromArgs, url, version } from './release.mjs';

test('an unset tag follows the latest release', () => {
	assert.deepEqual(assets(), { base: `${REPO}/releases/latest/download`, version: null });
	assert.deepEqual(assets(null), { base: `${REPO}/releases/latest/download`, version: null });
	assert.equal(checksumsUrl(), `${REPO}/releases/latest/download/checksums.txt`);
});

test('a pinned tag pins the release with or without the v prefix', () => {
	assert.equal(version('v0.2.0'), '0.2.0');
	assert.equal(version('0.2.0'), '0.2.0');
	assert.equal(version('  v0.2.0  '), '0.2.0');
	assert.equal(version(undefined), null);
	assert.equal(version(''), null);
	assert.equal(assets('v0.2.0').base, `${REPO}/releases/download/v0.2.0`);
});

test('url names the asset inside the resolved release', () => {
	assert.equal(url('relo-linux-arm64', '0.2.0'), `${REPO}/releases/download/v0.2.0/relo-linux-arm64`);
	assert.equal(url('relo-windows-amd64.exe'), `${REPO}/releases/latest/download/relo-windows-amd64.exe`);
});

test('tagFromArgs reads --relo-version and ignores anything else', () => {
	assert.equal(tagFromArgs([]), null);
	assert.equal(tagFromArgs(['--relo-version', 'v0.2.0']), '0.2.0');
	assert.equal(tagFromArgs(['--relo-version=0.2.0']), '0.2.0');
	assert.equal(tagFromArgs(['--other', '--relo-version', '0.2.0']), '0.2.0');
	assert.equal(tagFromArgs(['--other']), null);
	assert.equal(tagFromArgs(['--relo-version']), null);
});
