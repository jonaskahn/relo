<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import FilterPanel from '$lib/components/ui/filter-panel.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProcessLine from '$lib/components/logs/process-line.svelte';
	import { FILTER_PANEL_KEYS } from '$lib/filter-panel';
	import {
		buildDaemonQuery,
		isDaemonLevel,
		mergeDaemonLines,
		type DaemonLevel,
		type DaemonLine
	} from '$lib/process-logs';

	// DaemonSection is the daemon's own log, newest first, with a live tail.
	// Requests hide by default: the access lines of every poll would bury the
	// events the section exists for. It mounts only while its section shows,
	// so loading runs on mount and polling follows the live control. The page
	// owns the toolbar row, so the filters and the live switch arrive bound
	// and only the fields grid renders here.

	const PAGE_SIZE = 200;

	interface Props {
		live?: boolean;
		level?: DaemonLevel;
		onlevelchange?: (next: DaemonLevel) => void;
		hideRequests?: boolean;
		onhiderequestschange?: (next: boolean) => void;
		filtersOpen?: boolean;
	}

	let {
		live = true,
		level = 'all',
		onlevelchange,
		hideRequests = true,
		onhiderequestschange,
		filtersOpen = $bindable(false)
	}: Props = $props();

	let lines = $state.raw<DaemonLine[]>([]);
	let nextCursor = $state('');
	let end = $state('');
	let loading = $state(true);
	let loadingMore = $state(false);
	let failure = $state('');
	let polling = $state(false);

	async function load(reset = true) {
		if (reset) {
			loading = true;
			nextCursor = '';
		} else {
			loadingMore = true;
		}
		failure = '';
		try {
			const data = await api<{ items: DaemonLine[]; next_cursor?: string; end: string }>(
				'/logs/daemon?' +
					buildDaemonQuery({
						limit: PAGE_SIZE,
						before: reset ? '' : nextCursor,
						level,
						hideRequests
					})
			);
			if (reset) {
				lines = data.items ?? [];
				end = data.end;
			} else {
				lines = mergeDaemonLines(lines, data.items ?? [], 5000);
			}
			nextCursor = data.next_cursor ?? '';
			if (!reset) end = data.end || end;
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
			loadingMore = false;
		}
	}

	async function poll() {
		if (polling || !end) return;
		polling = true;
		try {
			const data = await api<{ items: DaemonLine[]; end: string }>(
				'/logs/daemon?' + buildDaemonQuery({ limit: PAGE_SIZE, after: end, level, hideRequests })
			);
			if ((data.items?.length ?? 0) > 0) {
				lines = mergeDaemonLines(lines, data.items, 5000);
			}
			end = data.end || end;
		} catch {
			// A failed poll stays silent: the next one retries, and the lines
			// on screen stay readable instead of flashing an error.
		} finally {
			polling = false;
		}
	}

	// The section mounts only while it shows, so the first read runs on mount
	// and the tail polls while the live control runs.
	onMount(() => {
		void load(true);
	});

	// A filter the page toolbar changed re-reads from the start. The mount
	// read above covers the first run, so it is skipped here. The flag is
	// deliberately not reactive: it must not trigger the effect it guards.
	let firstFilterRun = true;
	$effect(() => {
		// The first run returns before load(), so it must still read the
		// filters: an effect subscribes only to what a run reads, and a run
		// that reads nothing never re-runs.
		buildDaemonQuery({ limit: PAGE_SIZE, level, hideRequests });
		if (firstFilterRun) {
			firstFilterRun = false;
			return;
		}
		void load(true);
	});

	$effect(() => {
		if (!live) return;
		const timer = setInterval(() => void poll(), 2000);
		return () => clearInterval(timer);
	});
</script>

<div class="flex flex-col gap-4">
	<FilterPanel storageKey={FILTER_PANEL_KEYS.logsDaemon} columns={3} bind:open={filtersOpen}>
		<Field id="daemon-level" label={$t('ui.pages.logsPage.filterLevel')}>
			<NativeSelect
				id="daemon-level"
				value={level}
				onchange={(event) => {
					const { value } = event.currentTarget;
					if (isDaemonLevel(value)) onlevelchange?.(value);
				}}
			>
				<option value="all">{$t('ui.pages.logsPage.levelAll')}</option>
				<option value="warning">{$t('ui.pages.logsPage.levelWarnings')}</option>
				<option value="error">{$t('ui.pages.logsPage.levelErrors')}</option>
			</NativeSelect>
		</Field>
		<Field id="daemon-requests" label={$t('ui.pages.logsPage.requestsLabel')}>
			<NativeSelect
				id="daemon-requests"
				value={hideRequests ? 'hidden' : 'shown'}
				onchange={(event) => {
					onhiderequestschange?.(event.currentTarget.value !== 'shown');
				}}
			>
				<option value="hidden">{$t('ui.pages.logsPage.requestsHidden')}</option>
				<option value="shown">{$t('ui.pages.logsPage.requestsShown')}</option>
			</NativeSelect>
		</Field>
	</FilterPanel>

	{#if loading}
		<p class="text-sm text-muted-foreground">{$t('ui.common.loading')}</p>
	{:else if failure}
		<div
			class="flex items-center gap-3 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2"
			role="alert"
		>
			<p class="flex-1 text-sm text-destructive">{failure}</p>
			<Button variant="outline" size="sm" onclick={() => void load(true)}
				>{$t('ui.common.retry')}</Button
			>
		</div>
	{:else if lines.length === 0}
		<div class="empty-panel section-surface border-dashed">
			{$t('ui.pages.logsPage.daemonEmpty')}
		</div>
	{:else}
		<div class="flex flex-col gap-1.5">
			{#each lines as line (line.offset)}
				<ProcessLine {line} />
			{/each}
		</div>
		{#if nextCursor}
			<div class="mt-4 text-center">
				<Button variant="outline" size="sm" onclick={() => void load(false)} disabled={loadingMore}>
					<Icon name={loadingMore ? 'loader' : 'arrow-down'} size={14} spin={loadingMore} />
					{loadingMore ? $t('ui.common.loading') : $t('ui.pages.logsPage.daemonLoadOlder')}
				</Button>
			</div>
		{/if}
	{/if}
</div>
