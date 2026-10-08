<script lang="ts">
	import { t } from 'svelte-i18n';

	import CaptureView from '$lib/components/logs/capture-view.svelte';
	import { formatDuration } from '$lib/format';
	import type { UsageAttemptRow, UsageCapture } from '$lib/types';

	// RequestAttemptCard is one try at one account: what Relo sent it, what it
	// answered, and how the try ended. Either half of the exchange may be
	// missing, which the card says rather than drawing nothing.

	interface Props {
		requestId: number;
		attempt: UsageAttemptRow;
		request: UsageCapture | null;
		response: UsageCapture | null;
		loading?: boolean;
	}

	let { requestId, attempt, request, response, loading = false }: Props = $props();
</script>

<div class="border-border space-y-2 rounded-lg border p-3">
	<div class="flex items-center justify-between gap-3">
		<div class="min-w-0">
			<p class="text-sm font-medium">
				{$t('ui.pages.logsPage.attempt')}
				{attempt.Ordinal + 1}
			</p>
			<p class="mono-data truncate text-muted-foreground">
				{attempt.Provider} · {attempt.Model}
			</p>
			{#if attempt.CredentialLabel || attempt.CredentialID}
				<p class="mono-data truncate text-xs text-muted-foreground">
					{$t('ui.pages.logsPage.captureAccount')}: {attempt.CredentialLabel ||
						attempt.CredentialID}
				</p>
			{/if}
		</div>
		<div class="text-right">
			<p class="mono-data" class:text-destructive={attempt.Status >= 400}>
				{attempt.Status}
			</p>
			<p class="mono-data text-muted-foreground">
				{formatDuration(attempt.DurationMs)}
			</p>
		</div>
	</div>
	{#if attempt.ErrorCode}
		<p class="mono-data text-xs text-destructive">{attempt.ErrorCode}</p>
	{/if}
	<div class="space-y-1">
		{#if loading}
			<p class="text-xs text-muted-foreground">{$t('ui.common.loading')}</p>
		{:else if request === null && response === null}
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.logsPage.captureMissing')}
			</p>
		{:else}
			{#if request !== null}
				<CaptureView {requestId} capture={request} />
			{/if}
			{#if response !== null}
				<CaptureView {requestId} capture={response} />
			{/if}
		{/if}
	</div>
</div>
