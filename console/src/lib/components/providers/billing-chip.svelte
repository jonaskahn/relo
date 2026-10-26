<script lang="ts">
	import { t } from 'svelte-i18n';
	import { slide } from 'svelte/transition';

	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { isFreeConnection } from '$lib/provider-free';
	import {
		isPaidPlanConnection,
		isPayAsYouGoConnection,
		type BillingInput
	} from '$lib/provider-billing';

	// A connection wears exactly one spending chip: the keyless lane that bills
	// nothing, the plan that bills a fixed fee, or the credential that pays per
	// token. Every surface that names a connection draws it from here, so the
	// three lanes are read the same way on the card, the row, the detail header,
	// and the add flow.
	//
	// The word is the meaning and the icon is the mark. A surface with no room
	// beside a title passes hover, which reveals the word on the pointer instead
	// of the word standing there all the time; below the small breakpoint the
	// word folds to the icon on every surface, so a narrow card keeps its header
	// on one line.
	interface Props {
		provider: BillingInput;
		hover?: boolean;
		// class carries a surface's own placement, which the chip does not decide:
		// a list row aligns it to the first line, a title keeps it inline.
		class?: string;
	}

	let { provider, hover = false, class: extra = '' }: Props = $props();

	const free = $derived(isFreeConnection({ template_id: provider.template_id ?? '' }));
	const paidPlan = $derived(!free && isPaidPlanConnection(provider));
	const payg = $derived(!free && !paidPlan && isPayAsYouGoConnection(provider));

	const chip = $derived.by((): { mark: string; icon: IconName; label: string } => {
		if (free) {
			return {
				mark: 'data-free-chip',
				icon: 'sparkles',
				label: $t('ui.pages.providersPage.list.tagFree')
			};
		}
		if (paidPlan) {
			return {
				mark: 'data-paid-chip',
				icon: 'layers',
				label: $t('ui.pages.providersPage.list.tagPaidPlan')
			};
		}
		return {
			mark: 'data-payg-chip',
			icon: 'currency-dollar',
			label: $t('ui.pages.providersPage.list.tagPayAsYouGo')
		};
	});

	// The tone is the lane's own: a keyless pool reads as the good news it is, a
	// plan as the product it was bought as, and a key as the neutral default.
	const tone = $derived(
		free
			? 'bg-ok/10 text-ok'
			: paidPlan
				? 'bg-primary/10 text-primary'
				: 'bg-muted text-muted-foreground'
	);

	let pointed = $state(false);
	const shown = $derived(!hover || pointed);
</script>

{#if free || paidPlan || payg}
	<span
		{...{ [chip.mark]: true }}
		class="badge shrink-0 {tone} {extra}"
		aria-label={chip.label}
		role="img"
		onmouseenter={() => (pointed = true)}
		onmouseleave={() => (pointed = false)}
	>
		<Icon name={chip.icon} size={11} />
		{#if shown}
			<span
				transition:slide={hover ? { duration: 150, axis: 'x' } : { duration: 0 }}
				class={hover ? 'ms-1' : 'ms-1 max-sm:hidden'}>{chip.label}</span
			>
		{/if}
	</span>
{/if}
