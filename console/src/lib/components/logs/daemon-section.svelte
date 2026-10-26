<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import FilterPanel from '$lib/components/ui/filter-panel.svelte';
	import Select from '$lib/components/ui/select.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProcessLine from '$lib/components/logs/process-line.svelte';
	import { FILTER_PANEL_KEYS } from '$lib/filter-panel';
	import { formatDateTime } from '$lib/format';
	import {
		buildDaemonQuery,
		groupDaemonLines,
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

	function watchMore(node: HTMLElement, load: () => void) {
		let pending = false;
		const observer = new IntersectionObserver((entries) => {
			if (pending || !entries.some((entry) => entry.isIntersecting)) return;
			pending = true;
			load();
		});
		observer.observe(node);
		return () => observer.disconnect();
	}

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
	let openRuns = $state<Record<string, boolean>>({});
	const groups = $derived(groupDaemonLines(lines));
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
	<div class="max-sm:hidden">
		<FilterPanel storageKey={FILTER_PANEL_KEYS.logsDaemon} columns={3} bind:open={filtersOpen}>
			<Field id="daemon-level" label={$t('ui.pages.logsPage.filterLevel')}>
				<Select
					id="daemon-level"
					value={level}
					onValueChange={(next) => {
						if (isDaemonLevel(next)) onlevelchange?.(next);
					}}
					options={[
						{ value: 'all', label: $t('ui.pages.logsPage.levelAll') },
						{ value: 'warning', label: $t('ui.pages.logsPage.levelWarnings') },
						{ value: 'error', label: $t('ui.pages.logsPage.levelErrors') }
					]}
				/>
			</Field>
			<Field id="daemon-requests" label={$t('ui.pages.logsPage.requestsLabel')}>
				<Select
					id="daemon-requests"
					value={hideRequests ? 'hidden' : 'shown'}
					onValueChange={(next) => onhiderequestschange?.(next !== 'shown')}
					options={[
						{ value: 'hidden', label: $t('ui.pages.logsPage.requestsHidden') },
						{ value: 'shown', label: $t('ui.pages.logsPage.requestsShown') }
					]}
				/>
			</Field>
		</FilterPanel>
	</div>

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
		<div class="overflow-hidden rounded-card bg-surface shadow-float-1">
			{#each groups as group (group.key)}
				{#if group.count === 1}
					<ProcessLine line={group.lines[0]} plain />
				{:else}
					<div>
						<button
							type="button"
							class="flex w-full items-center gap-2.5 px-5 py-2.5 text-left hover:bg-hover"
							aria-expanded={openRuns[group.key] ?? false}
							onclick={() => (openRuns[group.key] = !openRuns[group.key])}
						>
							<Icon
								name="chevron-right"
								size={12}
								class="shrink-0 text-muted-foreground transition-transform duration-200 {openRuns[
									group.key
								]
									? 'rotate-90'
									: ''}"
							/>
							<span class="mono-data shrink-0 text-muted-foreground">
								{group.latestMs ? formatDateTime(group.latestMs) : '—'}
							</span>
							<span class="min-w-0 flex-1 truncate text-sm text-ink" title={group.message}>
								{group.message || '—'}
							</span>
							<span
								class="rounded-full bg-sunken px-2 py-0.5 text-xs font-medium text-muted-foreground"
							>
								×{group.count}
							</span>
						</button>
						{#if openRuns[group.key]}
							<div class="flex flex-col gap-1.5 px-5 pb-2">
								<p class="text-xs text-muted-foreground">
									{formatDateTime(group.firstMs)} – {formatDateTime(group.latestMs)}
								</p>
								{#each group.lines as line (line.offset)}
									<ProcessLine {line} plain />
								{/each}
							</div>
						{/if}
					</div>
				{/if}
			{/each}
		</div>
		{#if nextCursor}
			<div
				class="py-4 text-center text-xs text-muted-foreground"
				{@attach (node) => watchMore(node, () => void load(false))}
			>
				{loadingMore ? $t('ui.common.loading') : ''}
			</div>
		{/if}
	{/if}
</div>
