<script lang="ts">
	import type { UsageLogRow } from '$lib/types';
	import ReportIssueButton from '$lib/components/report-issue/report-issue-button.svelte';

	interface Props {
		row: UsageLogRow;
	}

	let { row }: Props = $props();

	function details(request: UsageLogRow): string {
		return [
			'Request ID: ' + (request.RequestID || request.ID),
			'Status: ' + request.Status,
			'Model: ' + request.Model,
			'Provider: ' + request.Provider,
			'Route: ' + (request.RouteProvider || request.Provider),
			'Duration: ' + request.DurationMs + ' ms',
			'Attempts: ' + request.Attempts,
			'Retried: ' + request.Retried
		].join('\n');
	}
</script>

{#if row.Status >= 400}
	<ReportIssueButton summary={`Relo request failed: ${row.Status}`} details={details(row)} />
{/if}
