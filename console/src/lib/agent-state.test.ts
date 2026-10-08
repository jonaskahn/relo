import { describe, expect, it } from 'vitest';

import {
	byLabel,
	contextOn,
	isTested,
	logoIdOf,
	patchAgent,
	refusalDetailKey,
	refusalOf,
	restartsClient,
	splitAgents,
	stateCode,
	stateKey,
	stateTone,
	warningsOf,
	warningKeys
} from './agent-state';
import type { IntegrationView } from './types';

function agentOf(patch: Partial<IntegrationView> = {}): IntegrationView {
	return {
		id: 'codex',
		client: 'codex',
		summary: 'Routes coding agents through Relo.',
		protocol: 'openai',
		manages_files: true,
		enabled: true,
		state: 'on',
		files: [],
		...patch
	};
}

describe('the agent status reading', () => {
	it('reads a healthy set-up agent as ready', () => {
		expect(stateCode(agentOf())).toBe('ready');
		expect(stateTone(agentOf())).toBe('pass');
	});

	it('lets a refused set-up outrank the stored state', () => {
		const agent = agentOf({
			enabled: false,
			state: 'on',
			preview: [
				{ kind: 'config', path: '/codex/config.toml', refused: true, code: 'not_installed' }
			]
		});
		expect(stateCode(agent)).toBe('notInstalled');
		expect(stateTone(agent)).toBe('muted');
		expect(refusalOf(agent)).toBe('not_installed');
	});

	it('warns about a client still holding the catalog it started with', () => {
		const agent = agentOf({ restart_needed: true });
		expect(stateCode(agent)).toBe('needsRestart');
		expect(stateTone(agent)).toBe('warn');
	});

	it('reads a drifted wiring and an errored one apart', () => {
		expect(stateCode(agentOf({ state: 'drifted' }))).toBe('drifted');
		expect(stateTone(agentOf({ state: 'drifted' }))).toBe('warn');
		expect(stateCode(agentOf({ state: 'error' }))).toBe('error');
		expect(stateTone(agentOf({ state: 'error' }))).toBe('fail');
		expect(stateCode(agentOf({ enabled: false, state: 'off' }))).toBe('off');
	});

	it('has nothing left to refuse once Relo wired the agent', () => {
		const agent = agentOf({
			preview: [{ kind: 'config', path: '/x', refused: true, code: 'not_installed' }]
		});
		expect(refusalOf(agent)).toBe('');
		expect(warningsOf(agent)).toEqual([]);
	});
});

describe('the agent card warnings', () => {
	it('reports every fact worth the operator attention, once', () => {
		const agent = agentOf({ models_stale: true, restart_needed: true });
		expect(warningsOf(agent)).toEqual(['modelsStale', 'restartNeeded']);
		expect(warningsOf(agentOf({ enabled: false }))).toEqual([]);
	});

	it('reports a refusal only before the agent is wired', () => {
		const agent = agentOf({
			enabled: false,
			preview: [{ kind: 'config', path: '/x', refused: true, code: 'relative_path' }]
		});
		expect(warningsOf(agent)).toEqual(['refused']);
	});
});

describe('the sentences behind the readings', () => {
	it('names each status reading in the catalogue', () => {
		expect(stateKey(agentOf())).toBe('ui.pages.integrationsPage.state.ready');
		expect(stateKey(agentOf({ state: 'drifted' }))).toBe('ui.pages.integrationsPage.state.drifted');
		expect(stateKey(agentOf({ restart_needed: true }))).toBe(
			'ui.pages.integrationsPage.state.needsRestart'
		);
	});

	it('names each refusal the daemon can report', () => {
		expect(refusalDetailKey('not_installed')).toBe('ui.pages.integrationsPage.notInstalledDetail');
		expect(refusalDetailKey('relative_path')).toBe('ui.pages.integrationsPage.relativePath');
		expect(refusalDetailKey('foreign_key')).toBe('ui.pages.integrationsPage.foreignKey');
		expect(refusalDetailKey('unrecognised')).toBe('ui.pages.integrationsPage.unrecognised');
		// No refusal has no sentence to name.
		expect(refusalDetailKey('')).toBe('');
	});

	it('gives the card one catalogue sentence per fact it reports', () => {
		const agent = agentOf({
			enabled: false,
			models_stale: true,
			restart_needed: true,
			preview: [{ kind: 'config', path: '/x', refused: true, code: 'unrecognised' }]
		});
		expect(warningKeys(agent)).toEqual(['ui.pages.integrationsPage.unrecognised']);
		expect(warningKeys(agentOf({ models_stale: true, restart_needed: true }))).toEqual([
			'ui.pages.integrationsPage.modelsChanged',
			'ui.pages.integrationsPage.restartNeeded'
		]);
		expect(warningKeys(agentOf())).toEqual([]);
	});
});

