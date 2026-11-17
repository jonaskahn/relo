/** Proxies: parsing, de-duplication, redaction, and health labels for the pool editor. */

export type ProxyScheme = 'http' | 'https' | 'socks5';

export interface ParsedProxy {
	scheme: ProxyScheme;
	host: string;
	port: number;
	username: string;
	password: string;
	label: string;
}

export type ProxyLineStatus = 'ready' | 'duplicate' | 'invalid';

export interface ProxyLine {
	line: number;
	raw: string;
	status: ProxyLineStatus;
	reason: string;
	proxy: ParsedProxy | null;
}

const schemes = new Set(['http', 'https', 'socks5', 'socks5h']);

/** Parse one pasted line into the six fields the pool stores. */
export function parseProxyLine(raw: string): ParsedProxy | null {
	const trimmed = raw.trim();
	if (trimmed === '') return null;
	let candidate = trimmed;
	if (!candidate.includes('://')) {
		if (candidate.includes('@')) {
			candidate = 'http://' + candidate;
		} else {
			const parts = candidate.split(':');
			if (parts.length === 2) {
				candidate = 'http://' + candidate;
			} else if (parts.length === 4) {
				candidate = `http://${parts[2]}:${parts[3]}@${parts[0]}:${parts[1]}`;
			} else {
				return null;
			}
		}
	}
	let parsed: URL;
	try {
		parsed = new URL(candidate);
	} catch {
		return null;
	}
	const scheme = parsed.protocol.slice(0, -1).toLowerCase();
	if (!schemes.has(scheme)) return null;
	const port = Number(parsed.port);
	if (!Number.isInteger(port) || port < 1 || port > 65535 || parsed.hostname === '') return null;
	const username = decodeURIComponent(parsed.username);
	const password = decodeURIComponent(parsed.password);
	return {
		scheme: scheme === 'socks5h' ? 'socks5' : (scheme as ProxyScheme),
		host: parsed.hostname,
		port,
		username,
		password,
		label: `${parsed.hostname}:${port}`
	};
}

/** Parse a paste block, ignoring comments and blanks, and report duplicates. */
export function parseProxyPaste(text: string, existing: string[] = []): ProxyLine[] {
	const seen = new Set(existing.map((value) => value.toLowerCase()));
	const lines: ProxyLine[] = [];
	text.split('\n').forEach((raw, index) => {
		const trimmed = raw.trim();
		if (trimmed === '' || trimmed.startsWith('#')) return;
		const proxy = parseProxyLine(trimmed);
		if (proxy === null) {
			lines.push({
				line: index + 1,
				raw: trimmed,
				status: 'invalid',
				reason: 'invalid',
				proxy: null
			});
			return;
		}
		const key = `${proxy.scheme}://${proxy.username}@${proxy.host}:${proxy.port}`.toLowerCase();
		if (seen.has(key)) {
			lines.push({
				line: index + 1,
				raw: trimmed,
				status: 'duplicate',
				reason: 'duplicate',
				proxy
			});
			return;
		}
		seen.add(key);
		lines.push({ line: index + 1, raw: trimmed, status: 'ready', reason: '', proxy });
	});
	return lines;
}

/** The address the row shows, with any password replaced by dots. */
export function redactProxy(
	proxy: Pick<ParsedProxy, 'host' | 'port' | 'username'> & { scheme: string; hasPassword?: boolean }
): string {
	const address = `${proxy.scheme}://${proxy.host}:${proxy.port}`;
	if (proxy.username === '' && !proxy.hasPassword) return address;
	return `${proxy.scheme}://${proxy.username}:••••@${proxy.host}:${proxy.port}`;
}

/** The label a home state shows for one pool entry. */
export function proxyStateLabel(entry: { state: string; cooling_until?: string }): string {
	switch (entry.state) {
		case 'ready':
			return 'ready';
		case 'cooling':
			return entry.cooling_until ? 'cooling_until:' + entry.cooling_until : 'cooling';
		case 'unreachable':
			return 'unreachable';
		default:
			return 'unverified';
	}
}
