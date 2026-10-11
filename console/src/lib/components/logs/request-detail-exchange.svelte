<script lang="ts">
	import { t } from 'svelte-i18n';

	import CaptureView from '$lib/components/logs/capture-view.svelte';
	import DetailDisclosure from '$lib/components/logs/detail-disclosure.svelte';
	import RequestAttemptCard from '$lib/components/logs/request-attempt-card.svelte';
	import type { UsageAttemptRow, UsageCapture, UsageCaptureKind } from '$lib/types';

	// RequestDetailExchange is what crossed the wire for one request: the
	// exchange the agent itself had, then what Relo sent each account and what
	// that account answered. Both sections stay collapsed until an operator
	// opens one, because a captured body can be tens of megabytes.

	interface Props {
		requestId: number;
		captures: UsageCapture[];
		attempts: UsageAttemptRow[];
		capturesLoading?: boolean;
		attemptsLoading?: boolean;
	}

	let {
		requestId,
		captures,
		attempts,
		capturesLoading = false,
		attemptsLoading = false
	}: Props = $props();

	// The two halves of the exchange the agent itself had are the captures
	// that belong to no attempt.
	const agentCapture = $derived(
		captures.find((capture) => capture.kind === 'agent_request') ?? null
	);
	const agentAnswer = $derived(
		captures.find((capture) => capture.kind === 'agent_response') ?? null
	);

	function messages(ordinal: number): {
		request: UsageCapture | null;
		response: UsageCapture | null;
	} {
		const of = (kind: UsageCaptureKind) =>
			captures.find((capture) => capture.ordinal === ordinal && capture.kind === kind) ?? null;
		return { request: of('provider_request'), response: of('provider_response') };
	}
</script>

<DetailDisclosure title={$t('ui.pages.logsPage.exchangeAgentTitle')}>
	<div class="space-y-1 px-4 py-3">
		{#if capturesLoading}
			<p class="text-muted-foreground text-sm">{$t('ui.common.loading')}</p>
		{:else if agentCapture === null && agentAnswer === null}
			<p class="text-muted-foreground text-sm">{$t('ui.pages.logsPage.captureMissing')}</p>
		{:else}
			{#if agentCapture !== null}
				<CaptureView {requestId} capture={agentCapture} />
			{/if}
			{#if agentAnswer !== null}
				<CaptureView {requestId} capture={agentAnswer} />
			{/if}
		{/if}
	</div>
</DetailDisclosure>

<DetailDisclosure title={$t('ui.pages.logsPage.exchangeProviderTitle')}>
	<div class="space-y-2 px-4 py-3">
		{#if attemptsLoading}
			<p class="text-muted-foreground text-sm">{$t('ui.common.loading')}</p>
		{:else if attempts.length === 0}
			<p class="text-muted-foreground text-sm">{$t('ui.pages.logsPage.attemptsEmpty')}</p>
		{:else}
			{#each attempts as attempt (attempt.Ordinal)}
				{@const exchange = messages(attempt.Ordinal)}
				<RequestAttemptCard
					{requestId}
					{attempt}
					request={exchange.request}
					response={exchange.response}
					loading={capturesLoading}
				/>
			{/each}
		{/if}
	</div>
</DetailDisclosure>
