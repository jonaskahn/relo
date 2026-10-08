// The route editor's draft, validation, and exact API body.

import { groupMembersOf } from '$lib/catalog';
import type { Group, GroupMember, Model } from '$lib/types';

/** The route editor's steps, in order. */
export const ROUTE_STEPS = ['basics', 'members', 'review'] as const;

/** One page of the route editor. */
export type RouteStep = (typeof ROUTE_STEPS)[number];

/** A group as the editor holds it while it is being changed. */
export interface RouteDraft {
	id: string;
	label: string;
	strategy: string;
	enabled: boolean;
	listed: boolean;
	// switchOn4xx and switchOn5xx move a request to the route's next member
	// after a status the relay does not always retry.
	switchOn4xx: boolean;
	switchOn5xx: boolean;
	members: GroupMember[];
}

/** Builds a new group's draft. */
export function emptyRouteDraft(): RouteDraft {
	return {
		id: '',
		label: '',
		strategy: 'priority',
		enabled: true,
		listed: true,
		switchOn4xx: true,
		switchOn5xx: true,
		members: []
	};
}

/** Opens an existing route, keeping the members in the order they are stored in. */
export function routeDraftFrom(group: Group): RouteDraft {
	return {
		id: group.id,
		label: group.label,
		strategy: group.strategy,
		enabled: group.enabled,
		listed: group.listed,
		switchOn4xx: group.switch_on_4xx,
		switchOn5xx: group.switch_on_5xx,
		members: groupMembersOf(group).map((member) => ({
			provider_id: member.provider_id,
			model_id: member.model_id,
			kind: memberKind(member),
			weight: member.weight,
			enabled: member.enabled,
			eligible: member.eligible,
			reason: member.reason,
			serving_accounts: member.serving_accounts ?? 0,
			active_accounts: member.active_accounts ?? 0
		}))
	};
}

/** Reads how a member resolves, treating a member stored before routes could name a bare id as
 *  one connection's own model. */
export function memberKind(member: GroupMember): 'model' | 'auto' {
	return member.kind === 'auto' ? 'auto' : 'model';
}

/** Builds a member that names a bare model identifier, which the router follows across every
 *  connection serving it. */
export function autoMember(modelID: string): GroupMember {
	return {
		provider_id: '',
		model_id: modelID,
		kind: 'auto',
		weight: 1,
		enabled: true,
		eligible: true,
		serving_accounts: 0,
		active_accounts: 0
	};
}

/** Renders the parts of a draft a save would store, which is what a close compares against to
 *  know whether anything would be thrown away. */
export function routeSnapshot(draft: RouteDraft): unknown {
	return {
		id: draft.id.trim(),
		label: draft.label.trim(),
		strategy: draft.strategy,
		enabled: draft.enabled,
		listed: draft.listed,
		switchOn4xx: draft.switchOn4xx,
		switchOn5xx: draft.switchOn5xx,
		members: draft.members.map((member) => ({
			provider_id: member.provider_id,
			model_id: member.model_id,
			kind: memberKind(member),
			weight: member.weight,
			enabled: member.enabled
		}))
	};
}

/** Reports the fields the first step still needs.
 *  The id is the model name a client asks for, so it is the one thing that has to be there. */
export function validateBasics(draft: RouteDraft): Record<string, string> {
	const problems: Record<string, string> = {};
	if (draft.id.trim() === '') problems.id = 'ui.pages.groupsPage.needId';
	return problems;
}

/** Reports whether the route can resolve at all: a route with no members has nothing to send a
 *  request to, and a route whose members disagree on calling tools cannot be promised to a
 *  caller. */
export function validateMembers(
	draft: RouteDraft,
	meta: Record<string, Model> = {}
): Record<string, string> {
	const problems: Record<string, string> = {};
	if (draft.members.length === 0) problems.members = 'ui.pages.groupsPage.needMembers';
	else if (memberCapabilities(draft, meta).toolsConflict) {
		problems.members = 'ui.pages.groupsPage.toolsConflict';
	}
	return problems;
}

/** What a set of members can promise a caller. */
export interface MemberCapabilities {
	// toolsConflict is true when one enabled member calls tools and another
	// explicitly does not, which the daemon refuses to save.
	toolsConflict: boolean;
	// reasoningMixed and visionMixed are true when one enabled member states
	// the capability and another explicitly lacks it, so the group cannot
	// promise it to a client that reads the published model list.
	reasoningMixed: boolean;
	visionMixed: boolean;
}

