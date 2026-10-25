// The "smart" half of the group editor: which billing lanes a group may draw
// members from, and which models on other connections are similar enough to
// a member already chosen to be worth offering as a one-click add.
//
// The lanes and the similarity rule both live here, beside their tests, so
// the picker and the Billing step read one implementation instead of two.
import type { GroupMember, Model, Provider } from '$lib/types';
import { billingLane, type BillingLane } from '$lib/provider-plan';

/** The billing lanes a group may be drawn from. `manual` means "no filter":
 *  every connection, including local endpoints. */
export const LANES = ['payg', 'plan', 'free', 'manual'] as const;
export type Lane = (typeof LANES)[number];

/** Reports how many connections belong to each lane among the given providers. */
export interface LaneCounts {
	payg: number;
	plan: number;
	free: number;
	manual: number;
}

/** Counts configured connections by the lane their billing places them in. */
export function laneCounts(providers: readonly Provider[]): LaneCounts {
	const counts: LaneCounts = { payg: 0, plan: 0, free: 0, manual: 0 };
	for (const provider of providers) {
		const lane = billingLane(provider);
		if (lane === 'payg' || lane === 'plan' || lane === 'free') counts[lane]++;
	}
	counts.manual = providers.length;
	return counts;
}

/** Reports whether a provider is allowed by the chosen lanes. An empty lane set
 *  (or `manual` present) allows every connection, including local ones. */
export function allowedByLanes(provider: Provider, lanes: readonly Lane[]): boolean {
	if (lanes.length === 0 || lanes.includes('manual')) return true;
	const lane = billingLane(provider);
	return lane === 'payg' || lane === 'plan' || lane === 'free'
		? lanes.includes(lane)
		: lanes.includes('manual');
}

/** Filters a model list down to the connections the chosen lanes allow. */
export function modelsInLanes(
	models: readonly Model[],
	providers: readonly Provider[],
	lanes: readonly Lane[]
): Model[] {
	const allowed = new Set(providers.filter((p) => allowedByLanes(p, lanes)).map((p) => p.id));
	return models.filter((model) => allowed.has(model.provider_id));
}

/** A normalized family key for a model. Prefers the models.dev family; when
 *  that is absent (no catalog match) it falls back to the model id with
 *  region prefixes, vendor prefixes, version numbers, date stamps and
 *  context markers stripped, so `claude-sonnet-4-6` and `claude-sonnet-5`
 *  collapse to the same key. */
export function familyKey(model: Model): string {
	if (model.family && model.family.trim() !== '') return model.family.trim().toLowerCase();
	let id = model.model_id.toLowerCase().trim();
	if (id.includes('/')) id = id.slice(id.lastIndexOf('/') + 1);
	for (const prefix of ['us.', 'eu.', 'apac.', 'jp.', 'global.']) {
		if (id.startsWith(prefix)) id = id.slice(prefix.length);
	}
	id = id.replace(/@\w+/g, '');
	id = id.replace(/-\d{8}/g, '');
	id = id.replace(/\d{8}/g, '');
	id = id.replace(/\[1m\]/gi, '');
	id = id.replace(/-1m\b/g, '');
	id = id.replace(/-v?\d+(\.\d+)*$/g, '');
	id = id.replace(/\b\d+(\.\d+)*\b/g, '');
	id = id.replace(/[-_.\s]+/g, '-');
	id = id.replace(/^-+|-+$/g, '');
	return id;
}

/** Models on *other* connections that are similar to a seed model: same family,
 *  same category, on a connection the chosen lanes allow, enabled and routable,
 *  with at least one serving account. Already-member models and the seed
 *  itself are excluded. Results are newest-first, then cheapest. */
export function similarModels(
	all: readonly Model[],
	seed: Model,
	providers: readonly Provider[],
	lanes: readonly Lane[],
	existingMembers: readonly GroupMember[] = []
): Model[] {
	const providerMap = new Map(providers.map((provider) => [provider.id, provider]));
	const existingKeys = new Set(
		existingMembers.map((member) => member.provider_id + '/' + member.model_id)
	);
	const existingKeysForAuto = new Set(
		existingMembers
			.filter((member) => member.provider_id === '' && member.model_id === seed.model_id)
			.map(() => '')
	);
	const seedTools = seed.capabilities.tools;
	const existingTools = existingMembers.map((member) => {
		const found = all.find(
			(model) => model.provider_id === member.provider_id && model.model_id === member.model_id
		);
		return found?.capabilities.tools ?? null;
	});
	const seedKey = familyKey(seed);
	const seedCategory = seed.category;
	const results: Model[] = [];
	const seen = new Set<string>();
	for (const model of all) {
		if (model.provider_id === seed.provider_id && model.model_id === seed.model_id) continue;
		if (!model.enabled) continue;
		if (!model.routable) continue;
		if ((model.serving_accounts ?? 0) <= 0) continue;
		const provider = providerMap.get(model.provider_id);
		if (!provider) continue;
		if (!allowedByLanes(provider, lanes)) continue;
		if (familyKey(model) !== seedKey) continue;
		if (seedCategory && model.category && model.category !== seedCategory) continue;
		const key = model.provider_id + '/' + model.model_id;
		if (seen.has(key)) continue;
		if (existingKeys.has(key)) continue;
		if (existingKeysForAuto.size > 0 && model.model_id === seed.model_id) continue;
		const tools = model.capabilities.tools;
		if (seedTools !== null && tools !== null && seedTools !== tools) {
			// tools conflict with the seed itself
			continue;
		}
		if (existingTools.some((value) => value !== null && tools !== null && value !== tools)) {
			// would introduce a tools conflict with an already-chosen member
			continue;
		}
		seen.add(key);
		results.push(model);
	}
	results.sort((left, right) => {
		const release = (right.release_date ?? '').localeCompare(left.release_date ?? '');
		if (release !== 0) return release;
		const priceLeft = left.prices.effective.input ?? Number.POSITIVE_INFINITY;
		const priceRight = right.prices.effective.input ?? Number.POSITIVE_INFINITY;
		return priceLeft - priceRight;
	});
	return results;
}

/** Groups similar-model suggestions by family, preserving the order they were
 *  produced in (newest-first, cheapest). */
export function groupSuggestions(models: readonly Model[]): { family: string; models: Model[] }[] {
	const groups: { family: string; models: Model[] }[] = [];
	const byFamily = new Map<string, Model[]>();
	for (const model of models) {
		const key = familyKey(model);
		const list = byFamily.get(key);
		if (list) list.push(model);
		else byFamily.set(key, [model]);
	}
	for (const [family, list] of byFamily) {
		groups.push({ family, models: list });
	}
	return groups;
}

/** Reports how many configured connections fall in each billing lane. */
export function connectionCountsByLane(
	providers: readonly Provider[]
): Record<BillingLane, number> {
	const counts: Record<BillingLane, number> = { free: 0, plan: 0, payg: 0, local: 0 };
	for (const provider of providers) counts[billingLane(provider)]++;
	return counts;
}

export type { BillingLane };
