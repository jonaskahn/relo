<script lang="ts">
	import { t } from 'svelte-i18n';

	import type { SpendReadout } from '$lib/usage-metrics';
	import type { UsageRollupRow } from '$lib/types';

	// RankTable is one of the two lists under the overview: the busiest
	// connections or the busiest models, each row a bar behind a name. What a
	// row states is the page's: a connection's spend and a model's are read
	// differently.

	interface Props {
		titleKey: string;
		rows: UsageRollupRow[];
		captionOf: (row: UsageRollupRow) => string;
		spendOf: (row: UsageRollupRow) => SpendReadout;
	}

	let { titleKey, rows, captionOf, spendOf }: Props = $props();

	const top = $derived(Math.max(1, ...rows.map((row) => spendOf(row).weight)));
</script>

<div class="section-surface p-4 sm:p-5">
	<h2 class="section-heading">{$t(titleKey)}</h2>
	{#if rows.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<ul class="mt-3 space-y-2">
			{#each rows as row (row.Key)}
				{@const caption = captionOf(row)}
				{@const spend = spendOf(row)}
				<li class="relative overflow-hidden rounded-md">
					<span
						class="absolute inset-y-0 start-0 bg-accent/10"
						style="width: {Math.max(2, (spend.weight / top) * 100)}%"
						aria-hidden="true"
					></span>
					<div class="relative flex items-baseline justify-between gap-3 px-2 py-1.5">
						<span
							class="mono-data truncate text-sm"
							title={caption !== row.Key ? row.Key : undefined}>{caption}</span
						>
						<span
							class="flex shrink-0 items-baseline gap-2 text-xs text-muted-foreground tabular-nums"
						>
							<span>{row.Requests.toLocaleString()} {$t('ui.pages.usagePage.unitRequests')}</span>
							<span class="text-sm font-medium text-foreground">{spend.text}</span>
						</span>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>
