<script lang="ts">
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import RequestRow from '$lib/components/logs/request-row.svelte';
	import type { UsageLogRow } from '$lib/types';

	// RequestList is the request log's rows: a transparent header naming the
	// columns, one dense row-card per request, and the page that continues
	// from the oldest one shown. Secondary columns hide below wide screens
	// instead of scrolling sideways; identity, status and cost survive every
	// width.

	interface Props {
		logs: UsageLogRow[];
		nextCursor?: string;
		loadingMore?: boolean;
		onloadMore: () => void;
		onopen: (row: UsageLogRow) => void;
	}

	let { logs, nextCursor = '', loadingMore = false, onloadMore, onopen }: Props = $props();
</script>

<div class="flex flex-col gap-2">
	<div
		class="grid grid-cols-[4.75rem_minmax(0,1fr)_3.5rem_4.75rem] items-center gap-x-3 px-3.5 text-xs font-medium text-muted-foreground sm:grid-cols-[4.75rem_minmax(0,1fr)_3.5rem_4.75rem_4.75rem_4.75rem] md:grid-cols-[4.75rem_minmax(0,1fr)_6.5rem_3.5rem_4.75rem_4.75rem_4.75rem] lg:grid-cols-[4.75rem_minmax(0,1fr)_6.5rem_5rem_5.5rem_3.5rem_4.75rem_4.75rem_4.75rem]"
	>
		<span>{$t('ui.pages.logsPage.columnTime')}</span>
		<span>{$t('ui.pages.logsPage.columnModel')}</span>
		<span class="hidden md:block">{$t('ui.pages.logsPage.columnProvider')}</span>
		<span class="hidden lg:block">{$t('ui.pages.logsPage.columnOrigin')}</span>
		<span class="hidden lg:block">{$t('ui.pages.logsPage.columnClient')}</span>
		<span class="text-right">{$t('ui.pages.logsPage.columnStatus')}</span>
		<span class="hidden text-right sm:block">{$t('ui.pages.logsPage.columnTokens')}</span>
		<span class="hidden text-right sm:block">{$t('ui.pages.logsPage.columnDuration')}</span>
		<span class="text-right">{$t('ui.pages.logsPage.columnCost')}</span>
	</div>
	{#each logs as row (row.ID)}
		<RequestRow {row} {onopen} />
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
