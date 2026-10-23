<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { page } from '$app/state';

	import { api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import RequestFilterPanel from '$lib/components/logs/request-filter-panel.svelte';
	import RequestList from '$lib/components/logs/request-list.svelte';
	import { coalesceReload } from '$lib/refresh';
	import { subscribe } from '$lib/sse';
	import { formatDuration, formatTokens } from '$lib/format';
	import { buildRequestQuery, computeRequestKpis, type RequestFilters } from '$lib/process-logs';
	import type { AccessKey, Provider, UsageLogRow } from '$lib/types';

	// AgentSection is the request log agents write to. The page owns the
	// toolbar row, so the filter values and the live switch arrive from it and
	// only the fields grid renders here. The section mounts only while it
	// shows, so a visit back reads fresh.

	interface Props {
		filters: RequestFilters;
		onfilters: (patch: Partial<RequestFilters>) => void;
		live?: boolean;
		filtersOpen?: boolean;
		onopen: (row: UsageLogRow) => void;
	}

	let { filters, onfilters, live = true, filtersOpen = $bindable(false), onopen }: Props = $props();

	let logs = $state<UsageLogRow[]>([]);
	let nextCursor = $state('');
	let loading = $state(true);
	let loadingMore = $state(false);
	let failure = $state('');
	let providers = $state<Provider[]>([]);
	let keys = $state<AccessKey[]>([]);
	let query = $state('');

	const visible = $derived.by(() => {
		const needle = query.trim().toLowerCase();
		if (!needle) return logs;
		return logs.filter((row) =>
			[row.Model, row.Provider, row.RouteProvider, row.ClientKeyName, row.Origin]
				.join(' ')
				.toLowerCase()
				.includes(needle)
		);
	});
	const kpis = $derived(computeRequestKpis(visible));

	async function load(reset = true) {
		if (reset) {
			loading = true;
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
				logs = [...logs, ...data.items];
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
		// The filter lists are a convenience; their failure is not fatal.
		try {
			providers = (await api<{ items: Provider[] }>('/connections')).items;
		} catch {
			providers = [];
		}
		try {
			keys = (await api<{ items: AccessKey[] }>('/clients/keys')).items ?? [];
		} catch {
			keys = [];
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

<div class="flex flex-col gap-4">
	<RequestFilterPanel {filters} {onfilters} {providers} {keys} bind:open={filtersOpen} />

	<div class="relative max-w-md">
		<span
			class="pointer-events-none absolute inset-y-0 start-3 flex items-center text-muted-foreground"
		>
			<Icon name="search" size={15} />
		</span>
		<Input
			class="ps-9"
			type="search"
			bind:value={query}
			placeholder={$t('ui.pages.logsPage.searchPlaceholder')}
			aria-label={$t('ui.pages.logsPage.searchPlaceholder')}
		/>
	</div>

	{#if !loading && !failure && logs.length > 0}
		<div class="grid grid-cols-2 gap-[var(--grid-gap)] xl:grid-cols-4">
			<div class="section-surface min-w-0 p-4">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.logsPage.requestsLoaded')}
				</p>
				<p class="mono-data mt-1 text-[1.375rem] font-medium tabular-nums">{kpis.total}</p>
			</div>
			<div class="section-surface min-w-0 p-4">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.dashboard.successTile')}
				</p>
				<p class="mono-data mt-1 text-[1.375rem] font-medium tabular-nums text-ok">
					{kpis.successRate}%
				</p>
			</div>
			<div class="section-surface min-w-0 p-4">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.usagePage.summaryTokens')}
				</p>
				<p class="mono-data mt-1 text-[1.375rem] font-medium tabular-nums">
					{formatTokens(kpis.totalTokens)}
				</p>
			</div>
			<div class="section-surface min-w-0 p-4">
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t('ui.pages.logsPage.avgSlowest')}
				</p>
				<p class="mono-data mt-1 text-[1.375rem] font-medium tabular-nums">
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
	{:else if failure}
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
			onloadMore={() => load(false)}
			{onopen}
		/>
	{/if}
</div>
