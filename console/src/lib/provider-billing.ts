import type { Provider } from '$lib/types';
import { isFreeConnection } from '$lib/provider-free';

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

function tokenPlan(p: BillingInput): boolean {
	const ids = [p.id, p.template_id, p.modelsdev_provider_id];
	if (ids.some((v) => (v ?? '').toLowerCase().includes('token-plan'))) return true;
	try {
		return new URL(p.base_url ?? '').hostname.toLowerCase().includes('token-plan');
	} catch {
		return false;
	}
}

function openCodePlan(p: BillingInput): boolean {
	if (OPENCODE_IDS.has(p.template_id ?? '') || OPENCODE_IDS.has(p.modelsdev_provider_id ?? ''))
		return true;
	try {
		const u = new URL(p.base_url ?? '');
		const host = u.hostname.toLowerCase();
		const path = u.pathname.toLowerCase();
		return host.includes('opencode.ai') && (path.includes('/zen') || path.includes('/inference/go'));
	} catch {
		return false;
	}
}

export function isPaidPlanConnection(p: BillingInput): boolean {
	if (isFreeConnection(p as any)) return false;
	if (PAID_PLAN_TEMPLATES.has(p.template_id ?? '')) return true;
	if (p.origin === 'signin' || p.kind === 'signin' || p.auth === 'oauth') return true;
	if (tokenPlan(p)) return true;
	if (openCodePlan(p) && p.template_id !== 'opencode-free') return true;
	return false;
}

export function isPayAsYouGoConnection(p: BillingInput): boolean {
	if (isFreeConnection(p as any)) return false;
	if (isPaidPlanConnection(p)) return false;
	return p.kind === 'key' || p.kind === 'cloud';
}