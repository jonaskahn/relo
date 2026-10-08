/** A live stream announces every change, and a console that refetches on every announcement
 *  floods the six browser connections it shares with its streams. coalesceReload collapses a
 *  burst of announcements into one reload and never runs two at once: a request that lands while
 *  a reload works queues one trailing run instead of another request. */
export interface ReloadScheduler {
	schedule(): void;
	cancel(): void;
}

/** Builds the scheduler above: one reload per burst of triggers, never two at once. */
export function coalesceReload(reload: () => void | Promise<void>, delayMs = 300): ReloadScheduler {
	let timer: ReturnType<typeof setTimeout> | undefined;
	let running = false;
	let queued = false;

	async function run() {
		if (running) {
			queued = true;
			return;
		}
		running = true;
		try {
			await reload();
		} finally {
			running = false;
			if (queued) {
				queued = false;
				void run();
			}
		}
	}

	return {
		schedule() {
			if (timer !== undefined) clearTimeout(timer);
			timer = setTimeout(() => {
				timer = undefined;
				void run();
			}, delayMs);
		},
		cancel() {
			if (timer !== undefined) clearTimeout(timer);
			timer = undefined;
			queued = false;
		}
	};
}
