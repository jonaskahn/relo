// The Integrations page drives one agent through the routes the daemon serves.
// Two of the words differ: an operator "sets up" what the daemon calls enable,
// and "removes" what it calls disable. The mapping lives here, on its own, so
// a test can hold every action to a route the daemon actually mounts.

import type { IntegrationView } from './types';

/** Names one thing the page does to an agent, and the two reads the test side of the detail
 *  modal makes: the models that agent's own key reaches, and one turn sent through it. */
export type IntegrationAction =
	| 'setup'
	| 'remove'
	| 'verify'
	| 'repair'
	| 'restart'
	| 'restore'
	| 'rotate'
	| 'models'
	| 'chat'
	| 'context'
	| 'token';

// integrationEndpoints is the daemon route each page action calls. Every value
// has to be a route the management API mounts under /integrations/{id}/.
const integrationEndpoints: Record<IntegrationAction, string> = {
	setup: 'enable',
	remove: 'disable',
	verify: 'verify',
	repair: 'repair',
	restart: 'restart',
	restore: 'restore',
	rotate: 'rotate',
	models: 'models',
	chat: 'chat',
	context: 'context',
	token: 'token'
};

/** Names the route one page action calls. */
export function integrationEndpoint(action: IntegrationAction): string {
	return integrationEndpoints[action];
}

/** The path one page action posts to. The identifier is encoded, so an agent whose name carries
 *  a slash still reaches its route. */
export function integrationPath(id: string, action: IntegrationAction): string {
	return '/integrations/' + encodeURIComponent(id) + '/' + integrationEndpoint(action);
}

/** Why a set-up would not write yet. An empty code means the plan can be written. */
export type SetupRefusal = '' | 'not_installed' | 'foreign_key' | 'relative_path' | 'unrecognised';

const setupRefusals = new Set<string>([
	'not_installed',
	'foreign_key',
	'relative_path',
	'unrecognised'
]);

/** Reports whether a refusal the daemon named is one the console still renders. */
export function isSetupRefusal(code: string): code is SetupRefusal {
	return setupRefusals.has(code);
}

/** The first reason a preview gives for not writing.
 *  A foreign provider can still be replaced; the other reasons stop the set-up. */
export function refusalCode(
	preview: { refused?: boolean; code?: string }[] | undefined
): SetupRefusal {
	for (const plan of preview ?? []) {
		if (plan.refused && plan.code && isSetupRefusal(plan.code)) {
			return plan.code;
		}
	}
	return '';
}

/** Reports a preview Relo will not write even if the operator confirms. */
export function setupBlocked(code: SetupRefusal): boolean {
	return code === 'not_installed' || code === 'relative_path' || code === 'unrecognised';
}

/** Reports when Relo's managed files or model catalog no longer match what it last wrote for a
 *  set-up agent. */
export function needsRepair(agent: IntegrationView): boolean {
	if (!agent.enabled) return false;
	if (agent.models_stale || agent.state === 'drifted') return true;
	return agent.files.some((file) => file.drifted || !file.present);
}

/** Powers notices outside the Agents page, where repair and restart are one actionable warning. */
export function needsAttention(agent: IntegrationView): boolean {
	return agent.enabled && (needsRepair(agent) || Boolean(agent.restart_needed));
}

/** Names the file Relo keeps the secret in, from a write that already happened or from the
 *  preview of one. */
export function keyPathOf(agent: {
	files?: { kind: string; path: string }[];
	preview?: { kind: string; path: string }[];
}): string {
	return (
		agent.files?.find((file) => file.kind === 'key')?.path ??
		agent.preview?.find((plan) => plan.kind === 'key')?.path ??
		''
	);
}
