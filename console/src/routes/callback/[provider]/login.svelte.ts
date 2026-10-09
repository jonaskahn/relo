// The login a callback page watches: the state it renders, and the poll that
// moves it. The page renders whatever `state` says, so every transition the
// operator sees is a change of this object rather than a step of the page's own.
import type { CallbackStatus } from '$lib/types';

/** Where a login can be. A final state is one the poll stops on. */
export type CallbackPhase = 'working' | 'connected' | 'failed' | 'expired';

export interface CallbackState {
	readonly phase: CallbackPhase;
	readonly account: string;
	/** The refusal category, when one is what stopped the login. */
	readonly error: string;
}

/** The page waits 700ms between polls and gives the daemon longer after a
 *  failure of its own, which is what the page it replaced did. */
export const pollIntervalMs = 700;
export const retryIntervalMs = 1500;

/** How long the panel sits still once it has arrived, in milliseconds. This is
 *  the one stretch in which the operator can read which account was stored, so
 *  it is short enough not to drag and long enough to be read at a glance. */
export const readMs = 1200;

/** How long the page takes to come apart, in milliseconds. The close waits for
 *  exactly this long.
 *
 *  Deliberately over the 400ms every other moment in the console is capped at
 *  (docs/design/specs/foundations/motion.md): an edge quantised into visible
 *  rows needs enough frames for a cell to flicker through, and below ~600ms it
 *  reads as a wipe rather than as the page coming apart. It runs once, on the
 *  way out, and it is the longest moment on the page. */
export const windMs = 3000;

/** The state a login starts in, before the first poll answers. */
export function initialState(): CallbackState {
	return { phase: 'working', account: '', error: '' };
}

/** A login the daemon no longer tracks. The page cannot tell an expiry from a
 *  login that never started, and shows the operator the same way. */
export function expiredState(): CallbackState {
	return { phase: 'expired', account: '', error: '' };
}

/** The phases a login passes through on its way to a stored account, and the
 *  ones that end it. `waiting` is a browser that has not come back yet;
 *  `exchanging` is an authorization that arrived and is being traded for a
 *  token. Neither is a refusal. */
const workingPhases = new Set(['waiting', 'exchanging', 'working']);

/** Reads one poll answer as the state to render. A login still on its way
 *  stays working, one that stored an account is connected and names it, and
 *  anything else is a failure carrying the refusal the daemon named, if it
 *  named one. */
export function stateFromStatus(status: CallbackStatus): CallbackState {
	if (workingPhases.has(status.phase)) return initialState();
	if (status.phase === 'connected') {
		return { phase: 'connected', account: status.account ?? '', error: '' };
	}
	return { phase: 'failed', account: '', error: status.error ?? '' };
}

/** Whether a state ends the wait, so the page stops asking. */
export function isFinal(state: CallbackState): boolean {
	return state.phase !== 'working';
}

/** One poll's answer: the status, or nothing when the daemon no longer tracks
 *  the login. The status endpoint answers a forgotten ticket with a 404, which
 *  is an expiry and not an outage. */
export type PollAnswer = CallbackStatus | null;

export interface PollOptions {
	/** The ticket the daemon issued this login under. */
	ticket: string;
	/** Answers one poll. Rejects only when the daemon could not be reached. */
	fetchStatus: (ticket: string) => Promise<PollAnswer>;
	/** Receives every state change, including the first answer. */
	onChange: (state: CallbackState) => void;
	/** Schedules the next poll, so a test drives the clock itself. */
	schedule: (run: () => void, ms: number) => void;
}

/** Polls one login until it reaches a final state. A daemon that did not answer
 *  is not a login that failed: the wait continues, more slowly, until the login
 *  settles or the daemon goes quiet. */
export function pollCallback(options: PollOptions): void {
	const { ticket, fetchStatus, onChange, schedule } = options;
	const tick = (): void => {
		void fetchStatus(ticket)
			.then((answer) => {
				const state = answer === null ? expiredState() : stateFromStatus(answer);
				onChange(state);
				if (isFinal(state)) return;
				schedule(tick, pollIntervalMs);
			})
			.catch(() => schedule(tick, retryIntervalMs));
	};
	tick();
}

/** The path the poll reads. It sits outside the management API prefix, beside
 *  the page that polls it, and answers without a session. */
export function statusPath(provider: string, ticket: string): string {
	return `/callback/${encodeURIComponent(provider)}/status?ticket=${encodeURIComponent(ticket)}`;
}
