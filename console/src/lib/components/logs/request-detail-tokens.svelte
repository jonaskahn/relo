<script lang="ts">
	import { t } from 'svelte-i18n';

	import DetailDisclosure from '$lib/components/logs/detail-disclosure.svelte';
	import { formatCost, formatDuration, formatUnitPrice } from '$lib/format';
	import type { Model, UsageLogRow } from '$lib/types';

	// RequestDetailTokens is what the request cost and what it was billed at.
	// The rates come from the model as the console reads it now, so a price
	// that changed since shows the current one; the note under the table says
	// so rather than leaving the two numbers to disagree silently.

	interface Props {
		row: UsageLogRow;
		model: Model | null;
		loading?: boolean;
	}

	let { row, model, loading = false }: Props = $props();

	function computeMicros(tokens: number, ratePerMillion: number | null | undefined): number | null {
		if (tokens <= 0 || ratePerMillion == null || ratePerMillion <= 0) return 0;
		return Math.round((tokens * ratePerMillion + 500000) / 1000000);
	}

	const durationText = $derived(formatDuration(row.DurationMs));
	const totalTokens = $derived((row.InputTokens + row.OutputTokens).toLocaleString());
	const totalCostText = $derived(formatCost(row.EstimatedCostMicros));

	const breakdown = $derived.by(() => {
		const prices = model?.prices?.effective;
		const cacheRead = row.CacheReadTokens;
		const cacheWrite = row.CacheWriteTokens;
		const plainInput = Math.max(0, row.InputTokens - cacheRead - cacheWrite);
		// The four categories a request is billed in. Their names are the same
		// words the model editor uses, so they share its keys rather than
		// carrying a second copy of four common words in every locale.
		const label = (name: 'input' | 'output' | 'cacheRead' | 'cacheWrite') =>
			$t('ui.pages.providersPage.models.' + name);
		const line = (
			name: 'input' | 'output' | 'cacheRead' | 'cacheWrite',
			tokens: number,
			rate: number | null | undefined
		) => {
			const cost = computeMicros(tokens, rate);
			return {
				label: label(name),
				tokensText: tokens.toLocaleString(),
				rateText: rate != null ? formatUnitPrice(rate) : '—',
				costText: cost != null ? formatCost(cost) : '—'
			};
		};
		return [
			line('input', plainInput, prices?.input),
			line('output', row.OutputTokens, prices?.output),
			line('cacheRead', cacheRead, prices?.cache_read ?? prices?.input),
			line('cacheWrite', cacheWrite, prices?.cache_write ?? prices?.input)
		];
	});
</script>

<DetailDisclosure title={$t('ui.pages.logsPage.detailTokensCost')}>
	<div class="px-4 py-3 text-sm">
		<div class="flex justify-between gap-4 pb-2">
			<span class="text-muted-foreground">{$t('ui.pages.logsPage.detailDuration')}</span>
			<span class="mono-data">{durationText}</span>
		</div>
		<table class="w-full text-sm">
			<thead>
				<tr class="text-xs font-medium text-muted-foreground">
					<th class="py-1.5 text-left font-medium">{$t('ui.pages.logsPage.columnType')}</th>
					<th class="py-1.5 text-right font-medium">{$t('ui.pages.logsPage.columnTokens')}</th>
					<th class="py-1.5 text-right font-medium"
						>{$t('ui.pages.logsPage.columnRatePerMillion')}</th
					>
					<th class="py-1.5 text-right font-medium">{$t('ui.pages.logsPage.columnCost')}</th>
				</tr>
			</thead>
			<tbody>
				{#each breakdown as line (line.label)}
					<tr>
						<td class="py-1.5 text-muted-foreground">{line.label}</td>
						<td class="mono-data py-1.5 text-right">{line.tokensText}</td>
						<td class="mono-data py-1.5 text-right">{line.rateText}</td>
						<td class="mono-data py-1.5 text-right">{line.costText}</td>
					</tr>
				{/each}
			</tbody>
			<tfoot>
				<tr class="font-medium">
					<td class="py-1.5">{$t('ui.pages.logsPage.columnTotal')}</td>
					<td class="mono-data py-1.5 text-right">{totalTokens}</td>
					<td class="py-1.5"></td>
					<td class="mono-data py-1.5 text-right">{totalCostText}</td>
				</tr>
			</tfoot>
		</table>
		{#if loading}
			<p class="pt-2 text-xs text-muted-foreground">{$t('ui.common.loading')}</p>
		{:else if !model}
			<p class="pt-2 text-xs text-muted-foreground">
				{$t('ui.pages.logsPage.detailRatesUnavailable')}
			</p>
		{/if}
	</div>
</DetailDisclosure>
