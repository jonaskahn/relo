import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import RoutingFlow from './routing-flow.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the routing-flow diagram', () => {
	it('is an accessible image with a title and description', () => {
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				members: [
					{
						member: {
							provider_id: 'openai',
							model_id: 'gpt-5',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'OpenAI',
						modelName: 'GPT-5',
						accounts: [],
						windows: []
					}
				],
				current: 0,
				next: 0,
				hidden: 0
			}
		}).body;
		expect(html).toContain('role="img"');
		expect(html).toContain('<title id="route-flow-title"');
		expect(html).toContain('<desc id="route-flow-desc"');
	});

	it('uses semantic theme tokens for muted text and the legend', () => {
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				members: [
					{
						member: {
							provider_id: 'openai',
							model_id: 'gpt-5',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'OpenAI',
						modelName: 'GPT-5',
						accounts: [],
						windows: []
					}
				],
				current: 0,
				next: 0,
				hidden: 0
			}
		}).body;
		expect(html).toContain('fill="var(--muted)"');
		expect(html).toContain('stroke="var(--muted)"');
		expect(html).toContain('fill="var(--ink)"');
		expect(html).toContain('background: var(--accent)');
		expect(html).not.toContain('class="fill-muted');
		expect(html).not.toContain('class="stroke-muted');
		expect(html).not.toContain('class="fill-accent');
		expect(html).not.toContain('class="h-2 w-4 rounded-sm bg-accent"');
	});

	it('renders current and next markers with the serving account', () => {
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				members: [
					{
						member: {
							provider_id: 'openai',
							model_id: 'gpt-5',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'OpenAI',
						modelName: 'GPT-5',
						accounts: [],
						windows: []
					},
					{
						member: {
							provider_id: 'anthropic',
							model_id: 'claude',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'Anthropic',
						modelName: 'Claude',
						accounts: [],
						windows: []
					}
				],
				current: 0,
				currentAccount: 'work',
				next: 1,
				hidden: 0
			}
		}).body;
		expect(html).toContain('In use');
		expect(html).toContain('Next start');
		expect(html).toContain('work');
	});

	it('renders readable quota windows and no attempt ordinals', () => {
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				members: [
					{
						member: {
							provider_id: 'openai',
							model_id: 'gpt-5',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'OpenAI',
						modelName: 'GPT-5',
						accounts: [],
						windows: [
							{
								credential_id: 'a',
								connection_id: 'openai',
								label: 'Week',
								window: '7d',
								window_seconds: 604_800,
								used_percent: 26,
								reset_at_ms: 0,
								source: 'probe',
								observed_at_ms: 0,
								stale: false
							}
						]
					}
				],
				current: 0,
				next: 0,
				hidden: 0
			}
		}).body;
		expect(html).toContain('7d');
		expect(html).toContain('26%');
		expect(html).not.toMatch(/#\d+ \d{3}/);
	});

	it('collapses more than nine members into an overflow row', () => {
		const members = Array.from({ length: 11 }, (_, index) => ({
			member: {
				provider_id: 'openai',
				model_id: 'model-' + index,
				weight: 1,
				enabled: true,
				eligible: true
			},
			providerLabel: 'OpenAI',
			modelName: 'Model ' + index,
			accounts: [],
			windows: []
		}));
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				members: members.slice(0, 9),
				current: 0,
				next: 0,
				hidden: 2
			}
		}).body;
		expect(html).toContain('+2 more members');
	});

	it('names the Agent Harness node', () => {
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				members: [
					{
						member: {
							provider_id: 'openai',
							model_id: 'gpt-5',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'OpenAI',
						modelName: 'GPT-5',
						accounts: [],
						windows: []
					}
				],
				current: 0,
				next: 0,
				hidden: 0
			}
		}).body;
		expect(html).toContain('Agent Harness');
	});

	it('draws a smooth current path with one reduced-motion-aware moving dot', () => {
		const html = render(RoutingFlow, {
			props: {
				groupLabel: 'Fast',
				members: [
					{
						member: {
							provider_id: 'openai',
							model_id: 'gpt-5',
							weight: 1,
							enabled: true,
							eligible: true
						},
						providerLabel: 'OpenAI',
						modelName: 'GPT-5',
						accounts: [],
						windows: []
					}
				],
				current: 0,
				next: 0,
				hidden: 0
			}
		}).body;
		expect(html).toMatch(/data-route-current[^>]*d="M [^"]* C /);
		expect(html.match(/<animateMotion/g)).toHaveLength(1);
		expect(html).toContain('route-moving-dot');
		const source = readFileSync(new URL('./routing-flow.svelte', import.meta.url), 'utf8');
		expect(source).toContain('@media (prefers-reduced-motion: reduce)');
		expect(source).toContain('.route-moving-dot');
		expect(html).not.toContain('min-w-[1120px]');
		expect(html).not.toContain('overflow-x-auto');
	});
});
