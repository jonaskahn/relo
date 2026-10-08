<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProcessLine from '$lib/components/logs/process-line.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import { formatDateTime } from '$lib/format';
	import { startupOutcomeTone, type DaemonLine, type StartupEntry } from '$lib/process-logs';

	// StartupSection is one row per boot transcript, newest first. A row opens
	// its transcript in place; the transcript loads once, on first open. It
	// mounts only while its section shows, so the list reads on mount. The
	// page owns the toolbar row, so refresh is exposed for its button.

	interface Props {
		loading?: boolean;
		onloadingchange?: (next: boolean) => void;
	}

	let { loading = true, onloadingchange }: Props = $props();

	let entries = $state<StartupEntry[]>([]);
	let failure = $state('');
	let open = $state<Record<string, boolean>>({});
	let transcripts = $state<
		Record<string, { lines: DaemonLine[]; loading: boolean; failure: string }>
	>({});

	function startedMs(entry: StartupEntry): number {
		const parsed = Date.parse(entry.started_at);
		return Number.isNaN(parsed) ? 0 : parsed;
	}

	async function load() {
		onloadingchange?.(true);
		failure = '';
		try {
			const data = await api<{ items: StartupEntry[] }>('/logs/startups');
			entries = data.items ?? [];
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			onloadingchange?.(false);
		}
	}

	async function openTranscript(entry: StartupEntry, willOpen: boolean) {
		open = { ...open, [entry.name]: willOpen };
		if (!willOpen || transcripts[entry.name]) return;
		transcripts = { ...transcripts, [entry.name]: { lines: [], loading: true, failure: '' } };
		try {
			const data = await api<{ lines: DaemonLine[] }>(
				'/logs/startups/' + encodeURIComponent(entry.name)
			);
			transcripts = {
				...transcripts,
				[entry.name]: { lines: data.lines ?? [], loading: false, failure: '' }
			};
		} catch (error) {
			transcripts = {
				...transcripts,
				[entry.name]: {
					lines: [],
					loading: false,
					failure: error instanceof Error ? error.message : String(error)
				}
			};
		}
	}

	onMount(() => {
		void load();
	});

	// refresh re-reads the list for the page toolbar's button.
	export function refresh() {
		void load();
	}
</script>

<div class="flex flex-col gap-4">
	{#if loading && entries.length === 0}
		<p class="text-sm text-muted-foreground">{$t('ui.common.loading')}</p>
	{:else if failure && entries.length === 0}
		<div
			class="flex items-center gap-3 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2"
			role="alert"
		>
			<p class="flex-1 text-sm text-destructive">{failure}</p>
			<Button variant="outline" size="sm" onclick={() => void load()}
				>{$t('ui.common.retry')}</Button
			>
		</div>
	{:else if entries.length === 0}
		<div class="empty-panel section-surface border-dashed">
			{$t('ui.pages.logsPage.startupEmpty')}
		</div>
	{:else}
		<div class="flex flex-col gap-1.5">
			{#each entries as entry (entry.name)}
				{@const transcript = transcripts[entry.name]}
				<details
					class="group rounded-panel border border-line bg-surface px-3.5 py-2"
					open={open[entry.name] ?? false}
					ontoggle={(event) => void openTranscript(entry, event.currentTarget.open)}
				>
					<summary
						class="flex cursor-pointer list-none items-center gap-2.5 select-none focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring [&::-webkit-details-marker]:hidden"
					>
						<Icon
							name="chevron-right"
							size={12}
							class="shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-90"
						/>
						<StatusBadge
							kind={startupOutcomeTone(entry.outcome)}
							label={entry.outcome === 'failed'
								? $t('ui.pages.logsPage.startupFailed')
								: $t('ui.pages.logsPage.startupStarted')}
						/>
						<span class="mono-data shrink-0 text-muted-foreground"
							>{formatDateTime(startedMs(entry))}</span
						>
						{#if entry.outcome === 'failed' && entry.error}
							<span class="mono-data min-w-0 flex-1 truncate text-danger" title={entry.error}
								>{entry.error}</span
							>
						{:else}
							<span class="min-w-0 flex-1"></span>
						{/if}
						{#if entry.current}
							<StatusBadge kind="muted" label={$t('ui.pages.logsPage.startupCurrent')} />
						{/if}
					</summary>
					<div class="flex flex-col gap-1.5 pt-2 pl-5">
						{#if !transcript || transcript.loading}
							<p class="text-sm text-muted-foreground">{$t('ui.common.loading')}</p>
						{:else if transcript.failure}
							<p class="text-sm text-destructive" role="alert">{transcript.failure}</p>
						{:else if transcript.lines.length === 0}
							<p class="text-sm text-muted-foreground">{$t('ui.pages.logsPage.transcriptEmpty')}</p>
						{:else}
							{#each transcript.lines as line (line.offset)}
								<ProcessLine {line} />
							{/each}
						{/if}
					</div>
				</details>
			{/each}
		</div>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.logsPage.startupKept')}</p>
	{/if}
</div>
