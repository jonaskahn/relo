import { describe, expect, it } from 'vitest';

import {
	byLabel,
	contextOn,
	isTested,
	logoIdOf,
	openCodeVersionChip,
	patchAgent,
	refusalDetailKey,
	refusalOf,
	restartsClient,
	splitAgents,
	stateCode,
	stateKey,
	stateTone,
	reasonLines
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
		expect(reasonLines(agent)).toEqual([]);
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

	it('keeps no warning once Relo wired the agent', () => {
		expect(reasonLines(agentOf({ reasons: [] }))).toEqual([]);
		expect(reasonLines(agentOf({ enabled: false }))).toEqual([]);
	});
});

describe('the reasons the daemon reports', () => {
	it('names each reason in the catalogue with its own detail', () => {
		expect(
			reasonLines(
				agentOf({ reasons: [{ code: 'file_changed', path: '/home/op/.codex/relo-key.sh' }] })
			)
		).toEqual([
			{
				key: 'ui.pages.integrationsPage.reason.fileChanged',
				values: { file: 'relo-key.sh' },
				path: '/home/op/.codex/relo-key.sh'
			}
		]);
		expect(
			reasonLines(
				agentOf({ reasons: [{ code: 'file_missing', path: '/home/op/.codex/relo-key.sh' }] })
			)
		).toEqual([
			{
				key: 'ui.pages.integrationsPage.reason.fileMissing',
				values: { file: 'relo-key.sh' },
				path: '/home/op/.codex/relo-key.sh'
			}
		]);
		expect(reasonLines(agentOf({ reasons: [{ code: 'restart_catalog_newer' }] }))).toEqual([
			{ key: 'ui.pages.integrationsPage.reason.restartCatalogNewer', values: {} }
		]);
	});

	it('shows the model counts when the daemon measured them', () => {
		expect(
			reasonLines(agentOf({ reasons: [{ code: 'models_changed', added: 3, removed: 1 }] }))
		).toEqual([
			{
				key: 'ui.pages.integrationsPage.reason.modelsChangedAdded',
				values: { count: 3 }
			},
			{
				key: 'ui.pages.integrationsPage.reason.modelsChangedRemoved',
				values: { count: 1 }
			}
		]);
		expect(reasonLines(agentOf({ reasons: [{ code: 'models_changed' }] }))).toEqual([
			{ key: 'ui.pages.integrationsPage.reason.modelsChangedBare', values: {} }
		]);
		expect(
			reasonLines(agentOf({ reasons: [{ code: 'models_changed', added: 2, removed: 0 }] })).map(
				(line) => line.key
			)
		).toEqual(['ui.pages.integrationsPage.reason.modelsChangedAdded']);
	});

	it('keeps the order the daemon sent and drops a code it does not know', () => {
		const agent = agentOf({
			reasons: [
				{ code: 'models_changed', added: 1, removed: 0 },
				{ code: 'file_changed', path: '/x/y' },
				{ code: 'something_new' }
			]
		});
		expect(reasonLines(agent).map((line) => line.key)).toEqual([
			'ui.pages.integrationsPage.reason.modelsChangedAdded',
			'ui.pages.integrationsPage.reason.fileChanged'
		]);
	});

	it('reports a refusal, never a reason, before the agent is wired', () => {
		const agent = agentOf({
			enabled: false,
			reasons: [{ code: 'file_changed', path: '/x/y' }],
			preview: [{ kind: 'config', path: '/x', refused: true, code: 'relative_path' }]
		});
		expect(reasonLines(agent)).toEqual([
			{ key: 'ui.pages.integrationsPage.relativePath', values: {} }
		]);
	});
});

describe('the agent card coverage and controls', () => {
	it('marks Relo verified wiring rather than this machine', () => {
		expect(isTested(agentOf({ id: 'opencode', client: 'opencode' }))).toBe(true);
		expect(isTested(agentOf({ id: 'claude-desktop', client: 'claude-desktop' }))).toBe(true);
		expect(isTested(agentOf({ id: 'cursor', client: 'cursor' }))).toBe(false);
		expect(isTested(agentOf({ id: 'dsh', client: 'dsh' }))).toBe(true);
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

describe('the OpenCode generations chip', () => {
	it('shows nothing for another agent or an empty reading', () => {
		expect(openCodeVersionChip(agentOf({ id: 'codex', openCodeVersions: 'v1+v2' }))).toBe('');
		expect(openCodeVersionChip(agentOf({ id: 'opencode' }))).toBe('');
		expect(openCodeVersionChip(agentOf({ id: 'opencode', openCodeVersions: 'v3' }))).toBe('');
	});

	it('names the generations the daemon read in the file', () => {
		expect(openCodeVersionChip(agentOf({ id: 'opencode', openCodeVersions: 'v1' }))).toBe('V1');
		expect(openCodeVersionChip(agentOf({ id: 'opencode', openCodeVersions: 'v2' }))).toBe('V2');
		expect(openCodeVersionChip(agentOf({ id: 'opencode', openCodeVersions: 'v1+v2' }))).toBe(
			'V1+V2'
		);
	});
});
