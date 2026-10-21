<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import AnimatedNumber from '$lib/components/ui/animated-number.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import { boardOverflowLabel, spendChartLimit, topSpendRows, type SpendRow } from '$lib/dashboard';
	import { formatCost } from '$lib/format';

	// SpendChart is one set of connections read as a list: the ones that cost the
	// most, and what each of them carried. The rows arrive already joined with
	// their connections, so the card draws what the window held.

	interface Props {
		/** The connections of the set, each with what it cost. */
		rows: SpendRow[];
		/** The message the card's title reads. */
		titleKey: string;
		/** The mark the title wears. */
		icon: IconName;
		/** The message under the title. */
		descriptionKey: string;
		/** The message a set with nothing in it reads. */
		emptyKey: string;
		loading: boolean;
		pending: Snippet;
	}

	let { rows, titleKey, icon, descriptionKey, emptyKey, loading, pending }: Props = $props();

	// The card names the costliest connections of the set; the total covers all of them.
	const shown = $derived(topSpendRows(rows));
	const overflow = $derived(boardOverflowLabel(rows.length, spendChartLimit));
	const total = $derived(rows.reduce((sum, row) => sum + row.costMicros, 0));
</script>

<div class="section-surface flex min-w-0 flex-col">
	<div class="px-4 pt-4 pb-3 sm:px-5">
		<h2 class="section-heading flex items-center gap-2">
			<Icon name={icon} size={15} class="shrink-0 text-faint" />
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
			<p class="flex items-baseline gap-1.5">
				<span class="font-mono text-2xl leading-none font-medium tabular-nums text-ink"
					><AnimatedNumber value={total} format={(value) => formatCost(Math.round(value))} /></span
				>
			</p>
			<ul class="flex min-w-0 flex-col gap-2">
				{#each shown as row (row.provider.id)}
					<li class="flex min-w-0 items-center gap-2">
						<ProviderLogo id={row.provider.id} label={row.provider.label} size="sm" />
						<span class="min-w-0 truncate text-sm font-medium text-ink" title={row.provider.label}
							>{row.provider.label}</span
						>
						<span class="mono-data ml-auto shrink-0">{formatCost(row.costMicros)}</span>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</div>
