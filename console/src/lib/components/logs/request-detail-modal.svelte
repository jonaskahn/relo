<script lang="ts">
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import RequestDetailExchange from '$lib/components/logs/request-detail-exchange.svelte';
	import RequestDetailOverview from '$lib/components/logs/request-detail-overview.svelte';
	import RequestDetailTokens from '$lib/components/logs/request-detail-tokens.svelte';
	import type { Model, UsageAttemptRow, UsageCapture, UsageLogRow } from '$lib/types';

	// RequestDetailModal is one request, in full: its identity, what it cost,
	// and what crossed the wire. It opens on the row the operator chose and
	// reads everything beside the row itself.

	interface Props {
		row: UsageLogRow | null;
		onclose: () => void;
	}

	let { row, onclose }: Props = $props();

	let attempts = $state<UsageAttemptRow[]>([]);
	let captures = $state<UsageCapture[]>([]);
	let attemptsLoading = $state(false);
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
		void read<{ items: UsageCapture[] }>('/activity/requests/' + id + '/captures', signal, {
			items: []
		}).then((data) => {
			if (settled()) return;
			captures = data.items ?? [];
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

<CenteredModal
	open={row !== null}
	onOpenChange={(value) => {
		if (!value) onclose();
	}}
	size="wide"
	title={$t('ui.pages.logsPage.detailTitle')}
>
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
