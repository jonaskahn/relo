import { afterEach, describe, expect, it, vi } from 'vitest';

import { subscribe } from './sse';

// The console reads the daemon's live state over one shared event stream, so
// what is under test is which frames reach which listener and when the shared
// stream opens and closes.
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

const closers: Array<() => void> = [];

// Every test detaches what it subscribed, so the shared stream of one test
// never answers the next.
function track(close: () => void) {
	closers.push(close);
	return close;
}

afterEach(() => {
	for (const close of closers.splice(0)) close();
	FakeEventSource.opened = [];
	vi.unstubAllGlobals();
});

function stub() {
	vi.stubGlobal('EventSource', FakeEventSource as unknown as typeof EventSource);
	return FakeEventSource;
}

describe('subscribe', () => {
	it('opens one stream naming every channel', () => {
		const Source = stub();
		track(subscribe('logs', vi.fn()));

		expect(Source.opened).toHaveLength(1);
		expect(Source.opened[0].url).toBe('/api/v1/events/logs,quota,status');
	});

	it('shares the stream between channels', () => {
		const Source = stub();
		track(subscribe('logs', vi.fn()));
		track(subscribe('quota', vi.fn()));
		track(subscribe('status', vi.fn()));

		expect(Source.opened).toHaveLength(1);
	});

	it('hands every frame on the channel to the listener', () => {
		const Source = stub();
		const onEvent = vi.fn();
		track(subscribe('logs', onEvent));

		Source.opened[0].emit('logs', JSON.stringify({ line: 'first' }));
		Source.opened[0].emit('logs', JSON.stringify({ line: 'second' }));

		expect(onEvent).toHaveBeenCalledTimes(2);
		expect(onEvent).toHaveBeenNthCalledWith(1, { line: 'first' });
		expect(onEvent).toHaveBeenNthCalledWith(2, { line: 'second' });
	});

	it('skips a frame it cannot read and leaves the stream open', () => {
		const Source = stub();
		const onEvent = vi.fn();
		track(subscribe('logs', onEvent));

		Source.opened[0].emit('logs', 'not json at all');
		expect(onEvent).not.toHaveBeenCalled();

		// The stream survives the frame it had to skip.
		Source.opened[0].emit('logs', JSON.stringify({ line: 'after' }));
		expect(onEvent).toHaveBeenCalledExactlyOnceWith({ line: 'after' });
		expect(Source.opened[0].closed).toBe(false);
	});

	it('ignores a frame sent on another channel', () => {
		const Source = stub();
		const onEvent = vi.fn();
		track(subscribe('logs', onEvent));

		Source.opened[0].emit('quota', JSON.stringify({ other: true }));

		expect(onEvent).not.toHaveBeenCalled();
	});

	it('keeps the stream open while another channel still listens', () => {
		const Source = stub();
		const close = track(subscribe('logs', vi.fn()));
		track(subscribe('status', vi.fn()));

		close();

		expect(Source.opened[0].closed).toBe(false);
	});

	it('closes the stream with the last listener', () => {
		const Source = stub();
		const first = track(subscribe('logs', vi.fn()));
		const second = track(subscribe('status', vi.fn()));

		first();
		second();

		expect(Source.opened).toHaveLength(1);
		expect(Source.opened[0].closed).toBe(true);
	});

	it('opens a fresh stream for a subscriber arriving after the last close', () => {
		const Source = stub();
		const first = track(subscribe('logs', vi.fn()));
		first();

		track(subscribe('quota', vi.fn()));

		expect(Source.opened).toHaveLength(2);
		expect(Source.opened[1].closed).toBe(false);
		expect(Source.opened[1].listeners.has('quota')).toBe(true);
	});
});
