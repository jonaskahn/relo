// Quiesces the daemon running from one home before its binary is replaced, and
// offers to start the new one: a graceful `daemon stop --force` first, then
// signals against this home's processes only, so a process that merely reused a
// port is never touched.
//
// The config scan reads key names instead of parsing TOML, so a key this
// scan cannot see keeps its default.
import { execFileSync, spawnSync } from 'node:child_process';
import { createInterface } from 'node:readline/promises';
import { closeSync, existsSync, openSync, readFileSync, rmSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';
import { ReadStream } from 'node:tty';

// reloHome is the state directory the daemon serves: $RELO_HOME, else ~/.relo.
export function reloHome(env = process.env) {
	return env.RELO_HOME?.trim() || join(homedir(), '.relo');
}

const DEFAULTS = { port: 10101 };

// servicePorts lists the port a home opens: the single listener's port. A
// zero port asks for an ephemeral one, which no scan can name.
export function servicePorts(configText = '') {
	let found;
	for (const line of configText.split('\n')) {
		const match = line.match(/^\s*port\s*=\s*(\d+)/);
		if (match) found ??= Number(match[1]);
	}
	const port = found ?? DEFAULTS.port;
	return port === 0 ? [] : [port];
}

// runtimePid reads the pid a daemon published, or null when no usable file
// is there.
export function runtimePid(text) {
	const match = text.match(/"pid"\s*:\s*(\d+)/);
	return match ? Number(match[1]) : null;
}

// parseLsof reads `lsof -nP -iTCP:<port> -sTCP:LISTEN -t` output: one pid
// per line.
export function parseLsof(text) {
	return text
		.split('\n')
		.map((line) => line.trim())
		.filter((line) => /^\d+$/.test(line))
		.map(Number);
}

// parseSs reads `ss -ltnp` output, collecting every pid= on a line that names
// the port.
export function parseSs(text, port) {
	const names = new RegExp(`:${port}(?!\\d)`);
	const pids = [];
	for (const line of text.split('\n')) {
		if (!names.test(line)) continue;
		for (const match of line.matchAll(/pid=(\d+)/g)) pids.push(Number(match[1]));
	}
	return pids;
}

// parseNetstat reads `netstat -ano -p tcp` output (windows), pairing a
// LISTENING local port with its owning pid.
export function parseNetstat(text, port) {
	const pids = [];
	for (const line of text.split('\n')) {
		const match = line.match(/^\s*TCP\s+\S+:(\d+)\s+\S+\s+LISTENING\s+(\d+)/i);
		if (match && Number(match[1]) === port) pids.push(Number(match[2]));
	}
	return pids;
}

// isReloImage reports whether an executable name is a relo binary, so a
// signal never reaches a process that merely reused the port. The installers
// name the daemon `relo`, but the release assets and this package keep the
// platform in the name (`relo-darwin-arm64`, `relo-darwin-arm64-0.1.1`), so
// both spellings count.
export function isReloImage(comm) {
	const base = comm.trim().split(/[\\/]/).pop().toLowerCase();
	if (base === 'relo' || base === 'relo.exe') return true;
	return /^relo-(?:darwin|linux|windows)-(?:amd64|arm64)(?:-[0-9][0-9a-z.+-]*)?(?:\.exe)?$/.test(base);
}

// isReloProcess reports whether a process is a relo binary. Linux truncates
// the command name at fifteen characters, which can cut `relo-linux-amd64`
// short, so the executable in the command line settles it when the name does
// not.
export function isReloProcess({ comm = '', args = '' } = {}) {
	if (isReloImage(comm)) return true;
	const executable = args.trim().split(/\s+/)[0] ?? '';
	return executable !== '' && isReloImage(executable);
}

// servesHome reports whether a relo process belongs to this home. The pid
// the home published always does; otherwise the command line must name
// --home. A relo started without --home serves the default home.
export function servesHome({ pid, owner, args, home, defaultHome }) {
	if (owner && pid === owner) return true;
	if (args.includes(`--home ${home}`) || args.includes(`--home=${home}`)) return true;
	if (!/--home(?:=|\s)/.test(args)) return home === defaultHome;
	return false;
}

// listeningPids lists the processes holding a port. An unavailable or empty
// lookup reads as no listeners, so a missing lsof never fails the install.
function listeningPids(port) {
	try {
		if (process.platform === 'win32') {
			const out = execFileSync('netstat', ['-ano', '-p', 'tcp'], {
				encoding: 'utf8',
				stdio: ['ignore', 'pipe', 'ignore']
			});
			return parseNetstat(out, port);
		}
		if (process.platform === 'darwin') {
			const out = execFileSync('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-t'], {
				encoding: 'utf8',
				stdio: ['ignore', 'pipe', 'ignore']
			});
			return parseLsof(out);
		}
		try {
			const out = execFileSync('lsof', ['-nP', `-iTCP:${port}`, '-sTCP:LISTEN', '-t'], {
				encoding: 'utf8',
				stdio: ['ignore', 'pipe', 'ignore']
			});
			return parseLsof(out);
		} catch {
			const out = execFileSync('ss', ['-ltnp'], {
				encoding: 'utf8',
				stdio: ['ignore', 'pipe', 'ignore']
			});
			return parseSs(out, port);
		}
	} catch {
		return [];
	}
}

