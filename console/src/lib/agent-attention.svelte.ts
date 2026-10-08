import { AGENT_CATALOG_CHANGED_EVENT, api } from './api';
import { needsAttention } from './integration-actions';
import type { IntegrationView } from './types';

function createAgentAttention() {
	let count = $state(0);
	let refreshRevision = 0;

	function set(agents: IntegrationView[]) {
		count = agents.filter(needsAttention).length;
	}

	async function refresh() {
		const revision = ++refreshRevision;
		try {
			const data = await api<{ items: IntegrationView[] }>('/integrations');
			if (revision !== refreshRevision) return;
			set(data.items ?? []);
		} catch {
			if (revision !== refreshRevision) return;
			count = 0;
		}
	}

	function watch() {
		const update = () => void refresh();
		window.addEventListener(AGENT_CATALOG_CHANGED_EVENT, update);
		return () => window.removeEventListener(AGENT_CATALOG_CHANGED_EVENT, update);
	}

	return {
		get count() {
			return count;
		},
		set,
		refresh,
		watch
	};
}

/** The strip that reports what an agent setup still needs. */
export const agentAttention = createAgentAttention();
