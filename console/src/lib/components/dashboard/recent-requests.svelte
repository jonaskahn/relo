<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import LiveButton from '$lib/components/ui/live-button.svelte';
	import { formatCost, formatDuration } from '$lib/format';
	import type { UsageLogRow } from '$lib/types';

	// RecentRequests is the tail of the request log as it arrives: fifteen
	// dense rows, each a link into the Logs detail. The list stretches to the
	// height of the blocks beside it so both end on one line.

	interface Props {
		rows: UsageLogRow[];
		loading?: boolean;
		live?: boolean;
		pending?: Snippet;
		ontoggle: (live: boolean) => void;
		onviewall: () => void;
	}

	let { rows, loading = false, live = false, pending, ontoggle, onviewall }: Props = $props();
</script>

<div class="section-surface flex min-h-0 flex-col">
	<div class="flex items-center justify-between gap-3 px-4 pt-4 pb-3 sm:px-5">
		<div>
			<h2 class="section-heading">{$t('ui.pages.dashboard.recentTitle')}</h2>
			<p class="text-xs text-muted-foreground">{$t('ui.pages.dashboard.recentDescription')}</p>
		</div>
		<div class="flex shrink-0 items-center gap-2">
			<LiveButton {live} ontoggle={() => ontoggle(!live)} />
			<IconAction
				icon="external-link"
				variant="outline"
				label={$t('ui.pages.dashboard.viewAll')}
				onclick={onviewall}
			/>
		</div>
	</div>
	<div class="flex min-h-0 flex-1 flex-col gap-1.5 overflow-auto px-4 pb-4 sm:px-5">
		{#if loading}
			{@render pending?.()}
		{:else if rows.length === 0}
			<p class="py-8 text-center text-sm text-muted-foreground">
				{$t('ui.pages.dashboard.recentEmpty')}
			</p>
		{:else}
			<!-- One grid template serves the header and every row, so the
			     model, provider, duration and cost columns keep their
			     places however the values change. -->
			<div class="log-row-grid px-3.5 pb-1.5 text-xs font-medium text-muted-foreground">
				<span>{$t('ui.pages.logsPage.columnStatus')}</span>
				<span>{$t('ui.pages.logsPage.columnModel')}</span>
				<span class="hidden md:block">{$t('ui.pages.logsPage.columnProvider')}</span>
				<span class="hidden text-right sm:block">{$t('ui.pages.logsPage.columnDuration')}</span>
				<span class="text-right">{$t('ui.pages.logsPage.columnCost')}</span>
			</div>
			{#each rows as row (row.ID)}
				<a
					href={'/logs?request=' + row.ID}
					class="log-row-grid rounded-panel border border-line bg-surface px-3.5 py-2 transition-[border-color,box-shadow,background-color] duration-150 hover:border-accent-border hover:bg-hover hover:shadow-[var(--shadow-hover)]"
				>
					<span class="flex min-w-0 items-center gap-1.5">
						<Icon
							name={row.Status >= 400 ? 'circle-x' : 'circle-check'}
							size={13}
							class={row.Status >= 400 ? 'text-danger' : 'text-ok'}
						/>
						<span class="mono-data {row.Status >= 400 ? 'text-danger' : 'text-muted-foreground'}"
							>{row.Status}</span
						>
					</span>
					<span class="mono-data min-w-0 truncate text-ink" title={row.Model}>{row.Model}</span>
					<span
						class="mono-data hidden min-w-0 truncate text-muted-foreground md:block"
						title={row.Provider}>{row.Provider}</span
					>
					<span class="mono-data hidden text-right tabular-nums text-muted-foreground sm:block"
						>{formatDuration(row.DurationMs)}</span
					>
					<span class="mono-data text-right tabular-nums"
						>{formatCost(row.EstimatedCostMicros)}</span
					>
				</a>
			{/each}
		{/if}
	</div>
</div>

<style>
	/* One grid template serves the recent-request header and every row, so
	   the model, provider, duration and cost columns keep their places
	   however the values change and the window narrows. */
	.log-row-grid {
		display: grid;
		align-items: center;
		column-gap: 0.75rem;
		grid-template-columns: 3.25rem minmax(0, 1fr) 5.5rem;
	}

	@media (min-width: 640px) {
		.log-row-grid {
			grid-template-columns: 3.25rem minmax(0, 1fr) 4.5rem 5.5rem;
		}
	}

	@media (min-width: 768px) {
		.log-row-grid {
			grid-template-columns: 3.25rem minmax(0, 1fr) 7rem 4.5rem 5.5rem;
		}
	}
</style>
