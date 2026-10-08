// The detail modal is one modal for one agent, and its content swaps per step.
// This module is the step machine: which step an action lands on, what each step
// is titled and says, and what a result step needs to carry. Keeping it pure
// means the flow's rules are testable without a modal, and the page holds no
// branch that decides what the operator reads next.

import type { IntegrationResult, IntegrationView } from './types';

/** The modal's content. There is no second modal: setup, rotation and their results are steps of
 *  this one. */
export type AgentStep = 'view' | 'result' | 'reveal';

/** The work an operator asks of one agent from this page. */
export type AgentAction = 'setup' | 'remove' | 'restore' | 'rotate' | 'repair' | 'restart';

/** The irreversible work that asks before it happens.
 *  Each one closes the modal, opens its confirmation, and returns to a step. */
export type ConfirmAction = 'remove' | 'restore' | 'rotate' | 'overwrite';

/** Reports whether a result step is really the one-time reveal: a manual client is handed the
 *  key its own setup steps name, and a rotation is handed the key it just minted.
 *  A client Relo configures itself has no secret to show, so it gets the plain result. */
export function revealNeedsToken(result: IntegrationResult, action: AgentAction): boolean {
	if (!result.token) return false;
	if (action === 'rotate') return true;
	return action === 'setup' && !result.integration.manages_files;
}

/** Names where an action leaves the operator. A set-up and a repair report their result in the
 *  modal they were started from; a rotation reports on the step that shows the key it minted. */
export function stepAfter(result: IntegrationResult, action: AgentAction): AgentStep {
	return revealNeedsToken(result, action) ? 'reveal' : 'result';
}

/** The reading a result step carries, so success and failure render from one vocabulary rather
 *  than a branch each. */
export type ResultCode =
	| 'setupSuccessAuto'
	| 'setupFailed'
	| 'repaired'
	| 'repairedCodex'
	| 'rotated'
	| 'restarted'
	| 'restartedOpenCode'
	| 'removed'
	| 'restored'
	| 'saved';

/** Reads the outcome one action reports for one agent. */
export function resultCodeFor(action: AgentAction, agent: IntegrationView): ResultCode {
	if (action === 'setup') return agent.manages_files ? 'setupSuccessAuto' : 'saved';
	if (action === 'rotate') return 'rotated';
	if (action === 'restart') return agent.id === 'opencode' ? 'restartedOpenCode' : 'restarted';
	if (action === 'repair')
		return agent.id === 'codex' && agent.restart_needed ? 'repairedCodex' : 'repaired';
	return action === 'remove' ? 'removed' : 'restored';
}

/** What a control bar reports a spinner for: the actions that change an agent's wiring, its
 *  context window, and the verification the test pane runs. */
export type Work = AgentAction | 'verify' | 'context';

/** The one piece of work in flight, named by the agent it belongs to. */
export type Running = { id: string; action: Work } | null;

/** Reports whether the named work is the one in flight for an agent, which is the control that
 *  carries the spinner while every other control stays live. One agent's action never freezes
 *  the page. */
export function runningOn(running: Running, agent: IntegrationView, action: Work): boolean {
	return running?.id === agent.id && running.action === action;
}

/** Reports whether a control is waiting on the one action already in flight for this agent.
 *  Other agents keep their controls. */
export function blocked(running: Running, agent: IntegrationView): boolean {
	return running?.id === agent.id;
}

/** The add-agent modal's content: pick an agent, set it up, watch verification prove the wiring,
 *  read the result. */
export type AddStep = 'choose' | 'setup' | 'verify' | 'result';

/** Lists the add flow in the order the stepper shows it. */
export const ADD_STEPS: readonly AddStep[] = ['choose', 'setup', 'verify', 'result'];

/** Names the step finishing one advances to. Choosing needs a selected agent, which the modal
 *  holds rather than this machine. */
export function addStepNext(step: AddStep): AddStep | null {
	switch (step) {
		case 'choose':
			return 'setup';
		case 'setup':
			return 'verify';
		case 'verify':
			return 'result';
		default:
			return null;
	}
}

/** Names the step the back control returns to. Verification runs itself, so it returns to the
 *  setup that started it; the result only closes. */
export function addStepBack(step: AddStep): AddStep | null {
	switch (step) {
		case 'setup':
			return 'choose';
		case 'verify':
			return 'setup';
		default:
			return null;
	}
}

/** Names the files a removal will restore from the snapshot rather than leaving as they are,
 *  which are the ones an editor changed after Relo wrote them.
 *  Naming them is what makes the confirmation honest about what a removal undoes. */
export function overwrittenFiles(agent: IntegrationView): string[] {
	return (agent.files ?? [])
		.filter((file) => file.present && file.drifted)
		.map((file) => file.path);
}
