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
// so tests can fake the network and skip the waits.
export async function fetchBytes(
	target,
	what,
	{ fetchImpl = fetch, sleep = (ms) => new Promise((done) => setTimeout(done, ms)) } = {}
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
			return Buffer.from(await response.arrayBuffer());
		} catch (error) {
			if (error?.permanent) throw error;
			last = error;
			if (attempt < ATTEMPTS) {
				console.warn(`relo: ${what} failed (${error.message}), retrying`);
				await sleep(attempt * 1000);
			}
		}
	}
	throw new Error(`could not download ${what} from ${target}: ${last.message}`);
}
