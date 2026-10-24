import { isFreeConnection } from '$lib/provider-free';
import { isPlanPricedConnection } from '$lib/provider-plan';

export type BillingInput = {
	id?: string;
	template_id?: string;
	kind?: string;
	auth?: string;
	origin?: string;
	base_url?: string;
	modelsdev_provider_id?: string;
};

const PAID_PLAN_TEMPLATES = new Set(['open-design', 'opencode-go']);
const OPENCODE_IDS = new Set(['opencode-go', 'opencode-free', 'opencode']);

function openCodePlan(p: BillingInput): boolean {
	if (OPENCODE_IDS.has(p.template_id ?? '') || OPENCODE_IDS.has(p.modelsdev_provider_id ?? ''))
		return true;
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

export function isPaidPlanConnection(p: BillingInput): boolean {
	if (isFreeConnection({ template_id: p.template_id ?? '' })) return false;
	if (PAID_PLAN_TEMPLATES.has(p.template_id ?? '')) return true;
	if (p.origin === 'signin' || p.kind === 'signin' || p.auth === 'oauth') return true;
	if (isPlanPricedConnection(p)) return true;
	if (openCodePlan(p) && p.template_id !== 'opencode-free') return true;
	return false;
}

export function isPayAsYouGoConnection(p: BillingInput): boolean {
	if (isFreeConnection({ template_id: p.template_id ?? '' })) return false;
	if (isPaidPlanConnection(p)) return false;
	return p.kind === 'key' || p.kind === 'cloud';
}
