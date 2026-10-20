// Writes the install's progress where the user can watch it, styled as a
// Game Boy screen: the DMG palette, a title screen that reveals itself, and
// a menu of steps inside the same frame.
//
// npm runs a postinstall in the background by default, so everything the
// installer prints on stdout is captured and shown only when the script
// fails. The user's terminal is still there, so the progress goes straight
// to /dev/tty and stays visible while the daemon binary downloads. Without a
// terminal to write to -- windows, or a piped install -- the same lines go
// to the console, where npm shows them under --foreground-scripts.
import { openSync, writeSync } from 'node:fs';

// The four shades of a Game Boy screen.
const DARK = [15, 56, 15];
const MID = [48, 98, 48];
const LIGHT = [139, 172, 15];
const BRIGHT = [155, 188, 15];

// The screen is one rectangle: the card, the steps, and the closing border
// all share this width, so every row ends on the same column.
const INNER = 76;

let handle;
let tried = false;
let active = null;
let frame = 0;
let ticker = null;
let renderer = null;
let framed = false;
let lastAt = 0;
let lastText = null;

// terminal is the file descriptor of the user's terminal, or null when there
// is none to write to.
function terminal() {
	if (tried) return handle;
	tried = true;
	if (process.platform === 'win32') return null;
	try {
		handle = openSync('/dev/tty', 'a');
	} catch {
		// No controlling terminal: the console is the only place left.
		handle = null;
	}
	return handle;
}

// emit writes to the terminal and reports whether it landed there.
function emit(text) {
	const fd = terminal();
	if (fd === null) return false;
	try {
		writeSync(fd, text);
		return true;
	} catch {
		// The terminal went away mid-install; the console still works.
		return false;
	}
}

// paint writes a line that will be rewritten in place. Windows has no
// /dev/tty here, so its console gets the line instead when it is a terminal.
function paint(text) {
	if (emit(text)) return;
	if (process.stdout?.isTTY) process.stdout.write(text);
}

// colorEnabled reports whether the output can carry color. NO_COLOR and a
// dumb terminal turn it off; FORCE_COLOR turns it on.
function colorEnabled() {
	if (process.env.NO_COLOR || process.env.FORCE_COLOR === '0') return false;
	if (process.env.FORCE_COLOR) return true;
	if (process.env.TERM === 'dumb') return false;
	return terminal() !== null || Boolean(process.stdout?.isTTY);
}

function truecolor() {
	return /^(truecolor|24bit)$/i.test(process.env.COLORTERM ?? '');
}

// shade paints text in one of the screen's greens. A terminal that speaks
// 24-bit color gets the exact DMG shade; anything else gets the nearest
// basic green, and no color at all stays plain.
function shade(color, text) {
	if (!colorEnabled()) return text;
	if (truecolor()) {
		return `\x1b[38;2;${color[0]};${color[1]};${color[2]}m${text}\x1b[0m`;
	}
	const code = color === BRIGHT ? '92' : color === LIGHT ? '32' : '2';
	return `\x1b[${code}m${text}\x1b[0m`;
}

const bright = (text) => shade(BRIGHT, text);
const light = (text) => shade(LIGHT, text);
const mid = (text) => shade(MID, text);

