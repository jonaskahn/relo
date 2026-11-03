import { afterEach, describe, expect, it, vi } from 'vitest';

import type * as Sse from './sse';

// The console reads the daemon's live state over one event stream. These
// tests give every simulated tab its own module instance, document, and
// navigator, while the fake lock manager and broadcast bus are shared the way
// the browser shares them between real tabs.

class FakeEventSource {
	static opened: FakeEventSource[] = [];

	url: string;
	closed = false;
	listeners = new Map<string, Array<(event: Event) => void>>();

	constructor(url: string) {
		this.url = url;
		FakeEventSource.opened.push(this);
	}

	addEventListener(name: string, handler: (event: Event) => void) {
		this.listeners.set(name, [...(this.listeners.get(name) ?? []), handler]);
	}

	close() {
		this.closed = true;
	}

	// emit delivers one frame the way the daemon's writer would.
	emit(name: string, data: string) {
		for (const handler of this.listeners.get(name) ?? []) {
			handler(new MessageEvent('message', { data }));
		}
	}
}

class FakeBroadcastChannel {
	static channels: FakeBroadcastChannel[] = [];

	name: string;
	onmessage: ((event: MessageEvent<unknown>) => void) | null = null;
	closed = false;

	constructor(name: string) {
		this.name = name;
		FakeBroadcastChannel.channels.push(this);
	}

	postMessage(data: unknown) {
		for (const peer of FakeBroadcastChannel.channels) {
			if (peer === this || peer.name !== this.name || peer.closed) continue;
			queueMicrotask(() => peer.onmessage?.(new MessageEvent('message', { data })));
		}
	}

	close() {
		this.closed = true;
	}
}

type LockWaiter = {
	callback: () => Promise<void>;
	resolve: () => void;
	reject: (reason: unknown) => void;
	settled: boolean;
};

class FakeLockManager {
	private holder: LockWaiter | null = null;
	private queue: LockWaiter[] = [];

	request(
		_name: string,
		options: { signal?: AbortSignal },
		callback: () => Promise<void>
	): Promise<void> {
		return new Promise((resolve, reject) => {
			const waiter: LockWaiter = { callback, resolve, reject, settled: false };
			const abort = () => {
				if (waiter.settled) return;
				waiter.settled = true;
				const waiting = this.queue.indexOf(waiter);
				if (waiting >= 0) this.queue.splice(waiting, 1);
				reject(new DOMException('aborted', 'AbortError'));
			};
			if (options.signal?.aborted) {
				abort();
				return;
			}
			options.signal?.addEventListener('abort', abort, { once: true });
			if (this.holder) this.queue.push(waiter);
			else void this.grant(waiter);
		});
	}

	private async grant(waiter: LockWaiter) {
		this.holder = waiter;
		try {
			await waiter.callback();
		} finally {
			if (this.holder === waiter) this.holder = null;
			waiter.settled = true;
			waiter.resolve();
			const next = this.queue.shift();
			if (next) void this.grant(next);
		}
	}
}

class FakeDocument {
	visibilityState: DocumentVisibilityState;
	private handlers: Array<() => void> = [];

	constructor(visibilityState: DocumentVisibilityState = 'visible') {
		this.visibilityState = visibilityState;
	}

	addEventListener(name: string, handler: () => void) {
		if (name === 'visibilitychange') this.handlers.push(handler);
	}

	become(visibilityState: DocumentVisibilityState) {
		this.visibilityState = visibilityState;
		for (const handler of this.handlers) handler();
	}
}

type Tab = {
	module: typeof Sse;
	document: FakeDocument;
};

const closers: Array<() => void> = [];

// Every test detaches what it subscribed, so the shared stream of one test
// never answers the next.
function track(close: () => void) {
	closers.push(close);
	return close;
}

function flush() {
	return new Promise((resolve) => setTimeout(resolve, 0));
}

async function openTab(
	options: {
		locks?: FakeLockManager;
		visibility?: DocumentVisibilityState;
		crossTab?: boolean;
	} = {}
): Promise<Tab> {
	const { locks = new FakeLockManager(), visibility = 'visible', crossTab = true } = options;
	const page = new FakeDocument(visibility);
	vi.stubGlobal('document', page);
	vi.stubGlobal('navigator', crossTab ? { locks } : {});
	vi.stubGlobal('BroadcastChannel', crossTab ? FakeBroadcastChannel : undefined);
	vi.stubGlobal('EventSource', FakeEventSource);
	vi.resetModules();
	const module = await import('./sse');
	return { module, document: page };
}

afterEach(() => {
	for (const close of closers.splice(0)) close();
	FakeEventSource.opened = [];
	FakeBroadcastChannel.channels = [];
	vi.unstubAllGlobals();
});

