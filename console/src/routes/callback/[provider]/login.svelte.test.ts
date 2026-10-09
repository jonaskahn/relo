import { describe, expect, it } from 'vitest';

import type { CallbackStatus } from '$lib/types';

import {
	expiredState,
	initialState,
	isFinal,
	pollCallback,
	stateFromStatus,
	readMs,
	statusPath,
	windMs,
	type CallbackState
} from './login.svelte';

/** Collects the states a poll reports and the waits it asks for, running each
 *  scheduled poll straight away so a test drives the sequence itself rather
 *  than waiting on a clock. The harness stops asking once its answers run out,
 *  which is what stops an unanswered poll from recursing forever. */
function harness(answers: (Partial<CallbackStatus> | null | Error)[]) {
	const seen: CallbackState[] = [];
	const waits: number[] = [];
	let index = 0;
	pollCallback({
		ticket: 'ticket-1',
		fetchStatus: () => {
			const answer = index < answers.length ? answers[index++] : (index++, null);
			if (answer === null) return Promise.resolve(null);
			if (answer instanceof Error) return Promise.reject(answer);
			return Promise.resolve({ provider: 'chatgpt', ...answer } as CallbackStatus);
		},
		onChange: (state) => seen.push(state),
		schedule: (run, ms) => {
			waits.push(ms);
			if (index < answers.length) run();
		}
	});
	// Every answer is read through a promise chain, so the sequence settles
	// once the microtask queue has drained.
	return { seen, waits, settled: () => new Promise((resolve) => setTimeout(resolve, 0)) };
}

describe('stateFromStatus', () => {
	it('reads a stored account as connected and names it', () => {
		expect(
			stateFromStatus({ provider: 'chatgpt', phase: 'connected', account: 'a@b.test' })
		).toEqual({ phase: 'connected', account: 'a@b.test', error: '' });
	});

	it('reads a connected login that named no account', () => {
		expect(stateFromStatus({ provider: 'chatgpt', phase: 'connected' }).account).toBe('');
	});

	it.each(['failed', 'denied', 'state', 'timeout'])('reads %s as a failure', (phase) => {
		expect(stateFromStatus({ provider: 'chatgpt', phase }).phase).toBe('failed');
	});

	it('carries the refusal the daemon named', () => {
		expect(stateFromStatus({ provider: 'chatgpt', phase: 'failed', error: 'denied' }).error).toBe(
			'denied'
		);
	});

	it('reads a failure that named no refusal as an empty one', () => {
		expect(stateFromStatus({ provider: 'chatgpt', phase: 'failed' }).error).toBe('');
	});
});

describe('isFinal', () => {
	it('settles on everything but a login still running', () => {
		expect(isFinal(initialState())).toBe(false);
		expect(isFinal(expiredState())).toBe(true);
		expect(isFinal({ phase: 'connected', account: 'a', error: '' })).toBe(true);
		expect(isFinal({ phase: 'failed', account: '', error: 'denied' })).toBe(true);
	});
});

describe('pollCallback', () => {
	it.each(['waiting', 'exchanging'])('keeps a login %s working', async (phase) => {
		const run = harness([{ phase }]);
		await run.settled();
		// A login on its way is not a refusal: it stays working and the poll
		// keeps asking.
		expect(run.seen).toEqual([initialState()]);
		expect(run.waits).toEqual([700]);
	});

	it('reports a connected login once and stops asking', async () => {
		const run = harness([
			{ phase: 'waiting' },
			{ phase: 'exchanging' },
			{ phase: 'connected', account: 'a@b.test' }
		]);
		await run.settled();
		expect(run.seen[run.seen.length - 1]).toEqual({
			phase: 'connected',
			account: 'a@b.test',
			error: ''
		});
		// One wait for each answer that did not settle the login, and none after
		// the one that did.
		expect(run.waits).toHaveLength(2);
	});

	it('reads a ticket the daemon no longer tracks as expired', async () => {
		const run = harness([null]);
		await run.settled();
		expect(run.seen).toEqual([expiredState()]);
		expect(run.waits).toEqual([]);
	});

	it('keeps waiting through a daemon that did not answer', async () => {
		const run = harness([new Error('connection refused')]);
		await run.settled();
		// An outage is not a login that failed: nothing is reported and the wait
		// continues, more slowly.
		expect(run.seen).toEqual([]);
		expect(run.waits).toEqual([1500]);
	});
});

describe('statusPath', () => {
	it('names the provider and the ticket beside the page that polls it', () => {
		expect(statusPath('chatgpt', 'ticket-1')).toBe('/callback/chatgpt/status?ticket=ticket-1');
	});

	it('escapes a provider and a ticket that carry address characters', () => {
		expect(statusPath('a/b', 'x y')).toBe('/callback/a%2Fb/status?ticket=x%20y');
	});
});

describe("the page's wait before it leaves", () => {
	it('reads the result, then dissolves, before the tab goes', () => {
		// There is no wait before the result: it is the only thing this tab has
		// left to say. What follows it is the one stretch in which the operator
		// can read it, and then the page comes apart. The three are asserted
		// together because together they are the whole of the page's life.
		expect(readMs + windMs).toBe(4200);
	});

	it('dissolves slowly enough that the edge reads as pixels rather than a wipe', () => {
		// Over the 400ms every other moment is capped at
		// (docs/design/specs/foundations/motion.md). Below roughly this, the
		// quantised rows have too few frames to be seen.
		expect(windMs).toBeGreaterThan(600);
	});

	it('leaves the operator long enough to read what happened', () => {
		// A tab that closes itself has no time to explain, so this stretch is the
		// only one in which the result can be taken in.
		expect(readMs).toBeGreaterThanOrEqual(1000);
	});
});
