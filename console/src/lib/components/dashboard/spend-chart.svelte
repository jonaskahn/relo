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

	// A free lane wears the same chip the connections page gives it; the other
	// prices are quiet, so only the plan claims the accent.
	const pricingTones: Record<PricingKind, string> = {
		payg: 'border border-border text-muted-foreground',
		plan: 'border border-accent/40 text-accent-ink',
		free: 'border border-ok/40 bg-ok/10 text-ok',
		local: 'border border-border text-muted-foreground'
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
						<span
							class="min-w-0 truncate text-sm font-medium text-ink"
							title={entry.row.provider.label}>{entry.row.provider.label}</span
						>
						<!-- The chip that says how the connection is priced stands
						     right beside the name; the amount keeps the far end. -->
						<span class="badge shrink-0 {pricingTones[entry.pricing]}">
							{$t(pricingKeys[entry.pricing])}
						</span>
						<span class="mono-data ml-auto shrink-0">{formatCost(entry.row.costMicros)}</span>
						{#if entry.row.unpriced > 0}
							<span class="badge shrink-0 border border-warn/40 bg-warn/10 text-warn">
								<Icon name="alert-triangle" size={11} />
								{$t('ui.pages.dashboard.spendUnpriced', {
									values: { count: entry.row.unpriced }
								})}
							</span>
						{/if}
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</div>
