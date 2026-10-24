import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';
import type { ProbeResult } from '$lib/types';

import StepReview from './step-review.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

const probe: ProbeResult = {
	probe_id: 'probe-1',
	expires_at_ms: 0,
	checks: [],
	counts: { listed: 1, matched: 1, priced: 1, from_listing: 1, from_manual: 0 },
	models: [],
	modelsdev_state: {
		source_url: '',
		fetched_at_ms: 0,
		stale: false,
		available: false,
		providers_count: 0,
		models_count: 0,
		last_attempt_at_ms: 0
	}
};

describe('the review step', () => {
	// The keyless pool is free because the provider logs what it is sent, so an
	// operator reads that where they choose what routes to it.
	it('warns that the keyless pool may log prompts', () => {
		const html = render(StepReview, {
			props: { probe, templateId: 'kilo-free', onchange: () => {} }
		}).body;

		expect(html).toContain('may be logged by the provider');
	});

	it('leaves the warning off for every other provider', () => {
		const html = render(StepReview, {
			props: { probe, templateId: 'opencode-free', onchange: () => {} }
		}).body;

		expect(html).not.toContain('may be logged by the provider');
	});

	// Review is the last look before the write, so it names what the connection
	// will cost through the same chip every other surface wears.
	it('names the billing lane the connection will be stored under', () => {
		const plan = render(StepReview, {
			props: {
				probe,
				templateId: 'alibaba-coding-plan-cn',
				billing: { template_id: 'alibaba-coding-plan-cn' },
				onchange: () => {}
			}
		}).body;
		expect(plan).toContain('data-paid-chip');

		const payg = render(StepReview, {
			props: {
				probe,
				templateId: 'openai',
				billing: { template_id: 'openai', kind: 'key' },
				onchange: () => {}
			}
		}).body;
		expect(payg).toContain('data-payg-chip');
	});

	it('leaves the billing line off a key added to a connection it already has', () => {
		const html = render(StepReview, {
			props: { probe, isAddKey: true, targetLabel: 'OpenAI', onchange: () => {} }
		}).body;

		expect(html).not.toContain('data-paid-chip');
		expect(html).not.toContain('data-payg-chip');
	});
});
