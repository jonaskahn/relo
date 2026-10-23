<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { page } from '$app/state';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import RequestList from '$lib/components/logs/request-list.svelte';
	import { coalesceReload } from '$lib/refresh';
	import { subscribe } from '$lib/sse';
	import { formatDuration, formatTimestamp, formatTokens } from '$lib/format';
	import { buildRequestQuery, computeRequestKpis, type RequestFilters } from '$lib/process-logs';
	import type { UsageLogRow } from '$lib/types';

	// AgentSection is the request log agents write to. The page owns the
	// toolbar, so status, search and the live switch arrive from it. The
	// section mounts only while it shows, so a visit back reads fresh.

	interface Props {
		status: RequestFilters['status'];
		query?: string;
		live?: boolean;
		onopen: (row: UsageLogRow) => void;
		onrows?: (rows: UsageLogRow[]) => void;
	}

	let { status, query = '', live = true, onopen, onrows }: Props = $props();

	const filters = $derived<RequestFilters>({
		provider: '',
		status,
		origin: 'any',
		key: '',
		client: ''
	});

	let logs = $state<UsageLogRow[]>([]);
	let nextCursor = $state('');
	let loading = $state(true);
	let loadingMore = $state(false);
	let failure = $state('');

	const visible = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return logs;
		return logs.filter((row) =>
			[
				row.Model,
				row.Provider,
				row.RouteProvider,
				row.ClientKeyName,
				row.Origin,
				formatTimestamp(row.TimestampMs)
			]
				.join(' ')
				.toLowerCase()
				.includes(needle)
		);
	});
	const kpis = $derived(computeRequestKpis(visible));

	$effect(() => {
		onrows?.(visible);
	});

	async function load(reset = true) {
		if (!reset && (loadingMore || !nextCursor)) return;
		if (reset) {
			// Rows already on screen stay put while a filter reload runs, so
			// the list animates instead of blanking.
			loading = logs.length === 0;
			nextCursor = '';
		} else {
			loadingMore = true;
		}
		failure = '';
		try {
			const data = await api<{ items: UsageLogRow[]; next_cursor?: string }>(
				'/activity/requests?' + buildRequestQuery(filters, reset ? '' : nextCursor)
			);
			if (reset) {
				logs = data.items;
			} else {
				// A cursor boundary must not render one request twice.
				const known = new Set(logs.map((row) => row.ID));
				logs = [...logs, ...data.items.filter((row) => !known.has(row.ID))];
			}
			nextCursor = data.next_cursor ?? '';
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	onMount(async () => {
		await load(true);
		// A dashboard row links straight to its own request, so the detail
		// opens once the first page has arrived. A request the log does not
		// hold (a filter moved the window) leaves it as it loaded.
		const wanted = page.url.searchParams.get('request');
		if (wanted) {
			const row = logs.find((entry) => String(entry.ID) === wanted || entry.RequestID === wanted);
			if (row) onopen(row);
		}
	});

	// A filter the page changed re-reads from the start. The mount read above
	// covers the first run, so it is skipped here. The flag is deliberately
	// not reactive: it must not trigger the effect it guards.
	let firstFilterRun = true;
	$effect(() => {
		// The first run returns before load(), so it must still read the
		// filters: an effect subscribes only to what a run reads, and a run
		// that reads nothing never re-runs.
		buildRequestQuery(filters);
		if (firstFilterRun) {
			firstFilterRun = false;
			return;
		}
		void load(true);
	});

	// The live tail reloads on the event stream; a burst of announcements
	// reloads once rather than once per relayed request. The pause control
	// stops the subscription without closing the page.
	const reloadLogs = coalesceReload(() => load(true));

	$effect(() => {
		if (!live) return;
		// A visit back to the request log catches up before it listens.
		reloadLogs.schedule();
		return subscribe('logs', () => reloadLogs.schedule());
	});

	onDestroy(() => reloadLogs.cancel());
</script>

<div class="flex flex-col gap-3">
	{#if !loading && !failure && logs.length > 0}
		<div class="grid grid-cols-2 gap-[var(--grid-gap)] xl:grid-cols-4">
			<div class="section-surface min-w-0 px-3.5 py-2">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.logsPage.requestsLoaded')}
				</p>
				<p class="mono-data text-[1.1875rem] font-medium tabular-nums">{kpis.total}</p>
			</div>
			<div class="section-surface min-w-0 px-3.5 py-2">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.dashboard.successTile')}
				</p>
				<p class="mono-data text-[1.1875rem] font-medium tabular-nums">
					{kpis.successRate}%
					<span class="ms-1 text-xs font-normal text-muted-foreground">
						{kpis.successCount}/{kpis.total}
					</span>
				</p>
			</div>
			<div class="section-surface min-w-0 px-3.5 py-2">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.usagePage.summaryTokens')}
				</p>
				<p class="mono-data text-[1.1875rem] font-medium tabular-nums">
					{formatTokens(kpis.totalTokens)}
				</p>
			</div>
			<div class="section-surface min-w-0 px-3.5 py-2">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.logsPage.avgSlowest')}
				</p>
				<p class="mono-data text-[1.1875rem] font-medium tabular-nums">
					{formatDuration(kpis.avgDurationMs)}
					<span class="ms-1 text-xs font-normal text-muted-foreground">
						· {formatDuration(kpis.slowestDurationMs)}
					</span>
				</p>
			</div>
		</div>
	{/if}

	{#if loading}
		<p class="text-muted-foreground text-sm">{$t('ui.common.loading')}</p>
	{:else if failure && logs.length === 0}
		<p
			class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
			role="alert"
		>
			{failure}
		</p>
	{:else if visible.length === 0}
		<div class="empty-panel section-surface border-dashed">
			{$t('ui.pages.logsPage.empty')}
		</div>
	{:else}
		<RequestList
			logs={visible}
			maxTokens={kpis.maxTokens}
			{nextCursor}
			{loadingMore}
			onloadMore={() => {
				if (!failure) void load(false);
			}}
			{onopen}
		/>
		{#if failure}
			<div
				class="flex items-center gap-3 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2"
				role="alert"
			>
				<p class="flex-1 text-sm text-destructive">{failure}</p>
				<Button variant="outline" size="sm" onclick={() => void load(false)}>
					{$t('ui.common.retry')}
				</Button>
			</div>
		{/if}
	{/if}
</div>
