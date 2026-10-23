<script lang="ts">
	import { tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { slide } from 'svelte/transition';
	import { t } from 'svelte-i18n';

	import { prefersReducedMotion } from '$lib/tab-motion';
	import RequestRow from '$lib/components/logs/request-row.svelte';
	import { formatTokens } from '$lib/format';
	import { groupRequestsByHour } from '$lib/process-logs';
	import type { UsageLogRow } from '$lib/types';

	// RequestList is one bordered panel of requests: a shared header, hour
	// bands, and one hairline row per request. The panel is the border; the
	// rows inside it do not each grow their own.

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

	// The sentinel watches the foot of the list. It re-arms after every page
	// (loadingMore and the row count change), so a sentinel still in view asks
	// for the next page until the cursor runs out. A failed page sets no new
	// rows, so it does not loop; Retry lives with the error.
	function watchMore(node: HTMLElement) {
		$effect(() => {
			void logs.length;
			if (loadingMore) return;
			const observer = new IntersectionObserver((entries) => {
				if (entries.some((entry) => entry.isIntersecting)) onloadMore();
			});
			observer.observe(node);
			return () => observer.disconnect();
		});
	}

	const groups = $derived(groupRequestsByHour(logs));
	// The first page paints still. A row that arrives afterwards uses the
	// shared slide; flip eases the rows it pushes down. Reduced motion skips both.
	let settled = $state(false);
	const motion = $derived(!prefersReducedMotion());

	$effect(() => {
		if (logs.length === 0 || settled) return;
		void tick().then(() => {
			settled = true;
		});
	});

	const cols =
		'grid-cols-[6.5rem_minmax(0,2fr)_auto_5.5rem_5rem] sm:grid-cols-[6.5rem_minmax(0,2fr)_minmax(0,1fr)_auto_6rem_5rem_4.5rem]';
</script>

<div
	class="overflow-hidden rounded-card border border-line bg-surface max-sm:border-transparent max-sm:bg-transparent"
	role="table"
	aria-label={$t('ui.pages.logsPage.tabAgent')}
>
	<div
		class="hidden h-10 items-center gap-3.5 bg-canvas px-5 text-xs font-medium text-muted-foreground sm:grid {cols}"
		role="row"
	>
		<span>{$t('ui.pages.logsPage.columnTime')}</span>
		<span>{$t('ui.pages.logsPage.columnModel')}</span>
		<span class="hidden sm:block">{$t('ui.pages.logsPage.columnClient')}</span>
		<span>{$t('ui.pages.logsPage.columnStatus')}</span>
		<span class="hidden sm:block">{$t('ui.pages.logsPage.columnTokens')}</span>
		<span class="hidden text-right sm:block">{$t('ui.pages.logsPage.columnDuration')}</span>
		<span class="text-right">{$t('ui.pages.logsPage.columnCost')}</span>
	</div>

	{#each groups as group (group.key)}
		<div
			class="flex items-center justify-between bg-canvas px-5 py-1.5 text-xs font-medium text-muted-foreground max-sm:bg-transparent max-sm:px-0.5"
		>
			<span>{group.hourLabel}</span>
			<span class="font-normal">
				{group.requests.length}
				{$t('ui.pages.usagePage.unitRequests')} · {formatTokens(group.totalTokens)}
				{$t('ui.pages.usagePage.unitTokens')}
			</span>
		</div>
		{#each group.requests as row (row.ID)}
			<div animate:flip={{ duration: motion && settled ? 200 : 0 }}>
				{#if !settled || !motion}
					<RequestRow {row} {maxTokens} {cols} {onopen} />
				{:else}
					<div in:slide={{ duration: 200 }}>
						<RequestRow {row} {maxTokens} {cols} {onopen} />
					</div>
				{/if}
			</div>
		{/each}
	{/each}

	{#if nextCursor}
		<div
			class="flex items-center justify-center px-5 py-4 text-xs text-muted-foreground"
			{@attach watchMore}
		>
			{loadingMore ? $t('ui.common.loading') : ''}
		</div>
	{/if}
</div>
