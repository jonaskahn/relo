// What the catalog pages show and in what order: the providers Relo can route
// to, the models they serve, and the groups an operator defined. The pages
// render the result; nothing here touches the DOM.

import type { Group, GroupMember, Model, Provider } from './types';

/** Names every ordering an operator may choose for a group. */
export const STRATEGIES = ['priority', 'round-robin', 'weighted', 'cheapest', 'fastest'] as const;

/** The capability classes a model may be reclassified into.
 *  The list stays open, so a class the catalog introduces needs no migration. */
export const CATEGORIES = [
	'chat',
	'reasoning',
	'vision',
	'image',
	'audio',
	'video',
	'embedding'
] as const;

/** The protocols an operator may pick when adding a provider.
 *  The list mirrors provider.APIFormats() on the daemon. */
export const API_FORMATS = [
	'openai-chat',
	'openai-responses',
	'anthropic',
	'vertex-anthropic',
	'gemini',
	'vertex',
	'bedrock-converse',
	'kiro'
] as const;

/** The listing dialects an operator may pick. The list mirrors provider.ModelsFormats() on the
 *  daemon. */
export const MODELS_FORMATS = ['none', 'openai', 'anthropic', 'gemini', 'bedrock', 'kiro'] as const;

/** Encodes one provider model as the management API spells it, with every segment of the
 *  identifier escaped so a slash inside a model id stays a separator on the wire. */
export function modelPath(providerID: string, modelID: string): string {
	const segments = modelID.split('/').map(encodeURIComponent);
	return '/models/' + encodeURIComponent(providerID) + '/' + segments.join('/');
}

/** Renders one rate as dollars per million tokens, and an unknown rate as the placeholder a
 *  table shows for "not stated". */
export function formatPrice(micros: number | null, unknown = '—'): string {
	if (micros === null || micros === undefined) return unknown;
	const dollars = micros / 1_000_000;
	if (dollars === 0) return '$0';
	if (dollars < 0.01) return '$' + dollars.toFixed(4);
	if (dollars < 1) return '$' + dollars.toFixed(3);
	return '$' + dollars.toFixed(2);
}

/** States what a model costs per million tokens in both directions as one reading, which is the
 *  pair a route surface shows beside a member. A member the console holds no model for states
 *  nothing rather than a price it cannot stand by. */
export function modelPrice(model: Model | undefined): string {
	if (!model) return '';
	return (
		formatPrice(model.prices.effective.input) + ' / ' + formatPrice(model.prices.effective.output)
	);
}

/** Names a connection the way a catalog surface shows it: its label, or its identifier when the
 *  connection is not among the ones handed in. */
export function providerLabel(providers: readonly Provider[], providerID: string): string {
	return providers.find((provider) => provider.id === providerID)?.label ?? providerID;
}

/** Renders a window size in the short form a card has room for. */
export function formatContext(tokens: number | null): string {
	if (tokens === null || tokens === undefined || tokens <= 0) return '—';
	if (tokens >= 1_000_000) {
		return (tokens / 1_000_000).toFixed(tokens % 1_000_000 === 0 ? 0 : 1) + 'M';
	}
	if (tokens >= 1000) return Math.round(tokens / 1000) + 'K';
	return String(tokens);
}

/** Swaps one group member with its neighbour, which is how the editor reorders a priority group. */
export function moveMember(members: GroupMember[], index: number, delta: number): GroupMember[] {
	const target = index + delta;
	if (target < 0 || target >= members.length) return members;
	const reordered = [...members];
	const moved = reordered[index];
	reordered[index] = reordered[target];
	reordered[target] = moved;
	return reordered;
}

/** Returns a group's members, treating an absent list as none. */
export function groupMembersOf(group: Group): GroupMember[] {
	return group.members ?? [];
}

/** Counts the members a request could actually use, which is what tells a group that works from
 *  one that only lists. */
export function eligibleMembers(group: Group): number {
	return groupMembersOf(group).filter((member) => member.eligible && member.enabled).length;
}

/** How many connections a group names. A bare model id follows every connection that serves it,
 *  so it is not a provider. */
export function groupProviderCount(group: Group): number {
	const ids = new Set<string>();
	for (const member of groupMembersOf(group)) {
		if (member.provider_id !== '') ids.add(member.provider_id);
	}
	return ids.size;
}

/** One connection's share of a group's members. */
export type ProviderModelCount = {
	providerId: string;
	count: number;
};

