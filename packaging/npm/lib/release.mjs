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

// pinnedTag pins an install to the version this package was published as. npm
// does not forward the install spec -- `@v0.1.8`, `@beta`, `@latest` -- to
// lifecycle scripts, so the installed package version is the only pin the
// postinstall can see. The publish job checks every declaration against the
// release tag first, so a published version always has a matching v<version>
// release to download: following the package keeps the binary and the shim on
// one version instead of installing the latest binary under an older package.
export function pinnedTag(published) {
	return version(published);
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

// selectBundle names the asset one host downloads from a release manifest:
// the raw binary when the release carries it, else this host's versioned
// tarball. It throws naming both when neither is listed, so a failed
// install says what is missing instead of what a 404 means.
export function selectBundle(entries, { os, arch, name }) {
	if (entries.has(name)) return { kind: 'raw', name };
	const bundle = tarballFor(entries, { os, arch });
	if (bundle) return { kind: 'tarball', name: bundle };
	throw new Error(
		`no ${name} or versioned tarball in the release. ` +
			'No release may be published yet; rerun the installer with --relo-version <tag> for a published tag, or without it for the latest release.'
	);
}

// tarballFor finds the versioned archive a release publishes for one host
// inside its own checksums manifest. Releases ship one tarball per
// platform, and the version is read out of the manifest entry itself, so
// this resolves both a pinned tag and the unpinned latest release without
// a second fetch.
export function tarballFor(entries, { os, arch }) {
	const match = [...entries.keys()].find((name) => {
		const parts = new RegExp(`^relo-(.+)-${os}-${arch}\\.tar\\.gz$`).exec(name);
		return parts && version(parts[1]);
	});
	return match ?? null;
}