/** Reads what the members of a draft can promise.
 *  A member whose model the console has not read yet is left out rather than guessed at; the
 *  daemon enforces the tool rule on save regardless. */
export function memberCapabilities(
	draft: RouteDraft,
	meta: Record<string, Model>
): MemberCapabilities {
	let calling = false;
	let refusing = false;
	let reasoning = false;
	let noReasoning = false;
	let vision = false;
	let noVision = false;
	for (const member of resolvedModels(draft, meta)) {
		const { tools, reasoning: reasons, vision: sees } = member.capabilities;
		if (tools === true) calling = true;
		if (tools === false) refusing = true;
		if (reasons === true) reasoning = true;
		if (reasons === false) noReasoning = true;
		if (sees === true) vision = true;
		if (sees === false) noVision = true;
	}
	return {
		toolsConflict: calling && refusing,
		reasoningMixed: reasoning && noReasoning,
		visionMixed: vision && noVision
	};
}

function resolvedModels(draft: RouteDraft, meta: Record<string, Model>): Model[] {
	const models: Model[] = [];
	for (const member of draft.members) {
		if (!member.enabled) continue;
		if (memberKind(member) === 'auto' || member.provider_id === '') {
			for (const model of Object.values(meta)) {
				if (model.model_id === member.model_id) models.push(model);
			}
			continue;
		}
		const model = meta[member.provider_id + '/' + member.model_id];
		if (model) models.push(model);
	}
	return models;
}

/** Renders the exact request the daemon is asked to store. */
export function routeSaveBody(draft: RouteDraft): Record<string, unknown> {
	return {
		label: draft.label.trim(),
		strategy: draft.strategy,
		enabled: draft.enabled,
		listed: draft.listed,
		switch_on_4xx: draft.switchOn4xx,
		switch_on_5xx: draft.switchOn5xx,
		members: draft.members.map((member) => ({
			provider_id: member.provider_id,
			model_id: member.model_id,
			kind: memberKind(member),
			weight: member.weight,
			enabled: member.enabled
		}))
	};
}

/** Counts the members a request could use: enabled and with an account that can serve it. */
export function eligibleMemberCount(members: GroupMember[]): number {
	return members.filter((member) => member.enabled && member.eligible).length;
}

/** Whether a request could use one member: serving, paused by the operator, or ineligible
 *  because no account can serve it. */
export type MemberStanding = 'serving' | 'paused' | 'ineligible';

/** Reads where one member stands: serving, paused, or one the rule will not pick. */
export function memberStanding(member: GroupMember): MemberStanding {
	if (!member.enabled) return 'paused';
	if (!member.eligible) return 'ineligible';
	return 'serving';
}

/** Lists the members the way the editor shows them: cheapest reads the catalog rates and sorts
 *  by them, every other ordering keeps the order the draft stores.
 *  Unknown rates sort last and ties keep their order. */
export function displayMembers(draft: RouteDraft, meta: Record<string, Model>): GroupMember[] {
	if (draft.strategy !== 'cheapest') return draft.members;
	return draft.members
		.map((member, position) => ({ member, position, rate: memberRate(member, meta) }))
		.sort((a, b) => a.rate - b.rate || a.position - b.position)
		.map((entry) => entry.member);
}

function memberRate(member: GroupMember, meta: Record<string, Model>): number {
	const model = meta[member.provider_id + '/' + member.model_id];
	if (!model) return Number.POSITIVE_INFINITY;
	const { input, output } = model.prices.effective;
	if (input === null && output === null) return Number.POSITIVE_INFINITY;
	return (input ?? 0) + (output ?? 0);
}

/** One member's share of a weighted group's requests, in whole percent.
 *  A paused member weighs nothing, so its row reads 0%. */
export function weightShare(member: GroupMember, members: GroupMember[]): number {
	if (!member.enabled) return 0;
	const total = members.reduce((sum, entry) => sum + (entry.enabled ? entry.weight || 0 : 0), 0);
	if (total <= 0) return 0;
	return Math.round(((member.weight || 0) / total) * 100);
}

/** Names the message that explains a member's place in the preview path.
 *  Weighted members carry their share instead, so only the first-member and round-robin wordings
 *  live here. */
export function resolutionReason(strategy: string, position: number): string {
	if (strategy === 'round-robin') return 'ui.pages.groupsPage.when.roundRobin';
	return position === 0
		? 'ui.pages.groupsPage.when.first'
		: 'ui.pages.groupsPage.when.afterUnavailable';
}