// processInfo describes a pid, or null when it is already gone. The command
// line comes from powershell on windows, which can fail; an empty line then
// falls back to the owner and default-home rules in servesHome.
function processInfo(pid) {
	try {
		if (process.platform === 'win32') {
			const listing = execFileSync('tasklist', ['/FI', `PID eq ${pid}`, '/FO', 'CSV', '/NH'], {
				encoding: 'utf8',
				stdio: ['ignore', 'pipe', 'ignore']
			}).trim();
			const comm = listing.match(/^"([^"]+)"/)?.[1] ?? '';
			let args = '';
			try {
				args = execFileSync(
					'powershell',
					['-NoProfile', '-Command', `(Get-CimInstance Win32_Process -Filter "ProcessId = ${pid}").CommandLine`],
					{ encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] }
				).trim();
			} catch {}
			return { comm, args };
		}
		const comm = execFileSync('ps', ['-p', String(pid), '-o', 'comm='], {
			encoding: 'utf8',
			stdio: ['ignore', 'pipe', 'ignore']
		}).trim();
		const args = execFileSync('ps', ['-p', String(pid), '-o', 'args='], {
			encoding: 'utf8',
			stdio: ['ignore', 'pipe', 'ignore']
		}).trim();
		return { comm, args };
	} catch {
		return null;
	}
}

function alive(pid) {
	try {
		process.kill(pid, 0);
		return true;
	} catch {
		return false;
	}
}

function signal(pid, name) {
	try {
		process.kill(pid, name);
	} catch {}
}

// sleep blocks without a dependency, so the poll below stays synchronous.
function sleep(ms) {
	Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);
}

