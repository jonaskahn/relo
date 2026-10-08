// What one agent card and one detail modal say about an agent. These are the
// readings every surface on the page shares: the tone behind a status pill, the
// copy key behind a warning, and the order the two sections hold. Keeping them
// pure means the card, the modal and the tests read one implementation of "what
// is true here", and none of them re-decides it from a raw field.

import { CLIENT_OPTIONS } from './client-setup';
import { needsRepair, refusalCode, type SetupRefusal } from './integration-actions';
import type { IntegrationView } from './types';

/** The pill tone. It maps onto the shared status tones: pass, warn, fail, muted. */
export type AgentTone = 'pass' | 'warn' | 'fail' | 'muted';

/** The reading behind a status pill, named so the page can render it from one catalog rather
 *  than holding an English sentence in a branch. */
export type StateCode =
	| 'ready'
	| 'drifted'
	| 'error'
	| 'off'
	| 'notInstalled'
	| 'pathRefused'
	| 'cannotEdit'
	| 'needsRestart';

/** The reading behind one agent's status pill. */
export function stateCode(agent: IntegrationView): StateCode {
	if (!agent.enabled) {
		switch (refusalOf(agent)) {
			case 'not_installed':
				return 'notInstalled';
			case 'relative_path':
				return 'pathRefused';
			case 'unrecognised':
				return 'cannotEdit';
		}
	}
	// A client still holding the catalog it started with is the one warning that
	// outranks a healthy wiring: the files match, the models do not.
	if (agent.enabled && agent.restart_needed) return 'needsRestart';
	switch (agent.state) {
		case 'on':
			return 'ready';
		case 'drifted':
			return 'drifted';
		case 'error':
			return 'error';
		default:
			return 'off';
	}
}

/** Maps a reading onto the shared pill tones. */
export function stateTone(agent: IntegrationView): AgentTone {
	switch (stateCode(agent)) {
		case 'ready':
			return 'pass';
		case 'drifted':
		case 'needsRestart':
			return 'warn';
		case 'error':
			return 'fail';
		default:
			return 'muted';
	}
}

/** Why a set-up would not write yet. An agent Relo already wired has nothing left to refuse. */
export function refusalOf(agent: IntegrationView): SetupRefusal {
	return agent.enabled ? '' : refusalCode(agent.preview);
}

/** The facts a card reports under its key row, each already a decision about what is worth the
 *  operator's attention. */
export type WarningCode = 'refused' | 'modelsStale' | 'restartNeeded';

/** Every fact a card should warn about, in the order they show. */
export function warningsOf(agent: IntegrationView): WarningCode[] {
	const codes: WarningCode[] = [];
	if (!agent.enabled && refusalOf(agent)) codes.push('refused');
	if (agent.enabled && agent.models_stale) codes.push('modelsStale');
	if (agent.enabled && agent.restart_needed) codes.push('restartNeeded');
	return codes;
}

/** The catalog key behind a status reading, so a surface renders the sentence from one catalogue
 *  rather than from an English branch. */
export function stateKey(agent: IntegrationView): string {
	return 'ui.pages.integrationsPage.state.' + stateCode(agent);
}

// REFUSAL_DETAILS is the sentence behind each refusal code, keyed by the code the
// daemon reports.
const REFUSAL_DETAILS: Record<string, string> = {
	not_installed: 'notInstalledDetail',
	relative_path: 'relativePath',
	foreign_key: 'foreignKey',
	unrecognised: 'unrecognised'
};

/** The copy key that says why a setup was refused. */
export function refusalDetailKey(code: SetupRefusal): string {
	const name = REFUSAL_DETAILS[code];
	return name ? 'ui.pages.integrationsPage.' + name : '';
}

/** The sentences the card reports under its key row, in the order the card lists them. */
export function warningKeys(agent: IntegrationView): string[] {
	const refusal = refusalOf(agent);
	const keys: string[] = [];
	if (!agent.enabled && refusal) {
		const detail = refusalDetailKey(refusal);
		if (detail) keys.push(detail);
	}
	if (agent.enabled && agent.models_stale) keys.push('ui.pages.integrationsPage.modelsChanged');
	if (agent.enabled && agent.restart_needed) keys.push('ui.pages.integrationsPage.restartNeeded');
	return keys;
}

// TESTED_IDS names the clients Relo has verified wiring for. The pill marks that
// coverage, not whether this machine has set the client up.
const TESTED_IDS = new Set(['opencode', 'claude-code', 'claude-desktop', 'codex', 'pi', 'omp']);

/** Reports whether Relo has verified its wiring for that client. */
export function isTested(agent: IntegrationView): boolean {
	return TESTED_IDS.has(agent.id);
}

/** The 1M window Relo writes into a set-up agent's configuration. */
export function contextOn(agent: IntegrationView): boolean {
	return agent.context_1m !== false;
}

/** Reports the clients that keep the environment they started with.
 *  OpenCode's background server and Codex's app-server both read the catalog once, so the card
 *  can restart them once Relo holds a key. */
export function restartsClient(agent: IntegrationView): boolean {
	return agent.enabled && (agent.id === 'opencode' || agent.id === 'codex');
}

/** The console's name for an agent's client, falling back to the client's own id for one the
 *  catalogue does not carry. */
export function labelOf(agent: IntegrationView): string {
	return CLIENT_OPTIONS.find((option) => option.id === agent.client)?.label ?? agent.client;
}

/** Names the mark to draw: the client's own where one is mirrored, never its vendor's. */
export function logoIdOf(agent: IntegrationView): string {
	return CLIENT_OPTIONS.find((option) => option.id === agent.client)?.logoId ?? agent.client;
}

/** The order both sections hold. A background re-sync replaces entries in place, so a stable
 *  order is what keeps a card an operator is reading from moving underneath them. */
export function byLabel(agents: IntegrationView[]): IntegrationView[] {
	return [...agents].sort(
		(a, b) => labelOf(a).localeCompare(labelOf(b)) || a.id.localeCompare(b.id)
	);
}

/** Divides the roster into the two sections the page renders. Both hold the stable order. */
export function splitAgents(agents: IntegrationView[]): {
	configured: IntegrationView[];
	notConfigured: IntegrationView[];
} {
	return {
		configured: byLabel(agents.filter((agent) => agent.enabled)),
		notConfigured: byLabel(agents.filter((agent) => !agent.enabled))
	};
}

/** Folds one action's answer back into the roster in place, so an action never reloads the page
 *  underneath the operator.
 *  An agent the roster does not carry is dropped: the page only ever renders what the daemon
 *  listed. */
export function patchAgent(agents: IntegrationView[], agent: IntegrationView): IntegrationView[] {
	return agents.map((entry) => (entry.id === agent.id ? agent : entry));
}

/** Re-exported so a card reads the repair check beside the readings it is judged on, rather than
 *  reaching into the actions module for one predicate. */
export { needsRepair };
