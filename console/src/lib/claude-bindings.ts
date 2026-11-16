import type {
	ClaudeBinding,
	ClaudeBindingEntry,
	ClaudeBindingTarget,
	ClaudeOfficialModel
} from './types';

/** The value a target selector carries, stable for any id a connection or route may hold. */
export function targetValue(target: ClaudeBindingTarget): string {
	return JSON.stringify([
		target.kind,
		target.group_id ?? '',
		target.provider_id ?? '',
		target.model_id ?? ''
	]);
}

/** Reads a selector value back into a target, or null for one that names nothing. */
export function targetFromValue(value: string): ClaudeBindingTarget | null {
	let parts: unknown;
	try {
		parts = JSON.parse(value);
	} catch {
		return null;
	}
	if (!Array.isArray(parts) || parts.length !== 4) return null;
	const [kind, groupId, providerId, modelId] = parts as string[];
	if (kind === 'route' && groupId) return { kind, group_id: groupId };
	if (kind === 'model' && providerId && modelId) {
		return { kind, provider_id: providerId, model_id: modelId };
	}
	return null;
}

/** The official ids that have no binding yet, in the order the daemon listed them. */
export function unboundOfficial(
	official: readonly ClaudeOfficialModel[],
	bindings: readonly ClaudeBindingEntry[]
): ClaudeOfficialModel[] {
	const bound = new Set(bindings.map((binding) => binding.native_id));
	return official.filter((model) => !bound.has(model.id));
}

/** The binding body for one row, without the state the daemon computes. */
export function bindingBody(row: ClaudeBinding): ClaudeBinding {
	return {
		native_id: row.native_id,
		target: row.target,
		direction: row.direction,
		// A fallback binding asks Anthropic first, so what Relo does on failure is not a choice.
		on_failure: row.direction === 'fallback' ? 'none' : row.on_failure,
		enabled: row.enabled
	};
}

/** A new binding for one official id: Relo answers, and a refusal is shown rather than spent. */
export function newBinding(nativeId: string, target: ClaudeBindingTarget): ClaudeBinding {
	return { native_id: nativeId, target, direction: 'replace', on_failure: 'none', enabled: true };
}