describe('one stream across tabs', () => {
	it('opens exactly one stream for several tabs listening at once', async () => {
		const locks = new FakeLockManager();
		const first = await openTab({ locks });
		const second = await openTab({ locks });
		const third = await openTab({ locks });

		track(first.module.subscribe('logs', vi.fn()));
		track(second.module.subscribe('quota', vi.fn()));
		track(third.module.subscribe('status', vi.fn()));
		await flush();

		expect(FakeEventSource.opened).toHaveLength(1);
		expect(FakeEventSource.opened[0].url).toBe('/api/v1/events/logs,quota,status');
	});

	it('hands the leader frames to listeners in the other tabs', async () => {
		const locks = new FakeLockManager();
		const leader = await openTab({ locks });
		const follower = await openTab({ locks });
		const leaderListener = vi.fn();
		const followerListener = vi.fn();

		track(leader.module.subscribe('logs', leaderListener));
		track(follower.module.subscribe('logs', followerListener));
		await flush();
		FakeEventSource.opened[0].emit('logs', JSON.stringify({ line: 'from the daemon' }));
		await flush();

		expect(leaderListener).toHaveBeenCalledExactlyOnceWith({ line: 'from the daemon' });
		expect(followerListener).toHaveBeenCalledExactlyOnceWith({ line: 'from the daemon' });
	});

	it('skips a frame it cannot read and leaves the stream open', async () => {
		const tab = await openTab();
		const onEvent = vi.fn();

		track(tab.module.subscribe('logs', onEvent));
		await flush();
		FakeEventSource.opened[0].emit('logs', 'not json at all');
		expect(onEvent).not.toHaveBeenCalled();

		FakeEventSource.opened[0].emit('logs', JSON.stringify({ line: 'after' }));
		expect(onEvent).toHaveBeenCalledExactlyOnceWith({ line: 'after' });
		expect(FakeEventSource.opened[0].closed).toBe(false);
	});

	it('ignores a frame sent on another channel', async () => {
		const tab = await openTab();
		const onEvent = vi.fn();

		track(tab.module.subscribe('logs', onEvent));
		await flush();
		FakeEventSource.opened[0].emit('quota', JSON.stringify({ other: true }));

		expect(onEvent).not.toHaveBeenCalled();
	});

	it('moves the stream to another tab when the leader closes it', async () => {
		const locks = new FakeLockManager();
		const leader = await openTab({ locks });
		const follower = await openTab({ locks });

		const closeLeader = track(leader.module.subscribe('logs', vi.fn()));
		track(follower.module.subscribe('logs', vi.fn()));
		await flush();
		expect(FakeEventSource.opened).toHaveLength(1);

		closeLeader();
		await flush();

		expect(FakeEventSource.opened).toHaveLength(2);
		expect(FakeEventSource.opened[0].closed).toBe(true);
		expect(FakeEventSource.opened[1].closed).toBe(false);
	});

	it('closes the stream with the last listener and reopens for a later one', async () => {
		const tab = await openTab();
		const first = track(tab.module.subscribe('logs', vi.fn()));
		const second = track(tab.module.subscribe('status', vi.fn()));
		await flush();

		first();
		expect(FakeEventSource.opened[0].closed).toBe(false);
		second();
		expect(FakeEventSource.opened[0].closed).toBe(true);

		track(tab.module.subscribe('quota', vi.fn()));
		await flush();
		expect(FakeEventSource.opened).toHaveLength(2);
		expect(FakeEventSource.opened[1].closed).toBe(false);
	});
});

describe('a hidden tab', () => {
	it('opens no stream until the tab becomes visible', async () => {
		const tab = await openTab({ visibility: 'hidden' });

		track(tab.module.subscribe('logs', vi.fn()));
		await flush();
		expect(FakeEventSource.opened).toHaveLength(0);

		tab.document.become('visible');
		await flush();
		expect(FakeEventSource.opened).toHaveLength(1);
	});

	it('closes its stream and reopens it when the tab becomes visible', async () => {
		const tab = await openTab();
		const onEvent = vi.fn();

		track(tab.module.subscribe('logs', onEvent));
		await flush();
		expect(FakeEventSource.opened).toHaveLength(1);

		tab.document.become('hidden');
		expect(FakeEventSource.opened[0].closed).toBe(true);

		tab.document.become('visible');
		await flush();

		expect(FakeEventSource.opened).toHaveLength(2);
		expect(FakeEventSource.opened[1].closed).toBe(false);
		expect(onEvent).toHaveBeenCalledExactlyOnceWith(undefined);
	});

	it('leaves the stream to another visible tab while it is hidden', async () => {
		const locks = new FakeLockManager();
		const leader = await openTab({ locks });
		const follower = await openTab({ locks });

		track(leader.module.subscribe('logs', vi.fn()));
		track(follower.module.subscribe('logs', vi.fn()));
		await flush();
		expect(FakeEventSource.opened).toHaveLength(1);

		leader.document.become('hidden');
		await flush();

		expect(FakeEventSource.opened).toHaveLength(2);
		expect(FakeEventSource.opened[0].closed).toBe(true);
		expect(FakeEventSource.opened[1].closed).toBe(false);
	});
});

describe('without cross-tab support', () => {
	it('keeps a per-tab stream and closes it while hidden', async () => {
		const tab = await openTab({ crossTab: false });
		const onEvent = vi.fn();

		track(tab.module.subscribe('logs', onEvent));
		expect(FakeEventSource.opened).toHaveLength(1);
		FakeEventSource.opened[0].emit('logs', JSON.stringify({ line: 'local' }));
		expect(onEvent).toHaveBeenCalledExactlyOnceWith({ line: 'local' });

		tab.document.become('hidden');
		expect(FakeEventSource.opened[0].closed).toBe(true);

		tab.document.become('visible');
		expect(FakeEventSource.opened).toHaveLength(2);
		expect(FakeEventSource.opened[1].closed).toBe(false);
		expect(onEvent).toHaveBeenCalledTimes(2);
		expect(onEvent).toHaveBeenLastCalledWith(undefined);
	});
});
