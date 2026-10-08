/** Names one live stream the daemon publishes. */
export type Channel = 'logs' | 'quota' | 'status';

// The console opens one stream for every channel it may listen to. A browser
// gives one origin six connections over HTTP/1.1, so a stream per subscriber
// would spend them all on the live updates and leave the console's reads
// waiting behind its own event traffic.
const STREAM = '/api/v1/events/logs,quota,status';

type Listener = (payload: unknown) => void;

let source: EventSource | null = null;
const listeners = new Map<Channel, Set<Listener>>();

function dispatch(channel: Channel) {
	return (event: Event) => {
		if (!(event instanceof MessageEvent) || typeof event.data !== 'string') return;
		let payload: unknown;
		try {
			payload = JSON.parse(event.data);
		} catch {
			// A frame the console cannot read is skipped; the stream stays open.
			return;
		}
		for (const listener of [...(listeners.get(channel) ?? [])]) listener(payload);
	};
}

function open() {
	if (source) return;
	source = new EventSource(STREAM);
	for (const channel of listeners.keys()) source.addEventListener(channel, dispatch(channel));
}

/** Hands every frame of one channel to the listener and returns the function that detaches it.
 *  The shared stream closes with the last listener. */
export function subscribe(channel: Channel, onEvent: Listener): () => void {
	let channelListeners = listeners.get(channel);
	if (!channelListeners) {
		channelListeners = new Set();
		listeners.set(channel, channelListeners);
		source?.addEventListener(channel, dispatch(channel));
	}
	channelListeners.add(onEvent);
	open();
	return () => {
		channelListeners?.delete(onEvent);
		const remaining = [...listeners.values()].some((entry) => entry.size > 0);
		if (!remaining && source) {
			source.close();
			source = null;
		}
	};
}
