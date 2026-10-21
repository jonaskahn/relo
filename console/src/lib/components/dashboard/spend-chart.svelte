<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import AnimatedNumber from '$lib/components/ui/animated-number.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import {
		boardOverflowLabel,
		pricingOf,
		spendChartLimit,
		visibleBoard,
		type PricingKind,
		type SpendRow
	} from '$lib/dashboard';
	import { formatCost } from '$lib/format';

	// SpendChart is one set of connections read as a list: what the window cost
	// each one, and how that connection is priced. The rows arrive already
	// joined with their connections, so the card draws what the window held.

	interface Props {
		/** The connections of the set, each with what it cost. */
		rows: SpendRow[];
		/** The message the card's title reads. */
		titleKey: string;
		/** The message under the title. */
		descriptionKey: string;
		/** The message a set with nothing in it reads. */
		emptyKey: string;
		loading: boolean;
		pending: Snippet;
	}

	let { rows, titleKey, descriptionKey, emptyKey, loading, pending }: Props = $props();

	// The chip every connection wears, named by the way that connection is priced.
	const pricingKeys: Record<PricingKind, string> = {
		payg: 'ui.pages.dashboard.pricePayAsYouGo',
		plan: 'ui.pages.dashboard.pricePlan',
		free: 'ui.pages.dashboard.priceFree',
		local: 'ui.pages.dashboard.priceLocal'
	};

	const priced = $derived(rows.map((row) => ({ row, pricing: pricingOf(row.provider) })));
	const shown = $derived(visibleBoard(priced, spendChartLimit));
	const overflow = $derived(boardOverflowLabel(priced.length, spendChartLimit));
	const total = $derived(rows.reduce((sum, row) => sum + row.costMicros, 0));
</script>

<div class="section-surface flex min-w-0 flex-col">
	<div class="px-4 pt-4 pb-3 sm:px-5">
		<h2 class="section-heading flex items-center gap-2">
			<Icon name="currency-dollar" size={15} class="shrink-0 text-faint" />
			{$t(titleKey)}
		</h2>
		<p class="text-xs text-muted-foreground">
			{$t(descriptionKey)}{#if !loading && overflow}{' '}{overflow}{/if}
		</p>
	</div>
	<div class="flex min-w-0 flex-1 flex-col gap-3 px-4 pb-4 sm:px-5">
		{#if loading}
			{@render pending()}
		{:else if rows.length === 0}
			<p class="py-4 text-sm text-muted-foreground">{$t(emptyKey)}</p>
		{:else}
			<p class="flex items-center gap-2">
				<span class="font-mono text-2xl leading-none font-medium tabular-nums text-ink"
					><AnimatedNumber value={total} format={(value) => formatCost(Math.round(value))} /></span
				>
			</p>
			<ul class="flex min-w-0 flex-col gap-2">
				{#each shown as entry (entry.row.provider.id)}
					<li class="flex min-w-0 items-center gap-2">
						<ProviderLogo id={entry.row.provider.id} label={entry.row.provider.label} size="sm" />
						<span class="min-w-0 flex-1 truncate text-sm text-ink" title={entry.row.provider.label}
							>{entry.row.provider.label}</span
						>
						{#if entry.row.unpriced > 0}
							<span
								class="inline-flex shrink-0 items-center gap-1 rounded-full bg-warn/10 px-2 py-0.5 text-[0.7rem] font-medium text-warn"
							>
								<Icon name="alert-triangle" size={11} />
								{$t('ui.pages.dashboard.spendUnpriced', {
									values: { count: entry.row.unpriced }
								})}
							</span>
						{/if}
						<span class="pricing-chip shrink-0">
							{$t(pricingKeys[entry.pricing])}
						</span>
						<span class="mono-data shrink-0">{formatCost(entry.row.costMicros)}</span>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</div>

<style>
	/* The pricing chip is the neutral pill: the muted name on a sunken plate, so
	   a row carries its way of being priced without the accent claiming it. */
	.pricing-chip {
		border-radius: 999px;
		background: var(--sunken);
		padding: 2px 8px;
		font-size: 0.7rem;
		font-weight: 500;
		color: var(--muted);
		white-space: nowrap;
	}
</style>
