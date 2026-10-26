<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import { CLIENT_OPTIONS } from '$lib/client-setup';
	import {
		formatCost,
		formatDateTime,
		formatDuration,
		formatTimestamp,
		formatTokens
	} from '$lib/format';
	import type { UsageLogRow } from '$lib/types';

	// RequestRow is one request inside the log panel. Wide screens read it as a
	// hairline row; a phone reads the same fields as a card. Status pairs its
	// icon with the code, never colour alone.

	interface Props {
		row: UsageLogRow;
		maxTokens?: number;
		cols: string;
		onopen: (row: UsageLogRow) => void;
	}

	let { row, maxTokens = 0, cols, onopen }: Props = $props();

	const totalTokens = $derived(Number(row.InputTokens || 0) + Number(row.OutputTokens || 0));
	const tokenBarPercent = $derived(
		maxTokens > 0 ? Math.max(2, Math.min(100, (totalTokens / maxTokens) * 100)) : 0
	);
	const isSlow = $derived(row.DurationMs >= 120_000);
	const failed = $derived(row.Status >= 400);
	const client = $derived(CLIENT_OPTIONS.find((option) => option.id === row.ClientApp));
	const providerId = $derived(row.RouteProvider || row.Provider);

	const originLabel = $derived(
		row.Origin === 'internal'
			? $t('ui.pages.logsPage.originInternal')
			: $t('ui.pages.logsPage.originExternal')
	);
</script>

<button
	type="button"
	data-plain
	data-request-row={row.ID}
	onclick={() => onopen(row)}
	aria-label={$t('ui.common.openCard', {
		values: { name: row.Model + ' ' + formatTimestamp(row.TimestampMs) }
	})}
	class="grid w-full items-center gap-x-3.5 px-5 py-2.5 text-left hover:bg-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring max-sm:mx-0 max-sm:mb-2.5 max-sm:grid-cols-[minmax(0,1fr)_auto] max-sm:gap-y-1 max-sm:rounded-card max-sm:bg-surface max-sm:px-4 max-sm:py-3.5 max-sm:shadow-float-1 sm:min-h-14 {cols}"
>
	<span
		class="mono-data order-3 text-xs text-muted-foreground max-sm:self-center sm:order-none sm:text-[0.8125rem]"
		title={formatDateTime(row.TimestampMs)}
	>
		{formatTimestamp(row.TimestampMs)}
	</span>

	<span class="flex min-w-0 items-center gap-2 max-sm:col-start-1">
		{#if providerId}
			<ProviderLogo id={providerId} label={providerId} size="xs" />
		{/if}
		<span class="min-w-0">
			<span class="block truncate text-[0.8125rem] font-semibold text-ink" title={row.Model}
				>{row.Model}</span
			>
			<span class="block truncate text-xs text-muted-foreground" title={providerId}
				>{providerId}</span
			>
		</span>
	</span>

	<span class="hidden min-w-0 items-center gap-2 sm:flex">
		{#if client?.logoId}
			<ProviderLogo id={client.logoId} label={client.label} size="xs" />
		{/if}
		<span class="min-w-0">
			<span class="block truncate text-[0.8125rem]" title={row.ClientKeyName || ''}
				>{row.ClientKeyName || '—'}</span
			>
			<span class="block truncate text-xs text-muted-foreground"
				>{client?.label ?? originLabel}</span
			>
		</span>
	</span>

	<span class="max-sm:col-start-2 max-sm:row-start-1 max-sm:justify-self-end">
		<span
			class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold {failed
				? 'bg-danger/10 text-danger'
				: 'bg-ok/10 text-ok'}"
		>
			<Icon name={failed ? 'circle-x' : 'circle-check'} size={13} />
			<span class="mono-data">{row.Status}</span>
		</span>
	</span>

	<span class="hidden min-w-0 sm:flex sm:flex-col">
		<span class="mono-data text-[0.8125rem] tabular-nums">{formatTokens(totalTokens)}</span>
		{#if maxTokens > 0}
			<span
				class="mt-1 block h-[3px] w-full overflow-hidden rounded-full bg-sunken"
				aria-hidden="true"
			>
				<span class="block h-full rounded-full bg-accent-200" style="width: {tokenBarPercent}%"
				></span>
			</span>
		{/if}
	</span>

	<span class="order-4 flex items-center justify-end gap-3 sm:order-none sm:block">
		<span class="mono-data text-xs text-muted-foreground sm:hidden"
			>{formatTokens(totalTokens)} {$t('ui.pages.usagePage.unitTokens')}</span
		>
		<span
			class="mono-data text-[0.8125rem] tabular-nums sm:block sm:text-right {isSlow
				? 'font-semibold text-warn'
				: 'text-muted-foreground'}"
			title={isSlow ? $t('ui.pages.logsPage.slowRequest') : undefined}
		>
			{formatDuration(row.DurationMs)}
		</span>
	</span>

	<span
		class="mono-data hidden text-right text-[0.8125rem] tabular-nums text-muted-foreground sm:block"
	>
		{formatCost(row.EstimatedCostMicros)}
	</span>
</button>
