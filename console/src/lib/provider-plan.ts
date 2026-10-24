// How a connection is sold, decided once. A plan bills a fixed fee, so its
// traffic is plan usage and never pay-as-you-go spend; the chip, the spend
// board, the usage report, and the tray's rows all read this one rule.

import { isFreeConnection } from './provider-free';

/** The identifier families a plan is sold under. models.dev lists a plan
 *  provider under one of these, and a template keeps the name it came with. */
const PLAN_ID_MARKERS = ['token-plan', 'coding-plan', 'code-plan', 'step-plan'];

/** The hosts a plan is served from. Alibaba sells its Coding Plan off a
 *  `coding` DashScope host, while the general key stays on the
 *  pay-as-you-go host, so the host is the only thing that separates them
 *  when a custom endpoint is pointed at one. */
const PLAN_HOST_MARKERS = [
	'token-plan',
	'coding.dashscope.aliyuncs.com',
	'coding-intl.dashscope.aliyuncs.com'
];

/** The curated templates sold as a subscription whose own kind is a plain key,
 *  so nothing about their identifier or host says they are a plan.
 *  `daemon/internal/cli/desktop_source.go` keeps the same list, and
 *  `provider-plan.test.ts` asserts the two agree. */
export const PLAN_TEMPLATES: readonly string[] = ['open-design', 'opencode-go'];

/** The identifiers the OpenCode gateway answers to. */
const OPENCODE_IDS = ['opencode-go', 'opencode-free', 'opencode'];

/** The fields a connection is classified by, so a stored connection and an
 *  add-flow template can both be read. */
export interface PlanInput {
	id?: string;
	template_id?: string;
	modelsdev_provider_id?: string;
	base_url?: string;
	kind?: string;
	auth?: string;
	origin?: string;
}

/** Names how one connection is sold, which is also how its traffic counts. */
export type BillingLane = 'free' | 'plan' | 'payg' | 'local';

/** Reports whether a connection is one a plan covers by its identifiers or its
 *  host, ignoring the curated template list and the sign-in lane. */
export function isPlanPricedConnection(p: PlanInput): boolean {
	for (const id of [p.id, p.template_id, p.modelsdev_provider_id]) {
		const lower = (id ?? '').toLowerCase();
		if (PLAN_ID_MARKERS.some((marker) => lower.includes(marker))) return true;
	}
	try {
		const host = new URL(p.base_url ?? '').hostname.toLowerCase();
		return PLAN_HOST_MARKERS.some((marker) => host.includes(marker));
	} catch {
		return false;
	}
}

/** Reports whether a connection is served by the OpenCode gateway. A custom
 *  base URL that still points at its plan paths counts the same way. */
export function isOpenCodePlan(p: PlanInput): boolean {
	for (const id of [p.template_id, p.modelsdev_provider_id]) {
		if (OPENCODE_IDS.includes(id ?? '')) return true;
	}
	try {
		const u = new URL(p.base_url ?? '');
		const host = u.hostname.toLowerCase();
		const path = u.pathname.toLowerCase();
		return (
			host.includes('opencode.ai') && (path.includes('/zen') || path.includes('/inference/go'))
		);
	} catch {
		return false;
	}
}

/** Reports whether a connection is a subscription: a sign-in account, a curated
 *  plan template, a plan-priced identifier or host, or the OpenCode gateway.
 *  The keyless free lanes answer false, because they are neither a plan nor
 *  metered. */
export function isPlanConnection(p: PlanInput): boolean {
	if (isFreeConnection({ template_id: p.template_id ?? '' })) return false;
	if (PLAN_TEMPLATES.includes(p.template_id ?? '')) return true;
	if (p.origin === 'signin' || p.kind === 'signin' || p.auth === 'oauth') return true;
	if (isPlanPricedConnection(p)) return true;
	return isOpenCodePlan(p);
}

/** Names how one connection is sold. Every surface reads this rather than
 *  carrying its own list, so a chip and a spend bucket cannot disagree. */
export function billingLane(p: PlanInput): BillingLane {
	if (isFreeConnection({ template_id: p.template_id ?? '' })) return 'free';
	if (isPlanConnection(p)) return 'plan';
	if (p.kind === 'key' || p.kind === 'cloud') return 'payg';
	return 'local';
}