/** Groups members by the connection they name, in the order those connections first appear.
 *  A bare model id has no connection, so those members share one empty id and still count as
 *  models added. */
export function providerModelCounts(members: GroupMember[]): ProviderModelCount[] {
	const counts = new Map<string, number>();
	for (const member of members) {
		counts.set(member.provider_id, (counts.get(member.provider_id) ?? 0) + 1);
	}
	return [...counts].map(([providerId, count]) => ({ providerId, count }));
}

/** How many connection logos a group card shows before the overflow mark.
 *  More cards on screen (vertical) leave room for five; wider cards (horizontal) fit seven. */
export function providerIconLimit(layout: 'vertical' | 'horizontal'): number {
	return layout === 'vertical' ? 5 : 7;
}

/** Splits the logos a card draws from the ones it only counts. */
export function visibleProviderModelCounts(
	chips: readonly ProviderModelCount[],
	limit: number
): { visible: readonly ProviderModelCount[]; overflow: number } {
	if (chips.length <= limit) return { visible: chips, overflow: 0 };
	return { visible: chips.slice(0, limit), overflow: chips.length - limit };
}

/** The name a client asks for: every client-facing name carries the Relo namespace, so a group
 *  an operator calls "combo" is reached as "reloc-combo".
 *  The daemon computes it, so the slug rule lives in one place; the identifier is the fallback
 *  for a group the daemon has not published yet. */
export function exposedName(group: Group): string {
	return group.client_id ?? 'reloc-' + group.id;
}

/** Names the advisory mismatches the daemon records on a group: the codes it returns in
 *  capability_warnings, paired with the message a chip shows and the tooltip that explains it. */
export const CAPABILITY_WARNINGS: Record<string, { chip: string; tooltip: string }> = {
	reasoning_off: {
		chip: 'ui.pages.groupsPage.chipReasoningOff',
		tooltip: 'ui.pages.groupsPage.warnReasoning'
	},
	vision_off: {
		chip: 'ui.pages.groupsPage.chipVisionOff',
		tooltip: 'ui.pages.groupsPage.warnVision'
	},
	tools_mixed: {
		chip: 'ui.pages.groupsPage.chipToolsMixed',
		tooltip: 'ui.pages.groupsPage.toolsConflict'
	}
};

/** Maps one warning the daemon recorded onto the message keys a chip and its tooltip read,
 *  falling back to the tools wording for a code this build does not know. */
export function capabilityWarningKeys(warning: string): { chip: string; tooltip: string } {
	return CAPABILITY_WARNINGS[warning] ?? CAPABILITY_WARNINGS.tools_mixed;
}

/** One badge on a group card. */
export type GroupSetupChip = {
	id: 'vision' | 'context' | 'reasoning';
	labelKey?: string;
	label?: string;
	tooltipKey: string;
};

/** The row under a group's name: vision, the context it can promise, then reasoning.
 *  A capability the members never state is left out, and a mixed warning keeps the explanation
 *  the daemon already records. */
export function groupSetupChips(group: Group): GroupSetupChip[] {
	const chips: GroupSetupChip[] = [];
	const warnings = new Set(group.capability_warnings ?? []);
	if (group.supports_vision === true) {
		chips.push({
			id: 'vision',
			labelKey: 'ui.pages.groupsPage.chipVision',
			tooltipKey: 'ui.pages.groupsPage.hintVision'
		});
	} else if (group.supports_vision === false) {
		chips.push({
			id: 'vision',
			labelKey: 'ui.pages.groupsPage.chipVisionOff',
			tooltipKey: warnings.has('vision_off')
				? 'ui.pages.groupsPage.warnVision'
				: 'ui.pages.groupsPage.hintNoVision'
		});
	}
	if (group.context_window) {
		chips.push({
			id: 'context',
			label: formatContext(group.context_window),
			tooltipKey: 'ui.pages.groupsPage.budgetHint'
		});
	}
	if (group.supports_reasoning === true) {
		chips.push({
			id: 'reasoning',
			labelKey: 'ui.pages.groupsPage.chipReasoning',
			tooltipKey: 'ui.pages.groupsPage.hintReasoning'
		});
	} else if (group.supports_reasoning === false) {
		chips.push({
			id: 'reasoning',
			labelKey: 'ui.pages.groupsPage.chipReasoningOff',
			tooltipKey: warnings.has('reasoning_off')
				? 'ui.pages.groupsPage.warnReasoning'
				: 'ui.pages.groupsPage.hintNoReasoning'
		});
	}
	return chips;
}
