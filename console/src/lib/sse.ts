/** Names one live stream the daemon publishes. */
export type Channel = 'logs' | 'quota' | 'status';

// The console opens one stream for every channel it may listen to. A browser
// gives one origin six connections over HTTP/1.1, so a stream per subscriber
// would spend them all on the live updates and leave the console's reads
// waiting behind its own event traffic.
const STREAM = '/api/v1/events/logs,quota,status';
const LOCK = 'relo-events';
const BUS = 'relo-events';
const CHANNELS: Channel[] = ['logs', 'quota', 'status'];

type Listener = (payload: unknown) => void;
type Frame = { channel: Channel; data: string };

// One document per page, read once so a test can give each simulated tab its
// own visibility state without the modules sharing the global.
const page = typeof document === 'undefined' ? null : document;

// Web Locks and BroadcastChannel are how one tab owns the stream and the rest
// read its frames. Without both, every tab keeps its own stream.
const crossTab =
	typeof navigator !== 'undefined' &&
	typeof navigator.locks?.request === 'function' &&
	typeof BroadcastChannel === 'function';

let source: EventSource | null = null;
let bus: BroadcastChannel | null = null;
let lockController: AbortController | null = null;
let releaseLock: (() => void) | null = null;
let leading = false;
let hidden = page?.visibilityState === 'hidden';
let wasHidden = hidden;
const listeners = new Map<Channel, Set<Listener>>();

function isChannel(value: unknown): value is Channel {
	return value === 'logs' || value === 'quota' || value === 'status';
}

function isFrame(value: unknown): value is Frame {
	return (
		typeof value === 'object' &&
		value !== null &&
		'channel' in value &&
		isChannel(value.channel) &&
		'data' in value &&
		typeof value.data === 'string'
	);
}

function hasListeners(): boolean {
	return [...listeners.values()].some((entry) => entry.size > 0);
}

function deliver(channel: Channel, data: string) {
	let payload: unknown;
	try {
		payload = JSON.parse(data);
	} catch {
		// A frame the console cannot read is skipped; the stream stays open.
		return;
	}
	for (const listener of [...(listeners.get(channel) ?? [])]) listener(payload);
}

function fireAll() {
	for (const channelListeners of listeners.values()) {
		for (const listener of [...channelListeners]) listener(undefined);
	}
}

function attach(channel: Channel) {
	source?.addEventListener(channel, (event) => {
		if (!(event instanceof MessageEvent) || typeof event.data !== 'string') return;
		deliver(channel, event.data);
		bus?.postMessage({ channel, data: event.data } satisfies Frame);
	});
}

function openSource() {
	if (source || hidden || !hasListeners()) return;
	source = new EventSource(STREAM);
	// Every tab reads every channel: the tab holding the lock may not be the
	// tab whose listener needs a channel, so the stream must carry all three.
	for (const channel of CHANNELS) attach(channel);
}

function closeSource() {
	source?.close();
	source = null;
}

function ensureBus() {
	if (!crossTab || bus) return;
	bus = new BroadcastChannel(BUS);
	bus.onmessage = (event: MessageEvent<unknown>) => {
		if (hidden || !isFrame(event.data)) return;
		deliver(event.data.channel, event.data.data);
	};
}

function closeBus() {
	bus?.close();
	bus = null;
}

function releaseLeadership() {
	const resolve = releaseLock;
	releaseLock = null;
	if (leading) {
		leading = false;
		closeSource();
	}
	if (lockController) {
		lockController.abort();
		lockController = null;
	}
	resolve?.();
}

function requestLeadership() {
	if (!crossTab || leading || lockController || hidden || !hasListeners()) return;
	const controller = new AbortController();
	lockController = controller;
	navigator.locks
		.request(LOCK, { signal: controller.signal }, async () => {
			if (controller.signal.aborted) return;
			lockController = null;
			leading = true;
			openSource();
			await new Promise<void>((resolve) => {
				releaseLock = resolve;
			});
		})
		.catch(() => {
			// Hiding the tab or dropping the last listener aborts a pending request.
		})
		.finally(() => {
			if (lockController === controller) lockController = null;
		});
}

function start() {
	if (!hasListeners() || hidden) return;
	if (!crossTab) {
		openSource();
		return;
	}
	ensureBus();
	requestLeadership();
}

function stop() {
	if (!crossTab) {
		closeSource();
		return;
	}
	releaseLeadership();
	closeBus();
}

function onVisibilityChange() {
	if (!page) return;
	if (page.visibilityState === 'hidden') {
		hidden = true;
		wasHidden = true;
		if (crossTab) releaseLeadership();
		else closeSource();
		return;
	}
	hidden = false;
	if (wasHidden) {
		wasHidden = false;
		fireAll();
	}
	start();
}

page?.addEventListener('visibilitychange', onVisibilityChange);

/** Hands every frame of one channel to the listener and returns the function that detaches it.
 *  One visible tab owns the shared stream and republishes its frames; every other tab reads those
 *  frames without opening a connection of its own. The stream closes with the last listener. */
export function subscribe(channel: Channel, onEvent: Listener): () => void {
	let channelListeners = listeners.get(channel);
	if (!channelListeners) {
		channelListeners = new Set();
		listeners.set(channel, channelListeners);
	}
	channelListeners.add(onEvent);
	start();
	return () => {
		channelListeners?.delete(onEvent);
		if (!hasListeners()) stop();
	};
}
