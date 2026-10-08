import { describe, expect, test } from 'vitest';

import {
	integrationEndpoint,
	integrationPath,
	isSetupRefusal,
	keyPathOf,
	needsAttention,
	refusalCode,
	setupBlocked,
	type IntegrationAction
} from './integration-actions';
import type { IntegrationView } from './types';

// The routes the management API mounts for one agent, read from the daemon's
// own route table rather than from the mapping under test.
const daemonRoutes = [
	'enable',
	'disable',
	'rotate',
	'repair',
	'restart',
	'restore',
	'verify',
	'models',
	'chat',
	'context',
	'token'
];

const actions: IntegrationAction[] = [
	'setup',
	'remove',
	'verify',
	'repair',
	'restart',
	'restore',
	'rotate',
	'models',
	'chat',
	'context'
];

describe('the integration actions', () => {
	test('every action names a route the daemon mounts', () => {
		for (const action of actions) {
			expect(daemonRoutes).toContain(integrationEndpoint(action));
		}
	});

	test('the page word and the daemon route differ where an operator expects it', () => {
		expect(integrationEndpoint('setup')).toBe('enable');
		expect(integrationEndpoint('remove')).toBe('disable');
	});

	test('a path names one agent, encoded', () => {
		expect(integrationPath('codex', 'remove')).toBe('/integrations/codex/disable');
		expect(integrationPath('a/b', 'verify')).toBe('/integrations/a%2Fb/verify');
	});

	test('a missing install blocks set-up, and a foreign provider can be replaced', () => {
		expect(refusalCode([{ refused: true, code: 'not_installed' }])).toBe('not_installed');
		expect(setupBlocked('not_installed')).toBe(true);
		expect(setupBlocked('relative_path')).toBe(true);
		expect(setupBlocked('unrecognised')).toBe(true);
		expect(refusalCode([{ refused: true, code: 'foreign_key' }])).toBe('foreign_key');
		expect(setupBlocked('foreign_key')).toBe(false);
		expect(refusalCode([{ refused: false, code: 'not_installed' }])).toBe('');
	});

	test('the key path comes from a written file, then from the preview', () => {
		expect(
			keyPathOf({
				files: [{ kind: 'key', path: '/state/key' }],
				preview: [{ kind: 'key', path: '/other' }]
			})
		).toBe('/state/key');
		expect(keyPathOf({ preview: [{ kind: 'key', path: '/state/key' }] })).toBe('/state/key');
		expect(keyPathOf({})).toBe('');
	});

	test('attention is limited to set-up agents that need repair or restart', () => {
		const agent = {
			id: 'codex',
			client: 'codex',
			summary: '',
			protocol: 'openai',
			manages_files: true,
			enabled: true,
			state: 'on',
			files: []
		} satisfies IntegrationView;

		expect(needsAttention({ ...agent, state: 'drifted' })).toBe(true);
		expect(needsAttention({ ...agent, models_stale: true })).toBe(true);
		expect(needsAttention({ ...agent, restart_needed: true })).toBe(true);
		expect(
			needsAttention({
				...agent,
				files: [{ kind: 'config', path: '/config', drifted: true, present: true }]
			})
		).toBe(true);
		expect(needsAttention({ ...agent, enabled: false, restart_needed: true })).toBe(false);
		expect(needsAttention(agent)).toBe(false);
	});
});

describe('the guard over a refusal the daemon named', () => {
	test('accepts only the codes the console still renders', () => {
		expect(isSetupRefusal('not_installed')).toBe(true);
		expect(isSetupRefusal('relative_path')).toBe(true);
		expect(isSetupRefusal('stale_preview')).toBe(false);
		expect(isSetupRefusal('')).toBe(false);
	});
});