// quiesce leaves no Relo running from one home before its binary is replaced:
// a graceful `daemon stop --force` through the already-installed binary, then
// signals against this home's processes only, then a check that the configured
// ports are free. Reports whether a daemon was there at all.
//
// A port another program holds fails the install. The daemon that would have
// served it is gone either way, so replacing its binary now would leave a new
// daemon that cannot bind -- and an automatic `npm install` must never kill a
// process that is not Relo.
export function quiesce(home, { binary } = {}) {
	const configPath = join(home, 'config.toml');
	const ports = servicePorts(existsSync(configPath) ? readFileSync(configPath, 'utf8') : '');
	const runtimePath = join(home, 'runtime.json');
	const defaultHome = join(homedir(), '.relo');
	let owner = 0;
	if (existsSync(runtimePath)) {
		try {
			owner = runtimePid(readFileSync(runtimePath, 'utf8')) ?? 0;
		} catch {
			owner = 0;
		}
		const info = owner ? processInfo(owner) : null;
		if (!info || !isReloProcess(info)) owner = 0;
	}
	const candidates = new Set();
	if (owner) candidates.add(owner);
	for (const port of ports) {
		for (const pid of listeningPids(port)) candidates.add(pid);
	}
	const victims = [...candidates].filter((pid) => {
		const info = processInfo(pid);
		return info && isReloProcess(info) && servesHome({ pid, owner, args: info.args, home, defaultHome });
	});
	const stopped = victims.length > 0;

	if (binary && existsSync(binary)) {
		// --force frees the ports too; a build that predates it rejects the
		// flag, and the signals below cover that build.
		spawnSync(binary, ['--home', home, 'daemon', 'stop', '--force'], { stdio: 'ignore' });
	}

	for (const pid of victims.filter(alive)) signal(pid, 'SIGTERM');
	// A daemon gets ten seconds to close its listeners before the kill it
	// cannot catch. On windows both signals terminate.
	const deadline = Date.now() + 10_000;
	let live = victims.filter(alive);
	while (live.length > 0 && Date.now() < deadline) {
		sleep(1000);
		live = live.filter(alive);
	}
	for (const pid of live) signal(pid, 'SIGKILL');
	rmSync(runtimePath, { force: true });

	for (const port of ports) {
		const holders = listeningPids(port);
		if (holders.length > 0) {
			throw new Error(
				`something else is listening on port ${port}\n` +
					'  free the port, or point relo at another one, and install again'
			);
		}
	}
	return { stopped };
}

// move steps a menu selection like a game menu: it wraps at both ends, so
// left from the first option lands on the last.
export function move(index, count, delta) {
	if (count <= 0) return 0;
	return (index + delta + count) % count;
}

// decodeKeys reads the keys a terminal chunk carries: 'left', 'right',
// 'enter', or 'interrupt'. pending holds an escape sequence the previous
// chunk ended in the middle of; the returned pending is carried into the
// next call.
//
// Arrows arrive in either form a terminal sends them, CSI (`ESC [ D`) and
// application-cursor SS3 (`ESC O D`). A modified arrow -- parameters before
// the final byte -- and every other sequence is consumed and ignored.
export function decodeKeys(pending = '', chunk = '') {
	const text = pending + chunk;
	const keys = [];
	let at = 0;
	while (at < text.length) {
		if (text[at] === '\x1b') {
			const end = escapeEnd(text, at);
			if (end === 0) return { keys, pending: text.slice(at) };
			// A lone Esc, or the Esc of an Alt chord, leaves nothing to act
			// on. Dropping only the Esc lets the next byte read as its own
			// key, so an Esc press never swallows a following Enter.
			if (end === at + 1) {
				at += 1;
				continue;
			}
			const sequence = text.slice(at, end);
			if (sequence === '\x1b[C' || sequence === '\x1bOC') keys.push('right');
			else if (sequence === '\x1b[D' || sequence === '\x1bOD') keys.push('left');
			at = end;
			continue;
		}
		const key = text[at];
		if (key === '\r' || key === '\n') keys.push('enter');
		else if (key === '\x03') keys.push('interrupt');
		at += 1;
	}
	return { keys, pending: '' };
}

// escapeEnd finds the byte after the escape sequence starting at `at`. Zero
// means the text ended before the sequence did, so the rest is a pending
// prefix for the next chunk.
function escapeEnd(text, at) {
	const next = text[at + 1];
	if (next === undefined) return 0;
	if (next === '[') {
		for (let cursor = at + 2; cursor < text.length; cursor += 1) {
			const code = text.charCodeAt(cursor);
			if (code >= 0x40 && code <= 0x7e) return cursor + 1;
			// A control byte cannot sit inside a CSI: drop the Esc so the
			// control byte still reads as itself.
			if (code < 0x20) return at + 1;
		}
		return 0;
	}
	if (next === 'O') {
		const third = text[at + 2];
		if (third === undefined) return 0;
		return third.charCodeAt(0) < 0x20 ? at + 1 : at + 3;
	}
	// A bare Esc, or the Esc of an Alt chord.
	if (next === '\x1b' || next.charCodeAt(0) < 0x20) return at + 1;
	return at + 2;
}

