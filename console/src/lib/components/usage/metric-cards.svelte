<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { metricLabelKey, type MetricValue } from '$lib/usage-metrics';
	import { metricValueOf, type ChartMetric } from '$lib/usage-charts';
	import type { UsageRollupRow } from '$lib/types';

	// MetricCards is the chosen figures at the top of the overview. The page
	// owns which metrics are chosen and in what order; a card only states what
	// one of them reads, plus a sparkline when the daily read can draw it.
	interface Props {
		cards: MetricValue[];
		days: UsageRollupRow[];
		sparklineNote?: string;
	}

	let { cards, days, sparklineNote = '' }: Props = $props();

	const ICONS: Record<string, IconName> = {
		requests: 'list',
		successes: 'circle-check',
		errors: 'circle-alert',
		success_rate: 'circle-check',
		error_rate: 'circle-alert',
		attempts: 'refresh',
		retried: 'refresh',
		tokens_input: 'sparkles',
		tokens_output: 'sparkles',
		tokens_total: 'sparkles',
		tokens_per_request: 'sparkles',
		cache_read: 'database',
		cache_write: 'database',
		spend: 'currency-dollar',
		spend_per_request: 'currency-dollar',
		plan_usage: 'currency-dollar',
		priced_requests: 'list',
		unpriced_requests: 'list',
		duration_avg: 'clock',
		duration_max: 'clock'
	};

	const SERIES: Record<string, ChartMetric | null> = {
		requests: 'requests',
		successes: 'requests',
		errors: 'errors',
		success_rate: 'requests',
		error_rate: 'requests',
		tokens_input: 'tokens',
		tokens_output: 'tokens',
		tokens_total: 'tokens',
		cache_read: 'tokens',
		cache_write: 'tokens',
		spend: 'spend',
		plan_usage: null,
		duration_avg: 'latency',
		duration_max: 'latency'
	};

	function cardLabel(card: MetricValue, unit: string): string {
		return card.unitKey ? card.exact + ' ' + unit : card.exact;
	}

	function points(card: MetricValue): number[] {
		const metric = SERIES[card.id];
		if (!metric) return [];
		return days.map((row) => {
			if (card.id === 'successes') return Math.max(0, Number(row.Requests) - Number(row.Errors));
			if (card.id === 'success_rate') {
				return Number(row.Requests) > 0
					? (Number(row.Requests) - Number(row.Errors)) / Number(row.Requests)
					: 0;
			}
			if (card.id === 'error_rate') {
				return Number(row.Requests) > 0 ? Number(row.Errors) / Number(row.Requests) : 0;
			}
			if (card.id === 'tokens_input') return Math.max(0, Number(row.InputTokens));
			if (card.id === 'tokens_output') return Math.max(0, Number(row.OutputTokens));
			if (card.id === 'cache_read') return Math.max(0, Number(row.CacheReadTokens));
			if (card.id === 'cache_write') return Math.max(0, Number(row.CacheWriteTokens));
			if (card.id === 'duration_max') return Math.max(0, Number(row.DurationMaxMs));
			return metricValueOf(row, metric);
		});
	}

	function hasSeries(card: MetricValue): boolean {
		return (
			card.id !== 'plan_usage' && card.id !== 'spend_per_request' && card.id !== 'duration_max'
		);
	}

	function sparkPath(values: number[]): string {
		if (values.length < 2) return '';
		const max = Math.max(1, ...values);
		return values
			.map((value, index) => {
				const x = (index / (values.length - 1)) * 104;
				const y = 40 - (value / max) * 34;
				return (index === 0 ? 'M' : 'L') + x.toFixed(1) + ' ' + y.toFixed(1);
			})
			.join(' ');
	}
</script>

{#if cards.length === 0}
	<div class="empty-panel section-surface border-dashed">
		{$t('ui.pages.usagePage.metricsEmpty')}
	</div>
{:else}
	<div class="grid grid-cols-2 gap-[var(--grid-gap)] xl:grid-cols-3">
		{#each cards.slice(0, 12) as card (card.id)}
			{@const unit = card.unitKey ? $t('ui.pages.usagePage.' + card.unitKey) : ''}
			{@const series = points(card)}
			<div
				class="section-surface flex min-w-0 items-center justify-between gap-4 p-[var(--card-pad)]"
				aria-label={cardLabel(card, unit)}
			>
				<div class="min-w-0">
					<p class="flex items-center gap-1.5 text-[0.8125rem] font-medium text-muted-foreground">
						<Icon name={ICONS[card.id] ?? 'chart-bar'} size={14} class="shrink-0 text-faint" />
						<span class="truncate">{$t(metricLabelKey(card.id))}</span>
					</p>
					<p
						class="mono-data mt-1.5 font-medium tabular-nums"
						style="font-size:1.75rem;line-height:1.1"
					>
						<span class:text-destructive={card.danger} title={cardLabel(card, unit)}
							>{card.value}</span
						>
						{#if card.unitKey}<span class="ms-1 text-xs font-normal text-muted-foreground"
								>{$t('ui.pages.usagePage.' + card.unitKey)}</span
							>{/if}
					</p>
					{#if card.noteKey}
						<p class="mt-1 text-xs text-muted-foreground">
							{$t('ui.pages.usagePage.' + card.noteKey, { values: card.noteValues })}
						</p>
					{/if}
				</div>
				{#if hasSeries(card) && series.length > 1}
					<svg
						class="hidden h-11 w-24 shrink-0 overflow-visible sm:block"
						viewBox="0 0 104 44"
						fill="none"
						aria-hidden="true"
					>
						<path
							d={sparkPath(series)}
							stroke={card.danger ? 'var(--danger)' : 'var(--accent)'}
							stroke-width="2"
							stroke-linecap="round"
							stroke-linejoin="round"
							vector-effect="non-scaling-stroke"
						/>
					</svg>
				{/if}
			</div>
		{/each}
	</div>
	{#if sparklineNote}
		<p class="mt-2 text-right text-xs text-muted-foreground">{sparklineNote}</p>
	{/if}
{/if}
