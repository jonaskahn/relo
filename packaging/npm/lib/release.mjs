// Where the daemon binaries live and how they are named. Kept out of
// install.mjs so the rules can be tested without running a download.

// REPO is the GitHub repository the release job uploads to.
export const REPO = 'https://github.com/jonaskahn/relo';

// version normalizes a release tag, accepting both `v0.1.0` and `0.1.0`.
export function version(tag) {
	return tag?.trim().replace(/^v/, '') || null;
}

// tagFromArgs reads the release to install from the command line. The
// installer takes no other argument; an unknown one is ignored so a bare
// postinstall keeps installing the latest release.
export function tagFromArgs(argv = []) {
	for (let i = 0; i < argv.length; i += 1) {
		const arg = argv[i];
		if (arg.startsWith('--relo-version=')) return version(arg.slice('--relo-version='.length));
		if (arg === '--relo-version' && i + 1 < argv.length) return version(argv[i + 1]);
	}
	return null;
}

// assets resolves the download directory for one release. Without a tag it
// returns the latest-release directory, which is how a release is always
// found when no pin is given.
export function assets(tag = null) {
	const pinned = version(tag);
	return {
		base: pinned ? `${REPO}/releases/download/v${pinned}` : `${REPO}/releases/latest/download`,
		version: pinned
	};
}

// checksums lists every asset in a release with its sha256, so one fetch
// covers the single binary this host needs.
export function checksumsUrl(tag = null) {
	return `${assets(tag).base}/checksums.txt`;
}

// url is the asset one host installs.
export function url(name, tag = null) {
	return `${assets(tag).base}/${name}`;
}
