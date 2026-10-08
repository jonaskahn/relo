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
		onopen: (row: UsageLogRow) => void;
	}

	let { row, onopen }: Props = $props();
</script>

<button
	type="button"
	data-plain
	onclick={() => onopen(row)}
	class="card-press grid w-full grid-cols-[4.75rem_minmax(0,1fr)_3.5rem_4.75rem] items-center gap-x-3 rounded-panel border border-line bg-surface px-3.5 py-2 text-left hover:border-accent-border hover:bg-hover hover:shadow-[var(--shadow-hover)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring sm:grid-cols-[4.75rem_minmax(0,1fr)_3.5rem_4.75rem_4.75rem_4.75rem] md:grid-cols-[4.75rem_minmax(0,1fr)_6.5rem_3.5rem_4.75rem_4.75rem_4.75rem] lg:grid-cols-[4.75rem_minmax(0,1fr)_6.5rem_5rem_5.5rem_3.5rem_4.75rem_4.75rem_4.75rem]"
>
	<span class="mono-data text-muted-foreground" title={formatDateTime(row.TimestampMs)}
		>{formatTimestamp(row.TimestampMs)}</span
	>
	<span class="min-w-0 truncate text-[13px] font-medium text-ink" title={row.Model}
		>{row.Model}</span
	>
	<span
		class="mono-data hidden min-w-0 truncate text-muted-foreground md:block"
		title={row.RouteProvider || row.Provider}>{row.RouteProvider || row.Provider}</span
	>
	<span class="hidden truncate text-[13px] text-muted-foreground lg:block"
		>{row.Origin === 'internal'
			? $t('ui.pages.logsPage.originInternal')
			: $t('ui.pages.logsPage.originExternal')}</span
	>
	<span
		class="mono-data hidden min-w-0 truncate text-muted-foreground lg:block"
		title={row.ClientKeyName || ''}>{row.ClientKeyName || '—'}</span
	>
	<span class="flex items-center justify-end gap-1">
		<Icon
			name={row.Status >= 400 ? 'circle-x' : 'circle-check'}
			size={13}
			class={row.Status >= 400 ? 'text-danger' : 'text-ok'}
		/>
		<span class="mono-data {row.Status >= 400 ? 'text-danger' : 'text-muted-foreground'}"
			>{row.Status}</span
		>
	</span>
	<span class="mono-data hidden text-right tabular-nums text-muted-foreground sm:block"
		>{formatTokens(row.InputTokens + row.OutputTokens)}</span
	>
	<span class="mono-data hidden text-right tabular-nums text-muted-foreground sm:block"
		>{formatDuration(row.DurationMs)}</span
	>
	<span class="mono-data text-right tabular-nums text-ink"
		>{formatCost(row.EstimatedCostMicros)}</span
	>
</button>
