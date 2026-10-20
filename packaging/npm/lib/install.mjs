// Puts the binary for this host inside the installed package. The launcher
// in bin/ execs it, so the daemon and its tray run as their own process and
// do not need node at run time. The install refuses a machine where another
// Relo already owns the relo command, whether an installer or an older
// package put it there: two installs would fight over the name, the home
// directory, and the ports. A relo running from this home is quiesced before
// its binary is replaced -- its daemon is stopped and the ports it served
// are freed -- and the install asks whether to start the new one.
//
// The binary always comes from a GitHub release: the one --relo-version names,
// the one this package was published as when that is a prerelease, or the
// latest release. Rerun this file by hand with the flag to install a specific
// release.
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { mkdir, rename, rm, writeFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { expect, parse } from './checksums.mjs';
import { askStart, quiesce, reloHome } from './daemon.mjs';
import { findConflictingRelo, findReloApp, findSetupInstall } from './conflict.mjs';
import { fetchBytes } from './download.mjs';
import {
	bar,
	bytes,
	choices,
	chosen,
	clear,
	closeScreen,
	done,
	duration,
	failed,
	interactive,
	note,
	openScreen,
	progress,
	separator,
	step,
	warn
} from './progress.mjs';
import { target } from './platform.mjs';
import { assets, checksumsUrl, prereleaseTag, selectBundle, tagFromArgs, url } from './release.mjs';
import { extractBinary } from './archive.mjs';

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const binaryDir = resolve(packageRoot, 'bin');
const home = reloHome();
const publishedVersion = JSON.parse(readFileSync(resolve(packageRoot, 'package.json'), 'utf8')).version;

// writeAtomic lands the binary in one rename, so an interrupted install leaves
// no truncated executable behind for the launcher to find.
async function writeAtomic(destination, body) {
	const staging = `${destination}.download`;
	await writeFile(staging, body, { mode: 0o755 });
	await rename(staging, destination);
}

// LOGO is the installer's block-letter mark. Six rows of shadow type give
// the title card its arcade weight.
const LOGO = [
	'██████╗ ███████╗██╗      ██████╗',
	'██╔══██╗██╔════╝██║     ██╔═══██╗',
	'██████╔╝█████╗  ██║     ██║   ██║',
	'██╔══██╗██╔══╝  ██║     ██║   ██║',
	'██║  ██║███████╗███████╗╚██████╔╝',
	'╚═╝  ╚═╝╚══════╝╚══════╝ ╚═════╝'
];

// label pads a step name to one column, so the values line up under it.
const label = (name) => name.padEnd(8);

// downloadStats renders the running download: the shimmering bar, the
// percentage, the bytes, the rate, and the time left.
function downloadStats(received, total, elapsed, tick) {
	const seconds = Math.max(elapsed, 1) / 1000;
	const rate = received / seconds;
	const parts = [];
	if (total > 0) parts.push(`${Math.floor((received / total) * 100)}%`);
	parts.push(total > 0 ? `${bytes(received)}/${bytes(total)}` : bytes(received));
	// A rate needs a moment to mean anything: the first chunk lands before
	// any time has passed, which reads as an absurd speed and a wrong ETA.
	if (elapsed >= 500) {
		parts.push(`${bytes(rate)}/s`);
		if (total > received && rate > 0) {
			parts.push(`ETA ${Math.max(1, Math.ceil((total - received) / rate))}s`);
		}
	}
	const barText = total > 0 ? `[${bar(received / total, { width: 12, color: true, frame: tick })}]  ` : '';
	return `${barText}${parts.join('  ')}`;
}

async function fromRelease(tag, asset, startedAt) {
	step(label('MANIFEST'));
	const checksums = await fetchBytes(checksumsUrl(tag), 'checksums.txt', { onRetry: warn });
	done(`${label('MANIFEST')}  ${bytes(checksums.length)}`);

	// One manifest lists every asset in the release, so a single fetch
	// verifies the binary before anything executable is written. A release
	// carrying the raw binary uses it; one carrying only the versioned
	// tarballs falls back to this host's archive, and the binary inside is
	// what the launcher beside it already expects.
	const manifest = parse(checksums);
	const pick = selectBundle(manifest, asset);
	step(label('DOWNLOAD'));
	const downloadStart = Date.now();
	let latest = { received: 0, total: 0 };
	const downloaded = await fetchBytes(url(pick.name, tag), pick.name, {
		onProgress: (received, total) => {
			latest = { received, total };
			progress(
				(tick) => downloadStats(latest.received, latest.total, Date.now() - downloadStart, tick),
				{ force: total > 0 && received === total }
			);
		},
		onRetry: warn
	});
	done(`${label('DOWNLOAD')}  ${bytes(downloaded.length)} in ${duration(Date.now() - downloadStart)}`);

	expect(manifest, pick.name, downloaded);
	done(`${label('CHECKSUM')}  sha256 verified`);

	const inner = asset.os === 'windows' ? 'relo.exe' : 'relo';
	let body = downloaded;
	if (pick.kind !== 'raw') {
		step(label('EXTRACT'));
		body = extractBinary(downloaded, { names: [inner] });
		done(`${label('EXTRACT')}  ${inner}`);
	}

	const destination = resolve(binaryDir, asset.name);
	// Quiesced first: on windows the rename below fails over a running exe, and
	// the new daemon cannot bind a port the old one still serves.
	step(label('DAEMON'));
	const { stopped } = quiesce(home, { binary: existsSync(destination) ? destination : undefined });
	done(`${label('DAEMON')}  ${stopped ? 'stopped the running daemon' : 'not running'}`);

	step(label('INSTALL'));
	await mkdir(binaryDir, { recursive: true });
	await writeAtomic(destination, body);
	done(`${label('INSTALL')}  ${bytes(body.length)} · ${asset.os}-${asset.arch} · ${duration(Date.now() - startedAt)}`);
	return destination;
}

try {
	const startedAt = Date.now();
	const tag = tagFromArgs(process.argv.slice(2)) ?? prereleaseTag(publishedVersion);
	const asset = target();
	const { version } = assets(tag);
	await openScreen(LOGO, 'I N S T A L L E R', [
		version ? `v${version}` : 'latest release',
		`${asset.os} · ${asset.arch}`.toUpperCase()
	]);
	step(label('CONFLICT'));
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
	// Relo.app installs no command on PATH, so the search above never sees
	// it; Relo-Setup without its PATH entry is invisible the same way.
	const app = findReloApp();
	if (app) {
		throw new Error(
			`Relo.app is already installed at ${app}\n` +
				'  remove that install first to install this package, or keep using that install instead'
		);
	}
	if (findSetupInstall()) {
		throw new Error(
			'Relo is already installed by Relo-Setup.\n' +
				'  remove that install first to install this package, or keep using that install instead'
		);
	}
	done(`${label('CONFLICT')}  no other relo install`);

	const destination = await fromRelease(tag, asset, startedAt);
	separator();
	const question = 'START RELO NOW?';
	const labels = ['YES', 'NO'];
	const answer = interactive()
		? await askStart({
				labels,
				onQuestion: (at) => choices(question, labels, at),
				onAnswer: (value) => chosen(question, value)
			})
		: await askStart();
	if (answer === 'yes') {
		closeScreen();
		// The daemon's error goes into the result block rather than through
		// the screen: a start refused before it spawned anything names the
		// cause on stderr, and that is the whole answer.
		const started = spawnSync(destination, ['daemon', 'start'], {
			stdio: ['inherit', 'inherit', 'pipe'],
			encoding: 'utf8'
		});
		if (started.status !== 0) {
			const detail = (started.stderr ?? '').trim() || started.error?.message || '';
			failed(
				'DAEMON DID NOT START',
				detail
					? `${detail}\n  start it later: relo daemon start`
					: 'start it later: relo daemon start'
			);
		}
	} else {
		note('start it later: relo daemon start');
		closeScreen();
	}
} catch (error) {
	// Remove only the staging file this install wrote. Removing the directory
	// would take bin/relo.mjs with it, leaving a package whose `relo` command
	// no longer exists.
	await rm(resolve(binaryDir, `${target().name}.download`), { force: true }).catch(() => {});
	clear();
	closeScreen();
	failed('INSTALL FAILED', error.message);
	process.exitCode = 1;
}
