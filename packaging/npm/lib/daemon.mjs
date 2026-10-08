// Quiesces the daemon running from one home before its binary is replaced, and
// offers to start the new one: a graceful `daemon stop --force` first, then
// signals against this home's processes only, so a process that merely reused a
// port is never touched.
//
// The config scan reads key names instead of parsing TOML, so a key this
// scan cannot see keeps its default.
import { execFileSync, spawnSync } from 'node:child_process';
import { createInterface } from 'node:readline/promises';
import { createReadStream, createWriteStream, existsSync, readFileSync, rmSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

// reloHome is the state directory the daemon serves: $RELO_HOME, else ~/.relo.
export function reloHome(env = process.env) {
	return env.RELO_HOME?.trim() || join(homedir(), '.relo');
}

const DEFAULTS = { port: 10101, openai: 10201, anthropic: 10202, gemini: 10203 };

// servicePorts lists the ports a home opens: the management port plus one per
// client protocol. A protocol whose port is 0 is not opened.
export function servicePorts(configText = '') {
	const found = {};
	for (const line of configText.split('\n')) {
		const match = line.match(/^\s*(port|openai|anthropic|gemini)\s*=\s*(\d+)/);
		if (match) found[match[1]] ??= Number(match[2]);
	}
	return ['port', 'openai', 'anthropic', 'gemini']
		.map((key) => found[key] ?? DEFAULTS[key])
		.filter((port) => port !== 0);
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

// isReloImage reports whether an executable name is the relo binary, so a
// signal never reaches a process that merely reused the port.
export function isReloImage(comm) {
	const base = comm.trim().split(/[\\/]/).pop().toLowerCase();
	return base === 'relo' || base === 'relo.exe';
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
		if (!info || !isReloImage(info.comm)) owner = 0;
	}
	const candidates = new Set();
	if (owner) candidates.add(owner);
	for (const port of ports) {
		for (const pid of listeningPids(port)) candidates.add(pid);
	}
	const victims = [...candidates].filter((pid) => {
		const info = processInfo(pid);
		return info && isReloImage(info.comm) && servesHome({ pid, owner, args: info.args, home, defaultHome });
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
				`relo: install failed -- something else is listening on port ${port}\n` +
					'  free the port, or point relo at another one, and install again'
			);
		}
	}
	return { stopped };
}

// askStart prompts on a terminal and reports 'yes', 'no', or 'unattended'
// when no terminal can be asked on. The answer never comes from stdin: under
// a piped install stdin is not the user.
export async function askStart() {
	if (process.platform === 'win32') {
		if (!process.stdin.isTTY) return 'unattended';
		const terminal = createInterface({ input: process.stdin, output: process.stdout });
		try {
			const answer = await terminal.question('start relo now? [Y/n] ');
			return /^[nN]/.test(answer.trim()) ? 'no' : 'yes';
		} finally {
			terminal.close();
		}
	}
	let input;
	try {
		input = createReadStream('/dev/tty');
		await new Promise((resolve, reject) => {
			input.once('open', resolve);
			input.once('error', reject);
		});
	} catch {
		return 'unattended';
	}
	const terminal = createInterface({ input, output: createWriteStream('/dev/tty') });
	try {
		const answer = await terminal.question('start relo now? [Y/n] ');
		return /^[nN]/.test(answer.trim()) ? 'no' : 'yes';
	} catch {
		return 'unattended';
	} finally {
		terminal.close();
	}
}
