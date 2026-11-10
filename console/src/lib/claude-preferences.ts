// Personal Claude Code choices: which model it opens on, and the effort and
// auto-compact window saved for that model. The settings file stays the source
// of truth; this only shapes what the manage modal sends and shows.

import type { ClaudeModelChoice, ClaudeModelConfigUpdate } from './types';

export const CLAUDE_COMPACT_MIN = 100_000;
export const CLAUDE_COMPACT_MAX = 1_000_000;

const contextSuffix = '[1m]';

// Aliases Claude Code documents for CLAUDE_CODE_SUBAGENT_MODEL. default is not
// an alias, and inherit belongs to an agent definition, which still wins.
export const claudeSubagentAliases = [
	{ value: 'best', label: 'ui.pages.integrationsPage.modelPreferencesSubagentBest' },
	{ value: 'fable', label: 'ui.pages.integrationsPage.modelPreferencesSubagentFable' },
	{ value: 'sonnet', label: 'ui.pages.integrationsPage.modelPreferencesSubagentSonnet' },
	{ value: 'opus', label: 'ui.pages.integrationsPage.modelPreferencesSubagentOpus' },
	{ value: 'haiku', label: 'ui.pages.integrationsPage.modelPreferencesSubagentHaiku' },
	{ value: 'sonnet[1m]', label: 'ui.pages.integrationsPage.modelPreferencesSubagentSonnet1m' },
	{ value: 'opus[1m]', label: 'ui.pages.integrationsPage.modelPreferencesSubagentOpus1m' },
	{ value: 'opusplan', label: 'ui.pages.integrationsPage.modelPreferencesSubagentOpusplan' }
] as const;

export interface ClaudePreferenceDraft {
	model: string;
	subagentModel: string;
	effort: string;
	compact: 'auto' | 'custom';
	tokens: string;
}

export interface ClaudeListedModel {
	readonly id: string;
	readonly name?: string;
	readonly efforts?: readonly string[];
}

/** The effort levels Claude Code can store for a model. Opus 4.6 and Sonnet 4.6
 *  stop at high. max is a session choice, so it is never offered. */
export function claudeEffortLevels(model: string): string[] {
	const bare = stripContext(model).toLowerCase();
	if (bare.includes('opus-4-6') || bare.includes('sonnet-4-6')) {
		return ['low', 'medium', 'high'];
	}
	return ['low', 'medium', 'high', 'xhigh'];
}

/** Models the select can offer. A ready test-pane listing wins; each saved model
 *  stays visible when that listing no longer contains it. */
export function claudePreferenceModels(
	savedModels: readonly string[],
	pane: readonly { id: string; name?: string }[],
	paneReady: boolean,
	catalog: readonly ClaudeModelChoice[]
): ClaudeListedModel[] {
	const listed =
		paneReady && pane.length > 0
			? pane.map((model) => ({
					id: model.id,
					name: model.name,
					efforts: catalog.find((choice) => choice.id === model.id)?.efforts
				}))
			: catalog.map((model) => ({
					id: model.id,
					name: model.name,
					efforts: model.efforts
				}));
	const missing: ClaudeListedModel[] = [];
	for (const saved of savedModels) {
		const id = saved.trim();
		if (
			id === '' ||
			listed.some((model) => model.id === id) ||
			missing.some((model) => model.id === id)
		) {
			continue;
		}
		missing.push({ id, efforts: claudeEffortLevels(id) });
	}
	return missing.length === 0 ? listed : [...missing, ...listed];
}

/** Effort values for the selected model, keeping a saved value the ladder no
 *  longer offers so the control does not silently replace it. */
export function claudeEffortChoices(
	modelId: string,
	savedEffort: string,
	known?: readonly string[]
): string[] {
	const levels = known && known.length > 0 ? [...known] : claudeEffortLevels(modelId);
	const saved = savedEffort.trim();
	if (saved !== '' && saved !== 'auto' && !levels.includes(saved)) {
		return [saved, ...levels];
	}
	return levels;
}

/** The request body for one save, or the field that is not ready to send. */
export function claudePreferencePayload(
	draft: ClaudePreferenceDraft
): { ok: true; body: ClaudeModelConfigUpdate } | { ok: false; error: 'model' | 'compact' } {
	const model = draft.model === 'auto' ? '' : draft.model.trim();
	const subagentModel = draft.subagentModel === 'auto' ? '' : draft.subagentModel.trim();
	const effort = model === '' || draft.effort === 'auto' ? '' : draft.effort.trim();
	if (draft.compact !== 'custom') {
		return {
			ok: true,
			body: { model, subagent_model: subagentModel, effort, auto_compact: 'auto' }
		};
	}
	if (model === '') return { ok: false, error: 'model' };
	const tokens = draft.tokens.trim();
	if (!/^[0-9]+$/.test(tokens)) return { ok: false, error: 'compact' };
	const window = Number(tokens);
	if (!Number.isSafeInteger(window) || window < CLAUDE_COMPACT_MIN || window > CLAUDE_COMPACT_MAX) {
		return { ok: false, error: 'compact' };
	}
	return {
		ok: true,
		body: { model, subagent_model: subagentModel, effort, auto_compact: String(window) }
	};
}

function stripContext(model: string): string {
	const trimmed = model.trim();
	return trimmed.endsWith(contextSuffix) ? trimmed.slice(0, -contextSuffix.length) : trimmed;
}