describe('the agent card coverage and controls', () => {
	it('marks Relo verified wiring rather than this machine', () => {
		expect(isTested(agentOf({ id: 'opencode', client: 'opencode' }))).toBe(true);
		expect(isTested(agentOf({ id: 'claude-desktop', client: 'claude-desktop' }))).toBe(true);
		expect(isTested(agentOf({ id: 'cursor', client: 'cursor' }))).toBe(false);
	});

	it('gives a coding agent its own mark, not its vendor mark', () => {
		expect(logoIdOf(agentOf({ client: 'codex' }))).toBe('codex');
		// Claude Desktop has no mark of its own and wears Anthropic's.
		expect(logoIdOf(agentOf({ client: 'claude-desktop' }))).toBe('anthropic');
		expect(logoIdOf(agentOf({ client: 'unknown-client' }))).toBe('unknown-client');
	});

	it('offers the 1M window only where the client reads it', () => {
		expect(contextOn(agentOf())).toBe(true);
		expect(contextOn(agentOf({ context_1m: false }))).toBe(false);
	});

	it('restarts only the clients that keep the environment they started with', () => {
		expect(restartsClient(agentOf({ id: 'opencode', client: 'opencode' }))).toBe(true);
		expect(restartsClient(agentOf())).toBe(true);
		expect(restartsClient(agentOf({ id: 'claude-code', client: 'claude-code' }))).toBe(false);
		expect(restartsClient(agentOf({ id: 'opencode', client: 'opencode', enabled: false }))).toBe(
			false
		);
	});
});

describe('the agent roster', () => {
	it('divides the roster into the two sections the page renders', () => {
		const roster = [
			agentOf({ id: 'codex', client: 'codex' }),
			agentOf({ id: 'cursor', client: 'cursor', enabled: false }),
			agentOf({ id: 'cline', client: 'cline', enabled: false }),
			agentOf({ id: 'pi', client: 'pi' })
		];
		const { configured, notConfigured } = splitAgents(roster);
		expect(configured.map((agent) => agent.id)).toEqual(['codex', 'pi']);
		expect(notConfigured.map((agent) => agent.id)).toEqual(['cline', 'cursor']);
	});

	it('holds a stable order, so a re-sync never reorders a card being read', () => {
		const codex = agentOf({ id: 'codex', client: 'codex' });
		const cursor = agentOf({ id: 'cursor', client: 'cursor' });
		// Claude Code sorts before Codex, and the id breaks a tie between two
		// clients the catalogue names identically.
		const claude = agentOf({ id: 'claude-code', client: 'claude-code' });
		const shared = agentOf({ id: 'shared-2', client: 'shared' });
		// The labels sort as Claude Code, Codex, Cursor, Shared.
		const order = byLabel([cursor, codex, shared, claude]).map((agent) => agent.id);
		expect(order).toEqual(['claude-code', 'codex', 'cursor', 'shared-2']);
		expect(byLabel([codex, claude, cursor, shared]).map((agent) => agent.id)).toEqual(order);
	});

	it('patches one agent in place instead of reloading the roster', () => {
		const roster = [agentOf({ id: 'codex', client: 'codex' }), agentOf({ id: 'pi', client: 'pi' })];
		const patched = patchAgent(roster, { ...roster[0], state: 'drifted' });
		expect(patched[0].state).toBe('drifted');
		expect(patched[1]).toBe(roster[1]);
		expect(patched.map((agent) => agent.id)).toEqual(['codex', 'pi']);
	});

	it('leaves the roster untouched for an agent the daemon does not list', () => {
		const roster = [agentOf({ id: 'codex', client: 'codex' })];
		expect(patchAgent(roster, agentOf({ id: 'gone', client: 'gone' }))).toEqual(roster);
	});
});
