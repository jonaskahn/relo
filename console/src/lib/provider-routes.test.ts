import { describe, expect, test } from 'vitest';

import { modelsRedirectTarget } from './provider-routes';

describe('the models redirect', () => {
	test('keeps a named provider and opens its models tab', () => {
		expect(modelsRedirectTarget('?provider=openai')).toBe(
			'/connections?provider=openai&tab=models'
		);
		expect(modelsRedirectTarget('?provider=openai&alias=1')).toBe(
			'/connections?provider=openai&tab=models'
		);
		expect(modelsRedirectTarget('?provider=a%2Fb')).toBe('/connections?provider=a%2Fb&tab=models');
	});

	test('sends everything else to the all-models view', () => {
		expect(modelsRedirectTarget('')).toBe('/connections?view=models');
		expect(modelsRedirectTarget('?alias=1')).toBe('/connections?view=models');
		expect(modelsRedirectTarget('?provider=')).toBe('/connections?view=models');
	});
});
