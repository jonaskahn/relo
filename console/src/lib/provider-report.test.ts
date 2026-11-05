import { describe, it, expect } from 'vitest';
import { hasProviderError } from './provider-report';
import type { UsageCapture, UsageCaptureKind } from './types';

const capture = (kind: UsageCaptureKind, status: number): UsageCapture => ({
	id: 1,
	kind,
	status,
	method: '',
	url: '',
	headers: {},
	body_bytes: 0,
	truncated: false
});

describe('provider-error report eligibility', () => {
	it.each([429, 500])('accepts a provider response %s', (status) => {
		expect(hasProviderError([capture('provider_response', status)])).toBe(true);
	});
	it.each([
		{ captures: [] },
		{ captures: [capture('provider_response', 200)] },
		{ captures: [capture('agent_response', 429)] },
		{ captures: [capture('agent_request', 400)] },
		{ captures: [capture('provider_request', 500)] }
	])('rejects captures without a provider error: %j', ({ captures }) => {
		expect(hasProviderError(captures)).toBe(false);
	});
});
