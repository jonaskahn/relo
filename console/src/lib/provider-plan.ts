// Whether a connection is sold as a subscription plan. A plan bills a fixed
// fee, so its traffic is plan usage and never pay-as-you-go spend; the
// console, the usage report, and the tray's rows all read this one rule.

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

/** The fields a connection is classified by, so a stored connection and an
 *  add-flow template can both be read. */
export interface PlanInput {
	id?: string;
	template_id?: string;
	modelsdev_provider_id?: string;
	base_url?: string;
}

/** Reports whether a connection is one a plan covers. The keyless free lanes
 *  are neither a plan nor metered, so their caller checks them first. */
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
