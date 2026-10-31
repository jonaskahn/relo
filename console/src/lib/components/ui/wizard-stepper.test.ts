import { render } from 'svelte/server';
import { addMessages, init } from 'svelte-i18n';
import { beforeAll, describe, expect, it } from 'vitest';

import en from '$lib/i18n/locales/en.json';

import WizardStepper from './wizard-stepper.svelte';

beforeAll(() => {
	addMessages('en', en);
	init({ fallbackLocale: 'en', initialLocale: 'en' });
});

describe('the wizard stepper', () => {
	it('keeps the markers and caption in separate shrinkable rows on a phone', () => {
		const html = render(WizardStepper, {
			props: {
				steps: ['provider', 'connect', 'verify', 'review'],
				current: 'provider',
				labelKeys: {
					provider: 'ui.pages.providersPage.add.stepProvider',
					connect: 'ui.pages.providersPage.add.stepConnect',
					verify: 'ui.pages.providersPage.add.stepVerify',
					review: 'ui.pages.providersPage.add.stepReview'
				},
				onjump: () => {},
				stepOfKey: 'ui.pages.providersPage.add.stepOf'
			}
		}).body;

		expect(html).toContain('flex-col gap-1 sm:flex-row');
		expect(html).toContain('flex min-w-0 items-center gap-1 sm:flex-1');
		expect(html).toContain('min-w-0 truncate text-xs text-muted-foreground sm:hidden');
	});
});
