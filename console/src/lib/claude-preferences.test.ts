import { describe, expect, test } from 'vitest';

import {
	claudeEffortChoices,
	claudeEffortLevels,
	claudePreferenceModels,
	claudePreferencePayload
} from './claude-preferences';

describe('Claude Code preference state', () => {
	test('opus 4.6 and sonnet 4.6 stop at high, and other models include extra high', () => {
		expect(claudeEffortLevels('claude-opus-4-6')).toEqual(['low', 'medium', 'high']);
		expect(claudeEffortLevels('claude-sonnet-4-6[1m]')).toEqual(['low', 'medium', 'high']);
		expect(claudeEffortLevels('claude-relo-openai--gpt-4o')).toEqual([
			'low',
			'medium',
			'high',
			'xhigh'
		]);
		expect(claudeEffortLevels('claude-opus-4-6')).not.toContain('max');
	});

	test('a saved model stays visible when the current list dropped it', () => {
		const listed = claudePreferenceModels(
			'opus',
			[{ id: 'claude-relo-openai--gpt-4o', name: 'GPT-4o' }],
			true,
			[]
		);
		expect(listed.map((model) => model.id)).toEqual(['opus', 'claude-relo-openai--gpt-4o']);
	});

	test('a ready test-pane list replaces the catalog, and a saved effort stays selectable', () => {
		const listed = claudePreferenceModels(
			'claude-opus-4-6',
			[{ id: 'claude-opus-4-6', name: 'Opus' }],
			true,
			[{ id: 'claude-sonnet-4-6', efforts: ['low', 'medium', 'high'] }]
		);
		expect(listed.map((model) => model.id)).toEqual(['claude-opus-4-6']);
		expect(claudeEffortChoices('claude-opus-4-6', 'xhigh', ['low', 'medium', 'high'])).toEqual([
			'xhigh',
			'low',
			'medium',
			'high'
		]);
	});

	test('the payload keeps auto as an empty model and a bounded window', () => {
		expect(
			claudePreferencePayload({ model: 'auto', effort: 'high', compact: 'auto', tokens: '' })
		).toEqual({ ok: true, body: { model: '', effort: '', auto_compact: 'auto' } });
		expect(
			claudePreferencePayload({
				model: 'claude-relo-openai--gpt-4o',
				effort: 'xhigh',
				compact: 'custom',
				tokens: '200000'
			})
		).toEqual({
			ok: true,
			body: { model: 'claude-relo-openai--gpt-4o', effort: 'xhigh', auto_compact: '200000' }
		});
		expect(
			claudePreferencePayload({
				model: 'opus',
				effort: 'high',
				compact: 'custom',
				tokens: '99999'
			}).ok
		).toBe(false);
		expect(
			claudePreferencePayload({
				model: 'auto',
				effort: 'auto',
				compact: 'custom',
				tokens: '200000'
			})
		).toEqual({ ok: false, error: 'model' });
	});
});
