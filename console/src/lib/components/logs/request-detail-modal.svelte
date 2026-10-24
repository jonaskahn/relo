<script lang="ts">
	import { t } from 'svelte-i18n';
	
	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import { formatTimestamp } from '$lib/format';
	import RequestDetailExchange from '$lib/components/logs/request-detail-exchange.svelte';
	import RequestDetailOverview from '$lib/components/logs/request-detail-overview.svelte';
	import RequestDetailTokens from '$lib/components/logs/request-detail-tokens.svelte';
	import type { Model, UsageAttemptRow, UsageCapture, UsageLogRow } from '$lib/types';

	// RequestDetailModal is one request, in full: its identity, what it cost,
	// and what crossed the wire. It opens on the row the operator chose and
	// reads everything beside the row itself.

	interface Props {
		row: UsageLogRow | null;
		rows?: UsageLogRow[];
		onclose: () => void;
		onselect?: (row: UsageLogRow) => void;
		onimpersonate?: (row: UsageLogRow) => void;
	}

	let { row, rows = [], onclose, onselect, onimpersonate }: Props = $props();

	const index = $derived(row ? rows.findIndex((entry) => entry.ID === row.ID) : -1);
	const previous = $derived(index > 0 ? rows[index - 1] : null);
	const next = $derived(index >= 0 && index < rows.length - 1 ? rows[index + 1] : null);

	function move(step: -1 | 1) {
		const target = step < 0 ? previous : next;
		if (target) onselect?.(target);
	}

	function onKeydown(event: KeyboardEvent) {
		if (row === null) return;
		const tag = document.activeElement?.tagName ?? '';
		if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
		if (event.key === 'ArrowLeft') {
			event.preventDefault();
			move(-1);
		} else if (event.key === 'ArrowRight') {
			event.preventDefault();
			move(1);
		}
	}

	let attempts = $state<UsageAttemptRow[]>([]);
	let captures = $state<UsageCapture[]>([]);
	let attemptsLoading = $state(false);
	let traced = $state(false);
	let capturesLoading = $state(false);
	let detailModel = $state<Model | null>(null);
	let detailModelLoading = $state(false);

	// A read that fails leaves its block empty rather than raising: the detail
	// still shows what the row recorded, and a missing capture is already a
	// state the sections name.
	async function read<T>(path: string, signal: AbortSignal, fallback: T): Promise<T> {
		try {
			return await api<T>(path, { signal });
		} catch {
			return fallback;
		}
	}

	// The three reads the detail needs are independent, so they start
	// together. A row that closes or changes cancels them: a late answer
	// would otherwise land on the next row.
	$effect(() => {
		const open = row;
		attempts = [];
		captures = [];
		traced = false;
		detailModel = null;
		attemptsLoading = open !== null;
		capturesLoading = open !== null;
		detailModelLoading = open !== null;
		if (!open) return;
		const id = open.ID;
		const controller = new AbortController();
		const signal = controller.signal;
		const settled = () => signal.aborted || row?.ID !== id;

		void read<{ items: UsageAttemptRow[] }>('/activity/requests/' + id + '/attempts', signal, {
			items: []
		}).then((data) => {
			if (settled()) return;
			attempts = data.items;
			attemptsLoading = false;
		});
		void read<{ items: UsageCapture[]; traced?: boolean }>(
			'/activity/requests/' + id + '/captures',
			signal,
			{ items: [] }
		).then((data) => {
			if (settled()) return;
			captures = data.items ?? [];
			traced = data.traced === true;
			capturesLoading = false;
		});
		const provider = open.RouteProvider || open.Provider;
		void read<Model | null>(
			'/models/' + encodeURIComponent(provider) + '/' + open.Model,
			signal,
			null
		).then((model) => {
			if (settled()) return;
			detailModel = model;
			detailModelLoading = false;
		});

		return () => controller.abort();
	});
</script>

<svelte:window onkeydown={onKeydown} />

<CenteredModal
	open={row !== null}
	onOpenChange={(value) => {
		if (!value) onclose();
	}}
	placement="drawer"
	size="wide"
	title={$t('ui.pages.logsPage.detailTitle')}
	description={row ? formatTimestamp(row.TimestampMs) + ' · ' + row.Model : undefined}
>
	{#snippet titleAside()}
		{#if traced}
			<StatusBadge kind="muted" label={$t('ui.pages.logsPage.traceBadge')} />
		{/if}
	{/snippet}
	{#snippet headerNav()}
		{#if traced && row}
			<!-- The traffic log holds this request whole, so the Chat tester can
			     send the same conversation again through Relo. Nothing is sent
			     until the operator presses Send on that page. -->
			<Button
				variant="outline"
				size="sm"
				title={$t('ui.pages.logsPage.impersonateHint')}
				onclick={() => {
					if (row && onimpersonate) onimpersonate(row);
				}}
			>
				<Icon name="message-square" size={14} />
				<span class="ms-1">{$t('ui.pages.logsPage.impersonate')}</span>
			</Button>
		{/if}
		{#if index >= 0}
			<div
				class="flex items-center gap-1"
				role="group"
				aria-label={$t('ui.pages.logsPage.browseRequests')}
			>
				<Button
					variant="outline"
					size="icon"
					aria-label={$t('ui.pages.logsPage.previousRequest')}
					disabled={previous === null}
					onclick={() => move(-1)}
				>
					<Icon name="arrow-left" size={16} />
				</Button>
				<span class="mono-data min-w-14 text-center text-xs text-muted-foreground"
					>{index + 1} / {rows.length}</span
				>
				<Button
					variant="outline"
					size="icon"
					aria-label={$t('ui.pages.logsPage.nextRequest')}
					disabled={next === null}
					onclick={() => move(1)}
				>
					<Icon name="chevron-right" size={16} />
				</Button>
			</div>
		{/if}
	{/snippet}
	{#if row}
		<div class="space-y-3">
			<RequestDetailOverview {row} />
			<RequestDetailTokens {row} model={detailModel} loading={detailModelLoading} />
			<RequestDetailExchange
				requestId={row.ID}
				{captures}
				{attempts}
				{capturesLoading}
				{attemptsLoading}
			/>
		</div>
	{/if}
</CenteredModal>