// width counts the columns a painted line occupies, ignoring its escapes.
function width(text) {
	return text.replace(/\x1b\[[0-9;]*m/g, '').length;
}

// row wraps a line in the screen's walls and pads it to the full width, so
// every line of the screen is a rectangle of the same size.
function row(text) {
	if (!framed) return text;
	const pad = Math.max(0, INNER - width(text) - 2);
	return `${bright('│')} ${text}${' '.repeat(pad)} ${bright('│')}`;
}

// stopActivity ends the redraw timer, so a finished (or failed) download
// never leaves one behind.
function stopActivity() {
	if (ticker !== null) {
		clearInterval(ticker);
		ticker = null;
	}
	renderer = null;
}

// clear wipes the line the progress bar drew on, so the next write -- a
// step, or an error -- starts on a clean line.
export function clear() {
	stopActivity();
	paint('\r\x1b[2K');
}

// rawLine clears the running line and prints one line exactly as given.
function rawLine(text) {
	clear();
	active = null;
	lastText = null;
	if (emit(`${text}\n`)) return;
	console.log(text);
}

// line clears the running line and prints one line, framed when the screen
// is open.
function line(text) {
	rawLine(row(text));
}

// pause breathes between the lines of the screen, so it draws itself the way
// a game's title screen comes up. The console gets no delay.
function pause(ms = 28) {
	return terminal() === null ? Promise.resolve() : new Promise((done) => setTimeout(done, ms));
}

// openScreen draws the top of the screen: the logo centered, the label, the
// release details, and the rule the menu sits under. closeScreen draws the
// bottom when the install is over.
export async function openScreen(logo, label, details) {
	const logoWidth = Math.max(...logo.map((text) => text.length));
	const center = (text, paint) => {
		const pad = Math.floor((INNER - text.length) / 2);
		return paint(' '.repeat(pad) + text + ' '.repeat(INNER - text.length - pad));
	};
	const blank = () => `${bright('│')}${' '.repeat(INNER)}${bright('│')}`;
	const card = (text, paint) => `${bright('│')}${center(text, paint)}${bright('│')}`;

	framed = true;
	rawLine(bright(`┌${'─'.repeat(INNER)}┐`));
	await pause();
	rawLine(blank());
	await pause();
	for (const text of logo) {
		rawLine(card(text.padEnd(logoWidth), bright));
		await pause();
	}
	rawLine(blank());
	await pause();
	rawLine(card(label, bright));
	await pause();
	rawLine(blank());
	await pause();
	for (const text of details) {
		rawLine(card(text, light));
		await pause();
	}
	rawLine(blank());
	await pause();
	rawLine(bright(`├${'─'.repeat(INNER)}┤`));
}

// separator draws the rule between two blocks of the screen.
export function separator() {
	if (!framed) return;
	rawLine(bright(`├${'─'.repeat(INNER)}┤`));
}

// closeScreen draws the bottom border. It is safe to call when no screen is
// open, so the error path can close whatever was drawn.
export function closeScreen() {
	if (!framed) return;
	framed = false;
	rawLine(bright(`└${'─'.repeat(INNER)}┘`));
}

// interactive reports whether there is a screen to draw and keys to read.
export function interactive() {
	return terminal() !== null || Boolean(process.stdin?.isTTY && process.stdout?.isTTY);
}

// choices draws a question and its option buttons on the running line. The
// cursor stays there, so the next key redraws the same row.
export function choices(question, labels, at) {
	active = null;
	lastText = null;
	clear();
	paint(row(`${light(question)}  ${buttons(labels, at)}`));
}

// chosen replaces the question row with the option that was picked.
export function chosen(question, value) {
	line(`${light('▶')} ${question} ${value}`);
}

// buttons renders the options as one row of keycaps. Every form of a label is
// the same width, so moving the selection never shifts the row.
export function buttons(labels, at) {
	return labels
		.map((label, index) => (index === at ? highlight(`▶ ${label} ◀`) : mid(`[ ${label} ]`)))
		.join('  ');
}

// highlight paints text as the chosen menu item: bright background, dark text.
// A terminal without 24-bit color falls back to inverse video.
function highlight(text) {
	if (!colorEnabled()) return text;
	if (truecolor()) {
		return `\x1b[48;2;${BRIGHT[0]};${BRIGHT[1]};${BRIGHT[2]}m\x1b[38;2;${DARK[0]};${DARK[1]};${DARK[2]}m${text}\x1b[0m`;
	}
	return `\x1b[7m${text}\x1b[0m`;
}

// selected paints the running step the way a Game Boy highlights the chosen
// menu item: bright background, dark text. The cursor blinks and the dots
// cycle so the chip reads as busy.
function selected(label, tick = 0) {
	const dots = '.'.repeat(tick % 4).padEnd(3);
	const cursor = tick % 4 < 2 ? '▶' : '▷';
	return highlight(` ${`${cursor} ${label}${dots}`} `);
}

// draw paints one running line, throttled so a fast download neither floods
// the terminal with frames nor skips a percentage.
function draw(text, force) {
	const now = Date.now();
	const onTerminal = terminal() !== null;
	if (!force) {
		if (text === lastText) return;
		if (onTerminal ? now - lastAt < 50 : now - lastAt < 1000) return;
	}
	lastText = text;
	lastAt = now;
	const rendered = row(`${selected(active ?? '', frame)}  ${text}`);
	if (onTerminal) {
		emit(`\r\x1b[2K${rendered}`);
		return;
	}
	console.log(rendered);
}

// step starts a step and leaves the cursor on its line. On the console the
// running line is printed by progress() instead, so a step that never
// reports progress leaves only its finished line behind.
export function step(text) {
	active = text;
	frame = 0;
	clear();
	lastText = null;
	emit(row(selected(text, 0)));
}

// done ends the running step.
export function done(text) {
	line(`${light('✓')} ${text}`);
}

// warn prints a step that did not go to plan but does not stop the install.
export function warn(text) {
	line(`${bright('!')} ${text}`);
}

// note prints something the installer did not do, such as a command to run
// later.
export function note(text) {
	line(`${mid('·')} ${mid(text)}`);
}

// failed prints a failed result as the fourth block, below the closed screen.
// It goes to stderr, where npm shows it even when a postinstall's stdout was
// captured.
export function failed(title, message) {
	console.error(errorBox(title, message));
}

// errorBox renders a failure result: a red-walled block the same width as the
// screen, the headline, and the message wrapped to fit. Exported so its shape
// can be tested without a terminal.
export function errorBox(title, message) {
	const wall = red('│');
	const rows = [red(`┌${'─'.repeat(INNER)}┐`)];
	const add = (text, paint) => {
		const pad = Math.max(0, INNER - width(text) - 2);
		rows.push(`${wall} ${paint(text)}${' '.repeat(pad)} ${wall}`);
	};
	add(`✗ ${title}`, redBright);
	add('', red);
	for (const line of wrapText(message, INNER - 2)) add(line, red);
	rows.push(red(`└${'─'.repeat(INNER)}┘`));
	return rows.join('\n');
}

// errorColor follows the same switches as the screen but looks at stderr,
// the stream the failure block is printed on, so a captured log stays clean
// when stderr is not a terminal.
function errorColor() {
	if (process.env.NO_COLOR || process.env.FORCE_COLOR === '0') return false;
	if (process.env.FORCE_COLOR) return true;
	if (process.env.TERM === 'dumb') return false;
	return Boolean(process.stderr?.isTTY);
}

const red = (text) => (errorColor() ? `\x1b[31m${text}\x1b[0m` : text);
const redBright = (text) => (errorColor() ? `\x1b[1;31m${text}\x1b[0m` : text);

// wrapText breaks a message into lines no wider than width, keeping the
// two-space indent a hint line starts with. A word longer than a line -- a
// path in a daemon error, say -- is split so the box walls stay aligned.
function wrapText(text, width) {
	const lines = [];
	for (const paragraph of text.split('\n')) {
		const indent = paragraph.match(/^\s*/)[0];
		const room = Math.max(1, width - indent.length);
		const words = paragraph.trim().split(/\s+/).filter(Boolean);
		if (words.length === 0) {
			lines.push('');
			continue;
		}
		let line = '';
		for (const word of words) {
			let rest = word;
			while (rest.length > room) {
				if (line !== '') {
					lines.push(indent + line);
					line = '';
				}
				lines.push(indent + rest.slice(0, room));
				rest = rest.slice(room);
			}
			if (rest === '') continue;
			if (line !== '' && line.length + 1 + rest.length > room) {
				lines.push(indent + line);
				line = rest;
			} else line = line === '' ? rest : `${line} ${rest}`;
		}
		if (line !== '') lines.push(indent + line);
	}
	return lines;
}

// progress starts a running line. render is called with a frame number and
// returns the text, so the line can keep moving -- the bar's shade, the
// blinking cursor -- even while no bytes arrive. A terminal redraws it at
// most twenty times a second; the console prints it once a second instead,
// since there the output is a log rather than a redrawn line. force draws
// the frame that ends a download.
export function progress(render, { force = false } = {}) {
	renderer = render;
	draw(render(frame), force);
	if (ticker !== null || terminal() === null) return;
	// Unreferenced: an animation must never be the reason an install hangs.
	ticker = setInterval(() => {
		if (renderer === null) return;
		frame += 1;
		draw(renderer(frame), true);
	}, 90);
	ticker.unref();
}

// bar renders a fraction of a whole as a fixed width block bar. Every filled
// cell is the same full block, so the bar stays a clean rectangle; a frame
// only moves the shade across those blocks, which keeps it moving even on a
// stalled connection. color dims the track.
export function bar(fraction, { width: cells = 24, color = false, frame: tick } = {}) {
	const filled = Math.max(0, Math.min(cells, Math.round(fraction * cells)));
	const track = '░'.repeat(cells - filled);
	if (!color || !colorEnabled()) return `${'█'.repeat(filled)}${track}`;
	let paint = '';
	for (let index = 0; index < filled; index += 1) {
		paint += tick !== undefined && (index + tick) % 6 > 3 ? light('█') : bright('█');
	}
	return `${paint}${mid(track)}`;
}

// bytes renders a byte count the way a person reads it.
export function bytes(count) {
	if (count < 1024) return `${count} B`;
	if (count < 1024 * 1024) return `${(count / 1024).toFixed(1)} KB`;
	return `${(count / (1024 * 1024)).toFixed(1)} MB`;
}

// duration renders an elapsed time the way a person reads it.
export function duration(ms) {
	if (ms < 1000) return `${Math.round(ms)}ms`;
	const seconds = ms / 1000;
	if (seconds < 60) return `${seconds.toFixed(1)}s`;
	return `${Math.floor(seconds / 60)}m ${Math.round(seconds % 60)}s`;
}
