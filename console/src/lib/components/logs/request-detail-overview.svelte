<script lang="ts">
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import DetailDisclosure from '$lib/components/logs/detail-disclosure.svelte';
	import { formatDateTime } from '$lib/format';
	import type { UsageLogRow } from '$lib/types';

	// RequestDetailOverview is the identity of one request: the id an operator
	// copies, the times and models the routing chose, and who sent it. It is
	// the one disclosure that opens with the modal.

	interface Props {
		row: UsageLogRow;
	}

	let { row }: Props = $props();

	let copiedId = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | undefined;

	async function copyRequestId() {
		try {
			await navigator.clipboard.writeText(String(row.RequestID || row.ID));
			copiedId = true;
			clearTimeout(copyTimer);
			copyTimer = setTimeout(() => (copiedId = false), 1500);
		} catch {
			copiedId = false;
		}
	}
</script>

<DetailDisclosure open title={$t('ui.pages.logsPage.detailOverview')}>
	<dl class="space-y-2 px-4 py-3 text-sm">
		<div class="flex items-center justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailRequest')}</dt>
			<dd class="flex min-w-0 items-center gap-1">
				<span class="mono-data truncate">{row.RequestID || row.ID}</span>
				<Button
					variant="ghost"
					size="icon"
					aria-label={$t('ui.pages.logsPage.copyRequestId')}
					onclick={() => void copyRequestId()}
				>
					<Icon name={copiedId ? 'check' : 'copy'} size={14} />
				</Button>
			</dd>
		</div>
		<div class="flex justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.columnTime')}</dt>
			<dd class="mono-data">{formatDateTime(row.TimestampMs)}</dd>
		</div>
		<div class="flex justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailStatus')}</dt>
			<dd class="mono-data" class:text-destructive={row.Status >= 400}>
				{row.Status}
			</dd>
		</div>
		<div class="flex justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailModel')}</dt>
			<dd class="mono-data">{row.Model}</dd>
		</div>
		{#if row.RequestedModel && row.RequestedModel !== row.Model}
			<div class="flex justify-between gap-4">
				<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailRequestedModel')}</dt>
				<dd class="mono-data">{row.RequestedModel}</dd>
			</div>
		{/if}
		<div class="flex justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailRoute')}</dt>
			<dd class="mono-data">
				{row.RouteProvider || row.Provider}{#if row.RouteReason}<span class="text-muted-foreground">
						· {row.RouteReason}</span
					>{/if}
			</dd>
		</div>
		<div class="flex justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailOrigin')}</dt>
			<dd class="mono-data">
				{row.Origin === 'internal'
					? $t('ui.pages.logsPage.originInternal')
					: $t('ui.pages.logsPage.originExternal')}
			</dd>
		</div>
		<div class="flex justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailClientKey')}</dt>
			<dd class="mono-data">{row.ClientKeyName || '—'}</dd>
		</div>
		<div class="flex justify-between gap-4">
			<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailSurface')}</dt>
			<dd class="mono-data">{row.Surface || '—'}</dd>
		</div>
		{#if row.CredentialLabel}
			<div class="flex justify-between gap-4">
				<dt class="text-muted-foreground">{$t('ui.pages.logsPage.detailCredential')}</dt>
				<dd class="mono-data">{row.CredentialLabel}</dd>
			</div>
		{/if}
	</dl>
</DetailDisclosure>
