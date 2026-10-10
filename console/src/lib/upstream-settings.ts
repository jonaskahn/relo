// Upstream settings describe how long the relay waits for provider bytes, how
// many times it retries one account before moving on, and how long a refused
// connection or route member stays out of rotation. The daemon stores them
// globally in the startup file, and a connection may override the first two.

/** The call waits an operator may pick, in seconds: the longest a provider call may stay silent
 *  between bytes. The shipped default is 300. */
export const CALL_WAIT_PRESETS = [180, 300, 600, 900, 1800];

/** The shipped call wait in seconds, used when neither the connection nor the daemon stores one. */
export const DEFAULT_CALL_WAIT = 300;

/** The retry counts an operator may pick: how many times one account is retried before the
 *  request moves to the next account. The shipped default is 3. */
export const RETRY_ATTEMPT_PRESETS = [1, 3, 5];

/** The shipped retry count, used when neither the connection nor the daemon stores one. */
export const DEFAULT_RETRY_ATTEMPTS = 3;

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

/** Renders a retry count the way the chips read it: "3×". */
export function retryAttemptsLabel(attempts: number): string {
	return `${attempts}\u00d7`;
}
