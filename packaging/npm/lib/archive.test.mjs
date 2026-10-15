import assert from 'node:assert/strict';
import { test } from 'node:test';
import { gzipSync } from 'node:zlib';

import { extractBinary } from './archive.mjs';

// entry builds one 512-byte tar header plus its data blocks. Only the
// fields the reader looks at are set; the checksum is left zeroed.
function entry(name, body = '', type = '0') {
	const header = Buffer.alloc(512);
	header.write(name, 0, 100, 'utf8');
	header.write(Number(body.length).toString(8).padStart(11, '0'), 124, 11, 'utf8');
	header[156] = type.charCodeAt(0);
	const data = Buffer.from(body);
	const padding = Buffer.alloc((512 - (data.length % 512)) % 512);
	return Buffer.concat([header, data, padding]);
}

function archive(...entries) {
	return gzipSync(Buffer.concat([...entries, Buffer.alloc(1024)]));
}

test('a tarball with the single binary returns its bytes', () => {
	const body = extractBinary(archive(entry('relo', 'binary-bytes')), { names: ['relo'] });
	assert.equal(body.toString(), 'binary-bytes');
});

test('a ./ prefixed name and surrounding pax headers are skipped to the binary', () => {
	const body = extractBinary(
		archive(
			entry('pax-header', 'comment=atime', 'x'),
			entry('icons', '', '5'),
			entry('./relo', 'binary-bytes')
		),
		{ names: ['relo'] }
	);
	assert.equal(body.toString(), 'binary-bytes');
});

test('a windows tarball matches relo.exe', () => {
	const body = extractBinary(archive(entry('relo.exe', 'MZ')), { names: ['relo.exe'] });
	assert.equal(body.toString(), 'MZ');
});

test('no matching member fails the install', () => {
	assert.throws(
		() => extractBinary(archive(entry('relo', 'x')), { names: ['relo.exe'] }),
		/relo: the release archive holds no relo\.exe/
	);
});

test('two matching binaries fail instead of picking one', () => {
	assert.throws(
		() => extractBinary(archive(entry('a/relo', 'one'), entry('b/relo', 'two')), { names: ['relo'] }),
		/more than one binary/
	);
});

test('an unsafe path fails even when its base name matches', () => {
	assert.throws(
		() => extractBinary(archive(entry('../relo', 'evil')), { names: ['relo'] }),
		/unsafe path/
	);
});

test('a corrupt entry size fails instead of reading wild offsets', () => {
	const header = entry('relo', 'x');
	header.write('not-octal!!', 124, 11, 'utf8');
	assert.throws(
		() => extractBinary(gzipSync(Buffer.concat([header, Buffer.alloc(1024)])), { names: ['relo'] }),
		/corrupt entry size/
	);
});

test('a member cut off mid-archive reports the truncation', () => {
	const full = entry('relo', 'x'.repeat(1000));
	const cut = gzipSync(full.subarray(0, 512 + 100));
	assert.throws(() => extractBinary(cut, { names: ['relo'] }), /truncates relo/);
});

test('non-gzip input fails with guidance', () => {
	assert.throws(() => extractBinary(Buffer.from('not an archive'), { names: ['relo'] }), /not valid gzip/);
});
