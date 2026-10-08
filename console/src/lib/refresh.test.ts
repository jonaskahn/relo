import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { coalesceReload } from './refresh';

// The scheduler must be judged against real timing, so the tests drive the
// clock themselves and hold a reload open to watch the overlap rules.
beforeEach(() => {
	vi.useFakeTimers();
});

afterEach(() => {
	vi.useRealTimers();
});

function deferred() {
	let resolve: () => void = () => undefined;
	const promise = new Promise<void>((done) => {
		resolve = done;
	});
	return { promise, resolve };
}

describe('coalesceReload', () => {
	it('runs one reload for a burst of announcements', async () => {
		const reload = vi.fn();
		const scheduler = coalesceReload(reload, 300);

		scheduler.schedule();
		scheduler.schedule();
		scheduler.schedule();
		expect(reload).not.toHaveBeenCalled();

		await vi.advanceTimersByTimeAsync(300);
		expect(reload).toHaveBeenCalledTimes(1);
	});

	it('runs again for an announcement after the burst', async () => {
		const reload = vi.fn();
		const scheduler = coalesceReload(reload, 300);

		scheduler.schedule();
		await vi.advanceTimersByTimeAsync(300);
		scheduler.schedule();
		await vi.advanceTimersByTimeAsync(300);

		expect(reload).toHaveBeenCalledTimes(2);
	});

	it('queues one trailing reload while a run is in flight', async () => {
		const gate = deferred();
		const reload = vi.fn(async () => {
			await gate.promise;
		});
		const scheduler = coalesceReload(reload, 300);

		scheduler.schedule();
		await vi.advanceTimersByTimeAsync(300);
		scheduler.schedule();
		scheduler.schedule();
		gate.resolve();
		await vi.runAllTimersAsync();

		// Two announcements during one run collapse into a single trailing run.
		expect(reload).toHaveBeenCalledTimes(2);
	});

	it('drops the queued reload when the caller cancels', async () => {
		const reload = vi.fn();
		const scheduler = coalesceReload(reload, 300);

		scheduler.schedule();
		scheduler.cancel();
		await vi.advanceTimersByTimeAsync(300);

		expect(reload).not.toHaveBeenCalled();
	});

	it('drops the trailing run when the caller cancels mid-flight', async () => {
		const gate = deferred();
		const reload = vi.fn(async () => {
			await gate.promise;
		});
		const scheduler = coalesceReload(reload, 300);

		scheduler.schedule();
		await vi.advanceTimersByTimeAsync(300);
		scheduler.schedule();
		scheduler.cancel();
		gate.resolve();
		await vi.runAllTimersAsync();

		expect(reload).toHaveBeenCalledTimes(1);
	});
});
