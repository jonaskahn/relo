import type { UsageLogRow } from './types';

/** Builds a request-log row for a test, with the fields a case does not name filled in. */
export function usageRowOf(patch: Partial<UsageLogRow> = {}): UsageLogRow {
	return {
		ID: 1,
		RequestID: 'req-1',
		TimestampMs: 1,
		Provider: 'openai',
		Model: 'gpt-5',
		RequestedModel: 'gpt-5',
		GroupID: null,
		CredentialLabel: 'work',
		Surface: 'chat-completions',
		Status: 200,
		DurationMs: 250,
		InputTokens: 10,
		OutputTokens: 20,
		CacheReadTokens: 0,
		CacheWriteTokens: 0,
		EstimatedCostMicros: null,
		RouteProvider: 'openai',
		RouteReason: '',
		ClientKeyID: 'key-1',
		ClientKeyName: 'test agent',
		ClientApp: 'codex',
		Origin: 'external',
		Attempts: 1,
		Retried: false,
		...patch
	};
}
