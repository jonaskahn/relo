import { isFreeConnection } from '$lib/provider-free';
import { isPlanConnection } from '$lib/provider-plan';

export type BillingInput = {
	id?: string;
	template_id?: string;
	kind?: string;
	auth?: string;
	origin?: string;
	base_url?: string;
	modelsdev_provider_id?: string;
};

export function isPaidPlanConnection(p: BillingInput): boolean {
	return isPlanConnection(p);
}

export function isPayAsYouGoConnection(p: BillingInput): boolean {
	if (isFreeConnection({ template_id: p.template_id ?? '' })) return false;
	if (isPaidPlanConnection(p)) return false;
	return p.kind === 'key' || p.kind === 'cloud';
}