// terminalInput opens the terminal for raw keys. POSIX reads the controlling
// tty, not stdin: under a piped install stdin is not the user. Windows has no
// /dev/tty, so a console stdin is the only source there.
function terminalInput() {
	if (process.platform === 'win32') {
		if (!process.stdin?.isTTY) return null;
		// close() unrefs stdin so the installer can exit; a second question
		// needs it flowing again or its read never settles.
		process.stdin.ref();
		process.stdin.resume();
		return {
			input: process.stdin,
			close() {
				// stdin stays open for the daemon spawn that follows.
				process.stdin.pause();
				process.stdin.unref();
			}
		};
	}
	let fd;
	try {
		fd = openSync('/dev/tty', 'r+');
	} catch {
		return null;
	}
	let input;
	try {
		input = new ReadStream(fd);
	} catch {
		// The stream owns the fd once it is built, but not when it is not.
		closeSync(fd);
		return null;
	}
	return {
		input,
		close() {
			input.destroy();
		}
	};
}

// select shows labels as a left/right menu and resolves the chosen index, -1
// when the user interrupts, or null when there is no terminal to ask on.
// onDraw hears every move, so the caller can repaint its own screen.
export async function select(labels, { onDraw } = {}) {
	const source = terminalInput();
	if (!source) return null;
	const { input } = source;
	let at = 0;
	let pending = '';
	let settled = false;
	let resolve;
	const settle = (value) => {
		if (settled) return;
		settled = true;
		resolve(value);
	};
	const onStop = () => settle(null);
	const onData = (chunk) => {
		if (settled) return;
		const decoded = decodeKeys(pending, chunk.toString('utf8'));
		pending = decoded.pending;
		for (const key of decoded.keys) {
			if (key === 'left' || key === 'right') {
				at = move(at, labels.length, key === 'right' ? 1 : -1);
				onDraw?.(at);
			} else if (key === 'enter') {
				settle(at);
				return;
			} else if (key === 'interrupt') {
				settle(-1);
				return;
			}
		}
	};
	try {
		return await new Promise((done) => {
			resolve = done;
			// The error listener goes on before raw mode: a failed setRawMode
			// emits rather than throws, and an unhandled emit would crash.
			input.once('error', onStop);
			input.once('end', onStop);
			input.once('close', onStop);
			try {
				input.setRawMode(true);
			} catch {
				settle(null);
				return;
			}
			if (settled) return;
			onDraw?.(at);
			input.on('data', onData);
		});
	} finally {
		// Restore cooked mode before the error listener is removed, so a
		// failed restore cannot surface as an unhandled error.
		try {
			input.setRawMode(false);
		} catch {
			// The terminal is gone; there is nothing left to restore.
		}
		input.removeListener('data', onData);
		input.removeListener('error', onStop);
		input.removeListener('end', onStop);
		input.removeListener('close', onStop);
		source.close();
	}
}

// askStart asks the start question on a terminal and reports 'yes', 'no', or
// 'unattended' when no terminal can be asked on. The answer never comes from
// stdin: under a piped install stdin is not the user. onQuestion and onAnswer
// let the caller draw the question inside its own screen; onQuestion hears
// every move of the left/right menu, and the first label is Yes.
//
// A raw key stream is used rather than readline: a readline stream leaves tty
// handles open, and those keep the installer's process alive after the work
// is done, so npm waits on a postinstall that has already finished. select()
// tears its stream down instead. Ctrl-C means "not now", never a failed
// install: the binary is already in place.
export async function askStart({ labels = ['YES', 'NO'], onQuestion, onAnswer } = {}) {
	if (process.platform === 'win32' && !onQuestion) {
		if (!process.stdin.isTTY) return 'unattended';
		const terminal = createInterface({ input: process.stdin, output: process.stdout });
		try {
			const answer = await terminal.question('START RELO NOW? [Y/n] ');
			return /^[nN]/.test(answer.trim()) ? 'no' : 'yes';
		} finally {
			terminal.close();
		}
	}
	const at = await select(labels, { onDraw: onQuestion });
	if (at === null) return 'unattended';
	if (at < 0) return 'no';
	onAnswer?.(labels[at]);
	return at === 0 ? 'yes' : 'no';
}
