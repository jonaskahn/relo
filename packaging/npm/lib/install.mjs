// Puts the binary for this host inside the installed package. The launcher
// in bin/ execs it, so the daemon and its tray run as their own process and
// do not need node at run time. The install refuses a machine where another
// Relo already owns the relo command, whether an installer or an older
// package put it there: two installs would fight over the name, the home
// directory, and the ports. A relo running from this home is quiesced before
// its binary is replaced -- its daemon is stopped and the ports it served
// are freed -- and the install asks whether to start the new one.
//
// The binary always comes from a GitHub release: the latest one, or the tag
// --relo-version names. Rerun this file by hand with the flag to install a
// specific release.
import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { mkdir, rename, rm, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { expect, parse } from './checksums.mjs';
import { askStart, quiesce, reloHome } from './daemon.mjs';
import { findConflictingRelo } from './conflict.mjs';
import { fetchBytes } from './download.mjs';
import { target } from './platform.mjs';
import { assets, checksumsUrl, tagFromArgs, url } from './release.mjs';

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const binaryDir = resolve(packageRoot, 'bin');
const home = reloHome();

// writeAtomic lands the binary in one rename, so an interrupted install leaves
// no truncated executable behind for the launcher to find.
async function writeAtomic(destination, body) {
	const staging = `${destination}.download`;
	await writeFile(staging, body, { mode: 0o755 });
	await rename(staging, destination);
}

async function fromRelease(tag) {
	const asset = target();
	const { version } = assets(tag);

	// One manifest lists every asset in the release, so a single fetch
	// verifies the binary before anything executable is written.
	const manifest = await fetchBytes(checksumsUrl(tag), 'checksums.txt');
	const body = await fetchBytes(url(asset.name, tag), asset.name);
	expect(parse(manifest), asset.name, body);
	const destination = resolve(binaryDir, asset.name);
	// Quiesced first: on windows the rename below fails over a running exe, and
	// the new daemon cannot bind a port the old one still serves.
	const { stopped } = quiesce(home, { binary: existsSync(destination) ? destination : undefined });
	await mkdir(binaryDir, { recursive: true });
	await writeAtomic(destination, body);
	console.log(`relo: installed the ${version ? `v${version} ` : ''}daemon binary for ${asset.os}-${asset.arch}`);
	console.log(stopped ? 'relo: stopped the running daemon' : 'relo: not running');
	return destination;
}

try {
	// One binary answers the relo command now, whether it came from this
	// package or an installer, so a command this package did not install
	// stops this one: it already provides the name, and a second Relo would
	// fight it for the home directory and the ports. A previous install from
	// this package is simply replaced below.
	const conflict = findConflictingRelo({ selfDir: binaryDir });
	if (conflict) {
		throw new Error(
			`another Relo install provides the relo command at ${conflict}\n` +
				'  remove that install first to install this package, or keep using that install instead'
		);
	}
	const destination = await fromRelease(tagFromArgs(process.argv.slice(2)));
	const answer = await askStart();
	if (answer === 'yes') {
		const started = spawnSync(destination, ['daemon', 'start'], { stdio: 'inherit' });
		if (started.status !== 0) {
			console.warn('relo: could not start the daemon; run relo daemon start to try again');
		}
	} else {
		console.log('relo: start it later with: relo daemon start');
	}
} catch (error) {
	// Remove only the staging file this install wrote. Removing the directory
	// would take bin/relo.mjs with it, leaving a package whose `relo` command
	// no longer exists.
	await rm(resolve(binaryDir, `${target().name}.download`), { force: true }).catch(() => {});
	console.error(`relo: install failed\n${error.message}`);
	process.exitCode = 1;
}
