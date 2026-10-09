<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { page } from '$app/state';

	import { api } from '$lib/api';
	import RequestFilterPanel from '$lib/components/logs/request-filter-panel.svelte';
	import RequestList from '$lib/components/logs/request-list.svelte';
	import { coalesceReload } from '$lib/refresh';
	import { subscribe } from '$lib/sse';
	import { buildRequestQuery, type RequestFilters } from '$lib/process-logs';
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

	{#if loading}
		<p class="text-muted-foreground text-sm">{$t('ui.common.loading')}</p>
	{:else if failure}
		<p
			class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
			role="alert"
		>
			{failure}
		</p>
	{:else if logs.length === 0}
		<div class="empty-panel section-surface border-dashed">
			{$t('ui.pages.logsPage.empty')}
		</div>
	{:else}
		<RequestList {logs} {nextCursor} {loadingMore} onloadMore={() => load(false)} {onopen} />
	{/if}
</div>
