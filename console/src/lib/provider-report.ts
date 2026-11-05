import type { UsageCapture } from '$lib/types';

/** Only an actual provider error reply permits sharing a provider exchange. */
export function hasProviderError(captures: UsageCapture[]): boolean {
	return captures.some((capture) => capture.kind === 'provider_response' && capture.status >= 400);
}

/** The report endpoint excludes agent messages and account identities. */
export interface ProviderReport {
	request_id: string;
	relo_version: string;
	generated_at: string;
	attempts: {
		ordinal: number;
		provider: string;
		model: string;
		status: number;
		duration_ms: number;
	}[];
}
