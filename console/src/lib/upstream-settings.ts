// Upstream settings describe how long the relay waits for provider bytes,
// how long it waits between retries, and how long a refused account or route
// member stays out of rotation. The daemon stores them globally in the
// startup file, and a connection may override the first two.

/** The call waits an operator may pick, in seconds: the longest a provider call may stay silent
 *  between bytes. The shipped default is 300. */
export const CALL_WAIT_PRESETS = [180, 300, 600, 900, 1800];

/** The shipped call wait in seconds, used when neither the connection nor the daemon stores one. */
export const DEFAULT_CALL_WAIT = 300;

/** The shipped wait before each retry, as low–high second ranges in retry order. */
export const DEFAULT_RETRY_BACKOFF: [number, number][] = [
	[1, 3],
	[3, 5],
	[5, 10]
];

/** The longest wait one retry may take, so a request is never parked for hours before its next
 *  attempt. */
export const RETRY_WINDOW_CEILING = 600;

/** The escalating failover waits an operator may pick, in seconds: the chosen wait is the first
 *  step, and each further failure of the same account or route member moves to the next, longer
 *  one. The first step may be zero: no wait at all, so the first refusal is immediately reusable.
 *  The shipped default is zero. */
export const FAILOVER_COOLDOWN_PRESETS = [0, 180, 300, 900, 1800, 3600, 18000];

/** The shipped failover wait in seconds, used when the daemon stores none: zero, the first step
 *  without a wait, so only a repeated failure starts escalating. */
export const DEFAULT_FAILOVER_COOLDOWN = 0;

/** Renders a failover wait the way the chips read it: whole hours as 1h, whole minutes as 15m,
 *  everything else in seconds. */
export function failoverCooldownLabel(seconds: number): string {
	if (seconds >= 3600 && seconds % 3600 === 0) return `${seconds / 3600}h`;
	return callWaitLabel(seconds);
}

/** Renders a call wait the way the chips read it: whole minutes as 1m, everything else in
 *  seconds. */
export function callWaitLabel(seconds: number): string {
	if (seconds >= 60 && seconds % 60 === 0) return `${seconds / 60}m`;
	return `${seconds}s`;
}

/** Renders three windows as "1–3s, 3–5s, 5–10s". */
export function retryBackoffLabel(windows: [number, number][]): string {
	return windows.map(([low, high]) => `${low}–${high}s`).join(', ');
}

/** Reports windows the daemon accepts: exactly three ranges with 0 <= low < high <= 600. */
export function retryBackoffValid(windows: [number, number][]): boolean {
	if (windows.length !== 3) return false;
	return windows.every(
		([low, high]) =>
			Number.isInteger(low) &&
			Number.isInteger(high) &&
			low >= 0 &&
			high > low &&
			high <= RETRY_WINDOW_CEILING
	);
}

/** Reads whatever the daemon answered into three usable windows, falling back to the shipped
 *  defaults when the value is missing or outside what the relay can draw from. */
export function normalizeRetryBackoff(value: unknown): [number, number][] {
	const fallback = (): [number, number][] => cloneWindows(DEFAULT_RETRY_BACKOFF);
	if (!Array.isArray(value) || value.length !== 3) return fallback();
	const windows: [number, number][] = [];
	for (const raw of value) {
		if (!Array.isArray(raw) || raw.length !== 2) return fallback();
		const window: [number, number] = [Number(raw[0]), Number(raw[1])];
		if (
			!Number.isInteger(window[0]) ||
			!Number.isInteger(window[1]) ||
			window[0] < 0 ||
			window[1] <= window[0] ||
			window[1] > RETRY_WINDOW_CEILING
		) {
			return fallback();
		}
		windows.push(window);
	}
	return windows;
}

/** Copies windows, so a draft never mutates the stored settings. */
export function cloneWindows(windows: [number, number][]): [number, number][] {
	return windows.map(([low, high]) => [low, high]);
}
