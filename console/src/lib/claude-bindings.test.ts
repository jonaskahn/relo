import { describe, expect, it } from 'vitest';

import {
	bindingBody,
	newBinding,
	targetFromValue,
	targetValue,
	unboundOfficial
} from './claude-bindings';
import type { ClaudeBindingEntry } from './types';

describe('claude bindings', () => {
	it('round-trips a route and a model through the selector value', () => {
		const route = { kind: 'route', group_id: 'cheap' } as const;
		const model = { kind: 'model', provider_id: 'a/b', model_id: 'm::1' } as const;
		expect(targetFromValue(targetValue(route))).toEqual(route);
		expect(targetFromValue(targetValue(model))).toEqual(model);
	});

	it('reads nothing out of a value that names no target', () => {
		for (const value of ['', 'nope', '[]', '["route","","",""]', '["model","p","",""]']) {
			expect(targetFromValue(value)).toBeNull();
		}
	});

	it('offers only the official ids that are not bound yet', () => {
		const bound = [{ native_id: 'claude-opus-5' }] as ClaudeBindingEntry[];
		const official = [
			{ id: 'claude-opus-5', family: 'opus', source: 'seed' },
			{ id: 'claude-haiku-4-5', family: 'haiku', source: 'seed' }
		];
		expect(unboundOfficial(official, bound).map((model) => model.id)).toEqual(['claude-haiku-4-5']);
	});

	it('starts a binding with Relo answering and nothing spent at Anthropic', () => {
		const created = newBinding('claude-haiku-4-5', { kind: 'route', group_id: 'cheap' });
		expect(created).toMatchObject({ direction: 'replace', on_failure: 'none', enabled: true });
	});

	it('drops the failure choice from a binding that asks Anthropic first', () => {
		const row = { ...newBinding('claude-haiku-4-5', { kind: 'route', group_id: 'c' }) };
		expect(bindingBody({ ...row, direction: 'fallback', on_failure: 'native' }).on_failure).toBe(
			'none'
		);
		expect(bindingBody({ ...row, on_failure: 'native' }).on_failure).toBe('native');
	});
});
