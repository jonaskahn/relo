import { describe, expect, it } from 'vitest';

import { isPlanPricedConnection } from './provider-plan';

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
		expect(
			isPlanPricedConnection({
				base_url: 'https://coding.dashscope.aliyuncs.com/v1'
			})
		).toBe(true);
		expect(
			isPlanPricedConnection({
				base_url: 'https://coding-intl.dashscope.aliyuncs.com/v1'
			})
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
