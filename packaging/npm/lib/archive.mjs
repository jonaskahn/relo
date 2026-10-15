// Reads the daemon binary out of a release tarball. Prereleases publish one
// versioned archive per platform instead of the raw binary the stable
// releases carry, so the installer falls back to this when the manifest
// lists no raw binary.
//
// Pure node:zlib plus a minimal tar walk, no dependency and no system tar,
// so the postinstall behaves the same on every host the package supports.
import { gunzipSync } from 'node:zlib';

function text(bytes) {
	return Buffer.from(bytes).toString('utf8').replace(/\0.*$/s, '').trim();
}

function entryName(header) {
	const name = text(header.subarray(0, 100));
	const prefix = text(header.subarray(345, 500));
	const joined = prefix ? `${prefix}/${name}` : name;
	return joined.replace(/^\.\//, '');
}

function entrySize(header) {
	const size = Number.parseInt(text(header.subarray(124, 136)), 8);
	if (!Number.isSafeInteger(size) || size < 0) {
		throw new Error('relo: the release archive has a corrupt entry size');
	}
	return size;
}

function safeName(name) {
	if (!name || name.startsWith('/') || name.startsWith('\\')) return false;
	return !name.split(/[\\/]/).includes('..');
}

// extractBinary returns the bytes of the single archive member whose file
// name is in names. Pax headers and directories are skipped; anything else
// that is not that one regular file fails the install instead of writing a
// wrong executable into the package.
export function extractBinary(tarball, { names }) {
	let tar;
	try {
		tar = gunzipSync(tarball);
	} catch {
		throw new Error('relo: the release archive is not valid gzip');
	}
	const found = [];
	let offset = 0;
	while (offset + 512 <= tar.length) {
		const header = tar.subarray(offset, offset + 512);
		if (header.every((byte) => byte === 0)) break;
		const name = entryName(header);
		const size = entrySize(header);
		const end = offset + 512 + size;
		if (end > tar.length) {
			throw new Error(`relo: the release archive truncates ${name || 'an entry'}`);
		}
		const type = String.fromCharCode(header[156]);
		if ((type === '0' || type === '\0') && names.includes(name.split('/').pop())) {
			if (!safeName(name)) {
				throw new Error(`relo: the release archive hides an unsafe path (${name})`);
			}
			found.push(Buffer.from(tar.subarray(offset + 512, end)));
		}
		offset += 512 + Math.ceil(size / 512) * 512;
	}
	if (found.length === 0) {
		throw new Error(`relo: the release archive holds no ${names.join(' or ')}`);
	}
	if (found.length > 1) {
		throw new Error('relo: the release archive holds more than one binary');
	}
	return found[0];
}
