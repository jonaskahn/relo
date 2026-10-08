<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { fade, slide } from 'svelte/transition';
	import { t } from 'svelte-i18n';

	import AnimatedNumber from '$lib/components/ui/animated-number.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import { reducedMotion } from '$lib/card-layout-motion';
	import { boardOverflowLabel, spendBoardLimit, visibleBoard, type SpendRow } from '$lib/dashboard';
	import { formatCost } from '$lib/format';
	import type { UsageRollupRow } from '$lib/types';

	// SpendBoard is the money half of the dashboard's second row: one line per
	// connection, and the access keys behind the one the operator opened. The
	// rows arrive already joined, so the board totals what it draws.

	/** The row the operator opened, with what its key read answered. */
	interface OpenedRow {
		providerId: string;
		keys: UsageRollupRow[];
		loading: boolean;
	}

	interface Props {
		rows: SpendRow[];
		loading: boolean;
		// swapped is true when the rows on screen belong to a choice that just
		// changed, which is what lets the board fade the new figures in while a
		// plain refresh stays quiet.
		swapped: boolean;
		set: 'api' | 'plan';
		opened: OpenedRow | null;
		onsetset: (set: 'api' | 'plan') => void;
		onexpand: (providerId: string) => void;
		pending: Snippet;
	}

	let { rows, loading, swapped, set, opened, onsetset, onexpand, pending }: Props = $props();

	const shown = $derived(visibleBoard(rows, spendBoardLimit));
	const overflow = $derived(boardOverflowLabel(rows.length, spendBoardLimit));
	const totalSpend = $derived(rows.reduce((sum, row) => sum + row.costMicros, 0));
	const fadeMs = $derived(reducedMotion() || !swapped ? 0 : 140);
</script>

<div class="section-surface flex min-w-0 flex-col">
	<div class="flex flex-wrap items-center justify-between gap-3 px-4 pt-4 pb-3 sm:px-5">
		<div class="min-w-0">
			<h2 class="section-heading">{$t('ui.pages.dashboard.spendTitle')}</h2>
			<p class="text-xs text-muted-foreground">
				{#if !loading && overflow}{overflow}{/if}
			</p>
		</div>
		<div class="flex flex-wrap items-center gap-2">
			<Segmented
				value={set}
				options={[
					{ value: 'api', label: $t('ui.pages.dashboard.spendApi') },
					{ value: 'plan', label: $t('ui.pages.dashboard.spendPlan') }
				]}
				ariaLabel={$t('ui.pages.dashboard.spendTitle')}
				onchange={(next) => onsetset(next === 'plan' ? 'plan' : 'api')}
			/>
		</div>
	</div>
	<div class="flex flex-col gap-2 px-4 pb-4 sm:px-5">
		{#if loading}
			{@render pending()}
		{:else if rows.length === 0}
			<p class="py-4 text-sm text-muted-foreground">
				{set === 'api'
					? $t('ui.pages.dashboard.spendEmpty')
					: $t('ui.pages.dashboard.spendPlanEmpty')}
			</p>
		{:else}
			<p class="flex flex-wrap items-center gap-2" in:fade={{ duration: fadeMs }}>
				<span class="font-mono text-2xl leading-none font-medium tabular-nums text-ink"
					><AnimatedNumber
						value={totalSpend}
						format={(value) => formatCost(Math.round(value))}
					/></span
				>
			</p>
			{#each shown as row (row.provider.id)}
				<div class="rounded-panel border border-line bg-surface" in:fade={{ duration: fadeMs }}>
					<button
						type="button"
						data-plain
						class="flex w-full min-w-0 flex-wrap items-center gap-x-3 gap-y-1 rounded-panel px-3 py-2 text-left transition-colors hover:bg-hover"
						aria-expanded={opened?.providerId === row.provider.id}
						onclick={() => onexpand(row.provider.id)}
					>
						<Icon
							name={opened?.providerId === row.provider.id ? 'chevron-down' : 'chevron-right'}
							size={14}
							class="shrink-0 text-faint"
						/>
						<ProviderLogo id={row.provider.id} label={row.provider.label} size="sm" />
						<span class="min-w-0 flex-1 truncate text-sm text-ink">{row.provider.label}</span>
						{#if row.unpriced > 0}
							<span
								class="inline-flex items-center gap-1 rounded-full bg-warn/10 px-2 py-0.5 text-[0.7rem] font-medium text-warn"
							>
								<Icon name="alert-triangle" size={11} />
								{$t('ui.pages.dashboard.spendUnpriced', { values: { count: row.unpriced } })}
							</span>
						{/if}
						<span class="mono-data shrink-0">{formatCost(row.costMicros)}</span>
						<span class="mono-data w-16 shrink-0 text-right text-muted-foreground"
							>{row.requests.toLocaleString()}</span
						>
					</button>
					{#if opened?.providerId === row.provider.id}
						<div
							class="m-2 mt-0 rounded-panel bg-sunken px-3 py-2"
							transition:slide={{ duration: reducedMotion() ? 0 : 220, easing: cubicOut }}
						>
							{#if opened.loading}
								{@render pending()}
							{:else}
								{#each opened.keys as key (key.Key)}
									<div class="flex items-center justify-between gap-2 py-0.5 text-xs">
										<span class="min-w-0 truncate"
											>{key.Key || $t('ui.pages.dashboard.spendUnknownKey')}</span
										>
										<span class="mono-data"
											>{formatCost(Number(key.CostMicros))} · {Number(
												key.Requests
											).toLocaleString()}</span
										>
									</div>
								{/each}
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		{/if}
		<p class="text-xs text-muted-foreground">{$t('ui.pages.dashboard.spendNote')}</p>
	</div>
</div>
