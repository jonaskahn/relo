import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

import {
	billingLane,
	isOpenCodePlan,
	isPlanConnection,
	isPlanPricedConnection,
	PLAN_TEMPLATES
} from './provider-plan';

describe('isPlanPricedConnection', () => {
	it('matches every identifier family a plan is sold under', () => {
		for (const id of [
			'xiaomi-token-plan-sgp',
			'alibaba-token-plan',
			'alibaba-coding-plan',
			'alibaba-coding-plan-cn',
			'zai-coding-plan',
			'zhipuai-coding-plan',
			'kimi-code-plan-cn',
			'kimi-code-plan-global',
			'minimax-coding-plan',
			'minimax-cn-coding-plan',
			'kuae-cloud-coding-plan',
			'umans-ai-coding-plan',
			'tencent-coding-plan',
			'volcengine-coding-plan',
			'stepfun-step-plan',
			'stepfun-ai-step-plan'
		]) {
			expect(isPlanPricedConnection({ id }), id).toBe(true);
		}
	});

	it('reads the template and the models.dev identifier, in any case', () => {
		expect(isPlanPricedConnection({ template_id: 'Zai-Coding-Plan' })).toBe(true);
		expect(isPlanPricedConnection({ modelsdev_provider_id: 'stepfun-step-plan' })).toBe(true);
	});

	it('matches the two plan-only DashScope hosts', () => {
		expect(isPlanPricedConnection({ base_url: 'https://coding.dashscope.aliyuncs.com/v1' })).toBe(
			true
		);
		expect(
			isPlanPricedConnection({ base_url: 'https://coding-intl.dashscope.aliyuncs.com/v1' })
		).toBe(true);
		expect(isPlanPricedConnection({ base_url: 'https://token-plan.example.test/v1' })).toBe(true);
	});

	it('leaves the pay-as-you-go twins outside it', () => {
		expect(isPlanPricedConnection({ id: 'alibaba', template_id: 'alibaba' })).toBe(false);
		expect(isPlanPricedConnection({ id: 'zai' })).toBe(false);
		expect(
			isPlanPricedConnection({ base_url: 'https://dashscope.aliyuncs.com/compatible-mode/v1' })
		).toBe(false);
		expect(isPlanPricedConnection({ base_url: 'not a url' })).toBe(false);
	});

	it('names no free lane: its caller checks that first', () => {
		expect(isPlanPricedConnection({ template_id: 'opencode-free' })).toBe(false);
		expect(isPlanPricedConnection({ template_id: 'kilo-free' })).toBe(false);
	});
});

describe('isOpenCodePlan', () => {
	it('finds the gateway by identifier or by path', () => {
		expect(isOpenCodePlan({ template_id: 'opencode-go' })).toBe(true);
		expect(isOpenCodePlan({ modelsdev_provider_id: 'opencode' })).toBe(true);
		expect(isOpenCodePlan({ base_url: 'https://opencode.ai/zen/go' })).toBe(true);
		expect(isOpenCodePlan({ base_url: 'https://opencode.ai/inference/go' })).toBe(true);
	});

	it('leaves a gateway URL that names no plan path alone', () => {
		expect(isOpenCodePlan({ base_url: 'https://opencode.ai/docs' })).toBe(false);
		expect(isOpenCodePlan({ base_url: 'not a url' })).toBe(false);
	});
});

describe('isPlanConnection', () => {
	it('counts every lane a subscription is sold through', () => {
		for (const template of PLAN_TEMPLATES) {
			expect(isPlanConnection({ template_id: template }), template).toBe(true);
		}
		expect(isPlanConnection({ origin: 'signin' })).toBe(true);
		expect(isPlanConnection({ kind: 'signin' })).toBe(true);
		expect(isPlanConnection({ auth: 'oauth' })).toBe(true);
		expect(isPlanConnection({ modelsdev_provider_id: 'zai-coding-plan' })).toBe(true);
	});

	// A curated plan is a plain key with nothing in its identifier saying so, so
	// the list is what makes it a plan.
	it('counts a curated template the pay-as-you-go rules would miss', () => {
		const plan = {
			template_id: 'open-design',
			kind: 'key',
			base_url: 'https://amr-link.open-design.ai/v1'
		};
		expect(isPlanPricedConnection(plan)).toBe(false);
		expect(isOpenCodePlan(plan)).toBe(false);
		expect(isPlanConnection(plan)).toBe(true);
	});

	it('leaves the keyless lanes and a plain key outside it', () => {
		expect(isPlanConnection({ template_id: 'kilo-free' })).toBe(false);
		expect(isPlanConnection({ template_id: 'opencode-free' })).toBe(false);
		expect(isPlanConnection({ template_id: 'alibaba', kind: 'key' })).toBe(false);
	});
});

describe('billingLane', () => {
	it('names the four lanes', () => {
		expect(billingLane({ template_id: 'kilo-free', kind: 'key' })).toBe('free');
		expect(billingLane({ template_id: 'open-design', kind: 'key' })).toBe('plan');
		expect(billingLane({ template_id: 'claude', kind: 'signin' })).toBe('plan');
		expect(billingLane({ template_id: 'alibaba', kind: 'key' })).toBe('payg');
		expect(billingLane({ template_id: 'bedrock', kind: 'cloud' })).toBe('payg');
		expect(billingLane({ template_id: 'ollama', kind: 'local' })).toBe('local');
		// A connection that names no template, and one that names no kind, are
		// both read rather than rejected.
		expect(billingLane({ id: 'openai', kind: 'key' })).toBe('payg');
		expect(billingLane({})).toBe('local');
	});
});

// The two copies of this rule are the console's and the tray's, and they must
// list the same curated templates: a connection the chip calls a plan and the
// tray charges for is the bug this test exists to stop.
describe('the tray copy', () => {
	it('lists the same plan templates as the daemon', () => {
		const source = readFileSync('../daemon/internal/cli/desktop_source.go', 'utf8');
		const match = source.match(/var planTemplates = \[\]string\{([^}]*)\}/);
		expect(match).not.toBeNull();
		const goTemplates = (match?.[1] ?? '')
			.split(',')
			.map((entry) => entry.trim().replace(/^"|"$/g, ''))
			.filter((entry) => entry !== '')
			.sort();
		expect(goTemplates).toEqual([...PLAN_TEMPLATES].sort());
	});
});
