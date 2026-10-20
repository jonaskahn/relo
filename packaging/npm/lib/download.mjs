// Fetches a URL with a few spaced attempts. Kept out of install.mjs so the
// retry and error rules can be tested without running a download.
//
// A permanent client error fails fast: retrying a missing asset only burns
// three waits before printing the same dead end. Network errors, throttling,
// and 5xx keep the retries, since GitHub answers a cold CDN edge with an
// error often enough that a single attempt reads as a broken install.
const ATTEMPTS = 3;

// failFast reports whether another attempt would hit the same wall. 408
// timed out and 429 was throttled, so both are worth retrying; every other
// 4xx names something the next attempt will hit too.
function failFast(status) {
	if (status === 408 || status === 429 || status >= 500) return false;
	return status >= 400 && status < 500;
}

// fetchBytes returns the body of a URL. fetchImpl and sleep take arguments
// so tests can fake the network and skip the waits. onProgress is called with
// the bytes received so far, the total when the server sends a length, and
// the milliseconds since the body started arriving. onRetry is told about
// each attempt that will be retried, so the caller can show it where the user
// is watching.
export async function fetchBytes(
	target,
	what,
	{
		fetchImpl = fetch,
		sleep = (ms) => new Promise((done) => setTimeout(done, ms)),
		onProgress,
		onRetry = (message) => console.warn(`relo: ${message}`)
	} = {}
) {
	let last;
	for (let attempt = 1; attempt <= ATTEMPTS; attempt += 1) {
		try {
			const response = await fetchImpl(target, { redirect: 'follow' });
			if (!response.ok) {
				if (failFast(response.status)) {
					throw Object.assign(
						new Error(
							`no ${what} in the release (HTTP ${response.status}). ` +
								'No release may be published yet; rerun the installer with --relo-version <tag> for a published tag, or without it for the latest release.'
						),
						{ permanent: true }
					);
				}
				throw new Error(`${response.status} ${response.statusText}`);
			}
			return await readBody(response, onProgress);
		} catch (error) {
			if (error?.permanent) throw error;
			last = error;
			if (attempt < ATTEMPTS) {
				onRetry(`${what} failed (${error.message}), retrying`);
				await sleep(attempt * 1000);
			}
		}
	}
	throw new Error(`could not download ${what} from ${target}: ${last.message}`);
}

// readBody reads the response body, reporting the bytes as they arrive when
// the response is a stream. A response without a readable stream -- a test
// double, or an older runtime -- is read whole.
async function readBody(response, onProgress) {
	if (!onProgress || !response.body?.getReader) {
		return Buffer.from(await response.arrayBuffer());
	}
	const total = Number(response.headers?.get?.('content-length')) || 0;
	const started = Date.now();
	const reader = response.body.getReader();
	const chunks = [];
	let received = 0;
	for (;;) {
		const { done, value } = await reader.read();
		if (done) break;
		chunks.push(Buffer.from(value));
		received += value.length;
		onProgress(received, total, Date.now() - started);
	}
	return Buffer.concat(chunks);
}
