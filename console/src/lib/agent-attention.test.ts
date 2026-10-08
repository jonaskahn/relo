import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { AGENT_CATALOG_CHANGED_EVENT } from './api';
import { agentAttention } from './agent-attention.svelte';
import type { IntegrationView } from './types';

// The strip that reports what an agent setup still needs reads the same roster
// list the page does, so it is checked on what it counts rather than on how it
// draws.

function agent(id: string, overrides: Partial<IntegrationView> = {}): IntegrationView {
	return {
		id,
		client: id,
		summary: '',
		protocol: 'openai',
		manages_files: true,
		enabled: true,
		state: 'on',
		files: [],
		...overrides
	} satisfies IntegrationView;
}

const listeners = new Map<string, EventListener>();

beforeEach(() => {
	listeners.clear();
	vi.stubGlobal('window', {
		addEventListener: (name: string, listener: EventListener) => listeners.set(name, listener),
		removeEventListener: (name: string) => listeners.delete(name)
	});
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

describe('agent attention', () => {
	it('counts the roster entries that need repair or a restart', () => {
		agentAttention.set([
			agent('codex'),
			agent('claude-code', { state: 'drifted' }),
			agent('other', { restart_needed: true }),
			agent('off', { enabled: false, restart_needed: true })
		]);
		expect(agentAttention.count).toBe(2);
	});

	it('reads the roster again when the catalog changes', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					json: () => Promise.resolve({ items: [agent('codex', { models_stale: true })] })
				})
			)
		);
		await agentAttention.refresh();
		expect(agentAttention.count).toBe(1);
	});

	it('leaves the count empty when the read fails, rather than keeping a stale one', async () => {
		agentAttention.set([agent('codex', { state: 'drifted' })]);
		vi.stubGlobal(
			'fetch',
			vi.fn(() => Promise.reject(new Error('offline')))
		);
		await agentAttention.refresh();
		expect(agentAttention.count).toBe(0);
	});

	it('drops an answer a later read has already replaced', async () => {
		const pending: Array<(value: unknown) => void> = [];
		vi.stubGlobal(
			'fetch',
			vi.fn(
				() =>
					new Promise((resolve) => {
						pending.push(resolve);
					})
			)
		);

		const first = agentAttention.refresh();
		const second = agentAttention.refresh();
		// The newer read answers first and the older one lands on a roster the
		// strip has already moved past.
		pending[1]({ ok: true, status: 200, json: () => Promise.resolve({ items: [agent('codex')] }) });
		await second;
		pending[0]({
			ok: true,
			status: 200,
			json: () => Promise.resolve({ items: [agent('codex', { state: 'drifted' })] })
		});
		await first;

		expect(agentAttention.count).toBe(0);
	});

	it('follows the catalog-changed event while a page has it open', () => {
		const stop = agentAttention.watch();
		expect(listeners.has(AGENT_CATALOG_CHANGED_EVENT)).toBe(true);
		stop();
		expect(listeners.has(AGENT_CATALOG_CHANGED_EVENT)).toBe(false);
	});
});
