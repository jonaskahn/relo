/** Process log view logic: the logs page sections, the request and daemon log queries, and the
 *  merge of a live tail into the lines already shown. */
export type LogSection = 'agent' | 'daemon' | 'startup';

/** One parsed line of the daemon's own log. */
export interface DaemonLine {
	offset: number;
	timestamp_ms: number;
	level: string;
	message: string;
	detail?: string;
}

/** One boot transcript the startup section lists. */
export interface StartupEntry {
	name: string;
	started_at: string;
	outcome: string;
	error?: string;
	current: boolean;
	size: number;
}

/** The severity floor the daemon log is read at. */
export type DaemonLevel = 'all' | 'warning' | 'error';

/** Reports whether a level a select handed back is one the log is read at. */
export function isDaemonLevel(value: string): value is DaemonLevel {
	return value === 'all' || value === 'warning' || value === 'error';
}

/** Reads the section the address names. Anything else is the request log the page showed before
 *  sections existed. */
export function parseLogSection(value: string | null): LogSection {
	if (value === 'daemon' || value === 'startup') return value;
	return 'agent';
}

/** The five ways the request log narrows. The page holds one so its filter toggle can count what
 *  is active while the section's fields edit it. */
export interface RequestFilters {
	provider: string;
	status: 'any' | 'ok' | 'error';
	// origin separates the traffic the console started from the traffic a
	// client sent through the data plane.
	origin: 'any' | 'internal' | 'external';
	// key names an access key and client names the coding client a key was
	// issued for, which is what tells an operator which agent made a request.
	key: string;
	client: string;
}

/** Reports whether a choice a select handed back is one the log can filter on. */
export function isRequestStatus(value: string): value is RequestFilters['status'] {
	return value === 'any' || value === 'ok' || value === 'error';
}

/** Reports whether a choice a select handed back is one the log can separate traffic by. */
export function isRequestOrigin(value: string): value is RequestFilters['origin'] {
	return value === 'any' || value === 'internal' || value === 'external';
}

/** Counts the filters that narrow the log, which is what a collapsed filter panel reports on its
 *  toggle. */
export function countRequestFilters(filters: RequestFilters): number {
	return [
		filters.provider,
		filters.status === 'any' ? '' : filters.status,
		filters.origin === 'any' ? '' : filters.origin,
		filters.client,
		filters.key
	].filter((value) => value !== '').length;
}

/** Encodes one page of the request log. An empty cursor reads from the newest row; a cursor
 *  pages forward from the last one shown. */
export function buildRequestQuery(filters: RequestFilters, cursor = ''): string {
	const params = new URLSearchParams();
	params.set('limit', '50');
	if (cursor) params.set('cursor', cursor);
	if (filters.provider) params.set('provider', filters.provider);
	// OK and Error name whole classes, so a 429 is an error and a 204 is
	// an answer rather than two codes nothing happens to match.
	if (filters.status === 'ok') params.set('status', '2xx');
	if (filters.status === 'error') params.set('status', '4xx,5xx');
	if (filters.origin !== 'any') params.set('origin', filters.origin);
	if (filters.key) params.set('client', filters.key);
	if (filters.client) params.set('client_app', filters.client);
	return params.toString();
}

/** One daemon log read. */
export interface DaemonQuery {
	limit: number;
	before?: string;
	after?: string;
	level: DaemonLevel;
	hideRequests: boolean;
}

/** Encodes one daemon log read. The live tail passes the end cursor back as after; paging older
 *  lines passes the oldest offset as before. */
export function buildDaemonQuery(query: DaemonQuery): string {
	const params = new URLSearchParams();
	params.set('limit', String(query.limit));
	if (query.before) params.set('before', query.before);
	if (query.after) params.set('after', query.after);
	if (query.level !== 'all') params.set('level', query.level);
	if (query.hideRequests) params.set('hide_requests', '1');
	return params.toString();
}

/** Folds a live tail into the lines shown, newest first.
 *  Offsets only grow, so a line seen before keeps its place and a line seen twice shows once.
 *  The cap bounds a tab left live for hours. */
export function mergeDaemonLines(
	current: readonly DaemonLine[],
	incoming: readonly DaemonLine[],
	limit = 1000
): DaemonLine[] {
	const seen = new Map<number, DaemonLine>();
	for (const line of current) seen.set(line.offset, line);
	for (const line of incoming) seen.set(line.offset, line);
	return [...seen.values()].sort((a, b) => b.offset - a.offset).slice(0, limit);
}

/** Maps a log severity onto the status pill tones, so a line never rests on color: info is a
 *  quiet dot, warning and error carry words. */
export function daemonLevelTone(level: string): 'muted' | 'warn' | 'fail' {
	switch (level.toLowerCase()) {
		case 'warning':
		case 'warn':
			return 'warn';
		case 'error':
			return 'fail';
		default:
			return 'muted';
	}
}

/** Maps a boot outcome onto the status pill tones: a start that reached serving is settled, a
 *  failed one carries an icon and a word. */
export function startupOutcomeTone(outcome: string): 'pass' | 'fail' {
	return outcome === 'failed' ? 'fail' : 'pass';
}
