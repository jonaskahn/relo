import { describe, expect, it } from 'vitest';

import {
	addStepBack,
	addStepNext,
	blocked,
	overwrittenFiles,
	resultCodeFor,
	revealNeedsToken,
	runningOn,
	stepAfter,
	type AgentAction,
	type AddStep
} from './agent-flow';
import type { IntegrationResult, IntegrationView } from './types';

function view(patch: Partial<IntegrationView> = {}): IntegrationView {
	return {
		id: 'codex',
		client: 'codex',
		summary: '',
		protocol: 'openai',
		manages_files: true,
		enabled: true,
		state: 'on',
		files: [],
		...patch
	};
}

function resultOf(patch: Partial<IntegrationResult> = {}): IntegrationResult {
	return { integration: view(), ...patch };
}

describe('where an action leaves the operator', () => {
	it('hands a manual set-up the key its own steps name, shown once', () => {
		const result = resultOf({ token: 'sk-relo-abc', integration: view({ manages_files: false }) });
		expect(revealNeedsToken(result, 'setup')).toBe(true);
		expect(stepAfter(result, 'setup')).toBe('reveal');
	});

	it('reports a set-up Relo writes itself as a plain result', () => {
		expect(stepAfter(resultOf(), 'setup')).toBe('result');
		expect(revealNeedsToken(resultOf({ token: 'sk-relo-abc' }), 'setup')).toBe(false);
	});

	it('shows the key a rotation minted, in the same step', () => {
		const result = resultOf({ token: 'sk-relo-new' });
		expect(stepAfter(result, 'rotate')).toBe('reveal');
	});

	it('never reveals a key the daemon did not hand over', () => {
		expect(stepAfter(resultOf(), 'rotate')).toBe('result');
		expect(revealNeedsToken(resultOf({ token: '' }), 'setup')).toBe(false);
	});

	it('reports a repair and a removal in the view, not a reveal', () => {
		expect(stepAfter(resultOf({ token: 'sk-relo-abc' }), 'repair')).toBe('result');
		expect(stepAfter(resultOf(), 'remove')).toBe('result');
	});
});

describe('the result a step reports', () => {
	it('names what each action did', () => {
		expect(resultCodeFor('setup', view({ manages_files: false }))).toBe('saved');
		expect(resultCodeFor('setup', view())).toBe('setupSuccessAuto');
		expect(resultCodeFor('rotate', view())).toBe('rotated');
		expect(resultCodeFor('repair', view())).toBe('repaired');
		expect(resultCodeFor('remove', view())).toBe('removed');
		expect(resultCodeFor('restore', view())).toBe('restored');
	});

	it('separates the two restart words by the client that keeps its environment', () => {
		expect(resultCodeFor('restart', view({ id: 'opencode', client: 'opencode' }))).toBe(
			'restartedOpenCode'
		);
		expect(resultCodeFor('restart', view())).toBe('restarted');
	});

	it('separates the two repair words by the client that still needs a restart', () => {
		expect(resultCodeFor('repair', view({ restart_needed: true }))).toBe('repairedCodex');
		expect(resultCodeFor('repair', view())).toBe('repaired');
	});
});

describe('one action in flight per agent', () => {
	const running: { id: string; action: AgentAction } = { id: 'codex', action: 'setup' };

	it('gives the spinner to the control doing the work', () => {
		expect(runningOn(running, view(), 'setup')).toBe(true);
		expect(runningOn(running, view(), 'repair')).toBe(false);
		expect(runningOn(running, view({ id: 'pi', client: 'pi' }), 'setup')).toBe(false);
		expect(runningOn(null, view(), 'setup')).toBe(false);
	});

	it('holds only its own agent, so one set-up never freezes the page', () => {
		expect(blocked(running, view())).toBe(true);
		expect(blocked(running, view({ id: 'pi', client: 'pi' }))).toBe(false);
		expect(blocked(null, view())).toBe(false);
	});
});

describe('what a removal undoes', () => {
	it('names the files a snapshot will restore over', () => {
		const agent = view({
			files: [
				{ kind: 'config', path: '/a', present: true, drifted: true },
				{ kind: 'key', path: '/b', present: true, drifted: false },
				{ kind: 'cache', path: '/c', present: false, drifted: false }
			]
		});
		expect(overwrittenFiles(agent)).toEqual(['/a']);
	});

	it('names nothing when no file drifted', () => {
		expect(
			overwrittenFiles(
				view({ files: [{ kind: 'key', path: '/b', present: true, drifted: false }] })
			)
		).toEqual([]);
		expect(overwrittenFiles(view())).toEqual([]);
	});
});
describe('the add-agent walk', () => {
	it('walks choose, setup, verify and result in order', () => {
		const steps: AddStep[] = ['choose', 'setup', 'verify', 'result'];
		for (let index = 0; index < steps.length - 1; index += 1) {
			expect(addStepNext(steps[index])).toBe(steps[index + 1]);
		}
		expect(addStepNext('result')).toBeNull();
	});

	it('returns from setup and verify, and the result only closes', () => {
		expect(addStepBack('setup')).toBe('choose');
		expect(addStepBack('verify')).toBe('setup');
		expect(addStepBack('choose')).toBeNull();
		expect(addStepBack('result')).toBeNull();
	});
});
