<script lang="ts">
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import RequestRow from '$lib/components/logs/request-row.svelte';
	import { formatTokens } from '$lib/format';
	import { groupRequestsByHour } from '$lib/process-logs';
	import type { UsageLogRow } from '$lib/types';

	// RequestList is the request log's rows: a transparent header naming the
	// columns, requests grouped by time interval (hour batches), one dense
	// row-card per request with visual token proportion bars, and the page
	// continuation from the oldest one shown.

	interface Props {
		logs: UsageLogRow[];
		maxTokens?: number;
		nextCursor?: string;
		loadingMore?: boolean;
		onloadMore: () => void;
		onopen: (row: UsageLogRow) => void;
	}

	let {
		logs,
		maxTokens = 0,
		nextCursor = '',
		loadingMore = false,
		onloadMore,
		onopen
	}: Props = $props();

	const groups = $derived(groupRequestsByHour(logs));
</script>

<div class="flex flex-col gap-2">
	<!-- Transparent table header -->
	<div
		class="grid grid-cols-[5rem_minmax(0,1.8fr)_4.25rem_5rem] items-center gap-x-3 px-3.5 py-1 text-xs font-medium text-muted-foreground sm:grid-cols-[5rem_minmax(0,1.8fr)_4.25rem_5rem_5.5rem_4.75rem] md:grid-cols-[5rem_minmax(0,2fr)_minmax(0,1.2fr)_4.25rem_5.5rem_5rem_4.75rem] lg:grid-cols-[5rem_minmax(0,2fr)_minmax(0,1.2fr)_4.25rem_6.5rem_5rem_4.75rem]"
	>
		<span>{$t('ui.pages.logsPage.columnTime')}</span>
		<span>{$t('ui.pages.logsPage.columnModel')}</span>
		<span class="hidden md:block">{$t('ui.pages.logsPage.columnClient')}</span>
		<span>{$t('ui.pages.logsPage.columnStatus')}</span>
		<span class="hidden sm:block">{$t('ui.pages.logsPage.columnTokens')}</span>
		<span class="hidden text-right sm:block">{$t('ui.pages.logsPage.columnDuration')}</span>
		<span class="text-right">{$t('ui.pages.logsPage.columnCost')}</span>
	</div>

	<!-- Requests grouped by hour interval -->
	{#each groups as group (group.key)}
		<div
			class="flex items-center justify-between px-3 py-1 text-xs font-medium text-muted-foreground"
		>
			<span class="font-semibold text-ink">{group.hourLabel}</span>
			<span class="mono-data text-[11.5px] opacity-80">
				{group.requests.length}
				{$t('ui.pages.usagePage.unitRequests')} · {formatTokens(group.totalTokens)}
				{$t('ui.pages.usagePage.unitTokens')}
			</span>
		</div>
		<div class="flex flex-col gap-1.5">
			{#each group.requests as row (row.ID)}
				<RequestRow {row} {maxTokens} {onopen} />
			{/each}
		</div>
	{/each}
</div>

{#if nextCursor}
	<div class="mt-4 text-center">
		<Button variant="outline" size="sm" onclick={onloadMore} disabled={loadingMore}>
			<Icon name={loadingMore ? 'loader' : 'arrow-down'} size={14} spin={loadingMore} />
			{loadingMore ? $t('ui.common.loading') : $t('ui.pages.logsPage.loadMore')}
		</Button>
	</div>
{/if}
