<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { metricLabelKey, metricSeriesPoint, type MetricValue } from '$lib/usage-metrics';
	import type { UsageRollupRow } from '$lib/types';

	// MetricCards is the chosen figures at the top of the overview. The page
	// owns which metrics are chosen and in what order; a card only states what
	// one of them reads, plus a sparkline when the daily read can draw it.
	interface Props {
		cards: MetricValue[];
		days: UsageRollupRow[];
		planDays?: UsageRollupRow[];
		sparklineNote?: string;
	}

	let { cards, days, planDays = [], sparklineNote = '' }: Props = $props();

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

	function cardLabel(card: MetricValue, unit: string): string {
		return card.unitKey ? card.exact + ' ' + unit : card.exact;
	}

	function points(card: MetricValue): Array<number | null> {
		const planByDay = new Map(planDays.map((row) => [row.Key, Number(row.CostMicros)]));
		return days.map((row) => metricSeriesPoint(card.id, row, planByDay.get(row.Key) ?? 0));
	}

	function drawable(series: Array<number | null>): boolean {
		const values = series.filter((value): value is number => value !== null);
		return values.some((value) => value > 0) && values.length > 1;
	}

	function sparkPath(values: Array<number | null>): string {
		if (values.length < 2) return '';
		const max = Math.max(1, ...values.map((value) => value ?? 0));
		let path = '';
		let drawing = false;
		for (const [index, value] of values.entries()) {
			if (value === null) {
				drawing = false;
				continue;
			}
			const x = (index / (values.length - 1)) * 104;
			const y = 40 - (value / max) * 34;
			path += (drawing ? 'L' : 'M') + x.toFixed(1) + ' ' + y.toFixed(1);
			drawing = true;
		}
		return path;
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
				{#if drawable(series)}
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
