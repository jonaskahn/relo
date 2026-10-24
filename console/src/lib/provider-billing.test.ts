import { describe, expect, it } from 'vitest';

import { isPaidPlanConnection, isPayAsYouGoConnection } from './provider-billing';

describe('isPaidPlanConnection', () => {
	it('treats the two keyless lanes as neither paid nor metered', () => {
		expect(isPaidPlanConnection({ template_id: 'kilo-free' })).toBe(false);
		expect(isPaidPlanConnection({ template_id: 'opencode-free' })).toBe(false);
	});

	it('counts the subscription templates as paid', () => {
		expect(isPaidPlanConnection({ template_id: 'open-design' })).toBe(true);
		expect(isPaidPlanConnection({ template_id: 'opencode-go' })).toBe(true);
	});

	it('counts a sign-in connection as paid from any of its three fields', () => {
		expect(isPaidPlanConnection({ origin: 'signin' })).toBe(true);
		expect(isPaidPlanConnection({ kind: 'signin' })).toBe(true);
		expect(isPaidPlanConnection({ auth: 'oauth' })).toBe(true);
	});

	it('finds a token plan in any identifier or in the host', () => {
		expect(isPaidPlanConnection({ id: 'vendor-token-plan' })).toBe(true);
		expect(isPaidPlanConnection({ template_id: 'Vendor-Token-Plan' })).toBe(true);
		expect(isPaidPlanConnection({ modelsdev_provider_id: 'token-plan' })).toBe(true);
		expect(isPaidPlanConnection({ base_url: 'https://token-plan.example.test/v1' })).toBe(true);
	});

	it('finds the OpenCode gateway by identifier or by path', () => {
		expect(isPaidPlanConnection({ template_id: 'opencode-go' })).toBe(true);
		expect(isPaidPlanConnection({ modelsdev_provider_id: 'opencode' })).toBe(true);
		expect(isPaidPlanConnection({ base_url: 'https://opencode.ai/zen/go' })).toBe(true);
		expect(isPaidPlanConnection({ base_url: 'https://opencode.ai/inference/go' })).toBe(true);
	});

	it('lets only the declared free template out of the OpenCode lane', () => {
		expect(
			isPaidPlanConnection({ template_id: 'opencode-free', base_url: 'https://opencode.ai/zen' })
		).toBe(false);
		expect(isPaidPlanConnection({ base_url: 'https://opencode.ai/zen' })).toBe(true);
	});

	it('rejects an address it cannot parse', () => {
		expect(isPaidPlanConnection({ base_url: 'not a url' })).toBe(false);
	});

	it('leaves a key or a local endpoint outside the paid lane', () => {
		expect(isPaidPlanConnection({ kind: 'key' })).toBe(false);
		expect(isPaidPlanConnection({ kind: 'local' })).toBe(false);
	});
});

describe('isPayAsYouGoConnection', () => {
	it('counts a key and a cloud connection as metered', () => {
		expect(isPayAsYouGoConnection({ kind: 'key' })).toBe(true);
		expect(isPayAsYouGoConnection({ kind: 'cloud' })).toBe(true);
	});

	it('leaves the free, paid, and local lanes out of it', () => {
		expect(isPayAsYouGoConnection({ template_id: 'kilo-free' })).toBe(false);
		expect(isPayAsYouGoConnection({ template_id: 'open-design' })).toBe(false);
		expect(isPayAsYouGoConnection({ kind: 'local' })).toBe(false);
	});
});
