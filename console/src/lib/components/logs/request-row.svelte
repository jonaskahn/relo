<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import {
		formatCost,
		formatDateTime,
		formatDuration,
		formatTimestamp,
		formatTokens
	} from '$lib/format';
	import type { UsageLogRow } from '$lib/types';

	// RequestRow is one request as a dense row-card: time, model and cost
	// survive every width; provider, tokens and duration join on wider
	// screens; origin and client key last. Status pairs its icon with the
	// code, never colour alone. The whole card opens the detail modal.
	interface Props {
		row: UsageLogRow;
		maxTokens?: number;
		onopen: (row: UsageLogRow) => void;
	}

	let { row, maxTokens = 0, onopen }: Props = $props();

	const totalTokens = $derived(Number(row.InputTokens || 0) + Number(row.OutputTokens || 0));
	const tokenBarPercent = $derived(
		maxTokens > 0 ? Math.max(3, Math.min(100, (totalTokens / maxTokens) * 100)) : 0
	);
	const isSlow = $derived(row.DurationMs >= 120_000);
</script>

<button
	type="button"
	data-plain
	onclick={() => onopen(row)}
	aria-label={$t('ui.common.openCard', {
		values: { name: row.Model + ' ' + formatTimestamp(row.TimestampMs) }
	})}
	class="card-press grid w-full grid-cols-[5rem_minmax(0,1.8fr)_4.25rem_5rem] items-center gap-x-3 rounded-panel border border-line bg-surface px-3.5 py-2 text-left hover:border-accent-border hover:bg-hover hover:shadow-[var(--shadow-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:grid-cols-[5rem_minmax(0,1.8fr)_4.25rem_5rem_5.5rem_4.75rem] md:grid-cols-[5rem_minmax(0,2fr)_minmax(0,1.2fr)_4.25rem_5.5rem_5rem_4.75rem] lg:grid-cols-[5rem_minmax(0,2fr)_minmax(0,1.2fr)_4.25rem_6.5rem_5rem_4.75rem]"
>
	<!-- Time -->
	<span
		class="mono-data text-muted-foreground whitespace-nowrap"
		title={formatDateTime(row.TimestampMs)}
	>
		{formatTimestamp(row.TimestampMs)}
	</span>

	<!-- Model & Provider -->
	<span class="min-w-0 pr-2">
		<span class="block truncate text-[13px] font-semibold text-ink" title={row.Model}>
			{row.Model}
		</span>
		<span
			class="block truncate text-[11.5px] text-muted-foreground"
			title={row.RouteProvider || row.Provider}
		>
			{row.RouteProvider || row.Provider}
		</span>
	</span>

	<!-- Client Key & Origin -->
	<span class="hidden min-w-0 pr-2 md:block">
		<span class="mono-data block truncate text-[12.5px] text-ink" title={row.ClientKeyName || ''}>
			{row.ClientKeyName || '—'}
		</span>
		<span class="block truncate text-[11px] text-muted-foreground">
			{row.Origin === 'internal'
				? $t('ui.pages.logsPage.originInternal')
				: $t('ui.pages.logsPage.originExternal')}
		</span>
	</span>

	<!-- Status -->
	<span class="flex items-center justify-start gap-1">
		<span
			class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11.5px] font-medium {row.Status >=
			400
				? 'bg-danger/10 text-danger'
				: 'bg-ok/10 text-ok'}"
		>
			<Icon
				name={row.Status >= 400 ? 'circle-x' : 'circle-check'}
				size={12}
				class={row.Status >= 400 ? 'text-danger' : 'text-ok'}
			/>
			<span class="mono-data font-semibold">{row.Status}</span>
		</span>
	</span>

	<!-- Tokens + visual bar -->
	<span class="hidden min-w-0 flex-col sm:flex">
		<span class="mono-data text-[12.5px] tabular-nums text-ink">
			{formatTokens(totalTokens)}
		</span>
		{#if maxTokens > 0}
			<span class="mt-1 block h-1 w-full overflow-hidden rounded-full bg-sunken" aria-hidden="true">
				<span class="block h-full rounded-full bg-accent-300" style="width: {tokenBarPercent}%"
				></span>
			</span>
		{/if}
	</span>

	<!-- Duration -->
	<span
		class="mono-data hidden text-right text-[12.5px] tabular-nums sm:block {isSlow
			? 'font-semibold text-warn'
			: 'text-muted-foreground'}"
		title={isSlow ? 'Slow request (>2m)' : undefined}
	>
		{formatDuration(row.DurationMs)}
	</span>

	<!-- Cost -->
	<span class="mono-data text-right text-[12.5px] tabular-nums text-ink">
		{formatCost(row.EstimatedCostMicros)}
	</span>
</button>
