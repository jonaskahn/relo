<script lang="ts">
	import { t } from 'svelte-i18n';

	import Segmented from '$lib/components/ui/segmented.svelte';
	import {
		barGeom,
		formatChartValue,
		metricValueOf,
		shortDay,
		trendTotal,
		type ChartMetric
	} from '$lib/usage-charts';
	import type { UsageRollupRow } from '$lib/types';
	import type { IconName } from '$lib/components/ui/icon.svelte';

	// TrendCard is the range at day grain: one column per day, tallest first
	// read, with the metric the operator picked beneath the header. The days
	// arrive already bucketed, so the readout states the same rows it draws.
	interface Props {
		days: UsageRollupRow[];
		metric: ChartMetric;
		metricLabel: string;
		options: Array<{ value: ChartMetric; label: string; icon: IconName }>;
		onmetric: (metric: ChartMetric) => void;
	}

	let { days, metric, metricLabel, options, onmetric }: Props = $props();

	let hoveredKey = $state('');

	const values = $derived(days.map((row) => ({ key: row.Key, value: metricValueOf(row, metric) })));
	const max = $derived(Math.max(1, ...values.map((entry) => entry.value)));
	// The readout answers the bar under the pointer, and states the range's own
	// figure when nothing is being pointed at.
	const readout = $derived.by(() => {
		const hovered = values.find((entry) => entry.key === hoveredKey);
		if (hovered) return shortDay(hovered.key) + ' · ' + formatChartValue(hovered.value, metric);
		return formatChartValue(trendTotal(days, metric), metric);
	});
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t('ui.pages.usagePage.chartTitle')}</h2>
		<div class="max-w-full overflow-x-auto">
			<Segmented
				value={metric}
				{options}
				ariaLabel={$t('ui.pages.usagePage.chartMetric')}
				onchange={(next) => {
					hoveredKey = '';
					onmetric(next);
				}}
			/>
		</div>
	</div>
	{#if values.length === 0}
		<p class="mt-3 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<p
			class="mb-2 flex min-h-5 flex-wrap items-center gap-2 text-sm font-medium tabular-nums text-ink"
		>
			{readout}
		</p>
		<svg
			viewBox="0 0 640 84"
			class="h-24 w-full"
			preserveAspectRatio="none"
			role="img"
			aria-label={$t('ui.pages.usagePage.chartTitle') + ' · ' + metricLabel}
		>
			<line
				x1="0"
				y1="83"
				x2="640"
				y2="83"
				stroke="var(--line-strong)"
				stroke-width="1"
				vector-effect="non-scaling-stroke"
			/>
			{#each values as entry, index (entry.key)}
				{@const bar = barGeom(index, values.length, entry.value, max)}
				<rect
					x={bar.x}
					y={bar.y}
					width={bar.w}
					height={bar.h}
					rx="2"
					fill={hoveredKey === entry.key
						? 'var(--accent-400)'
						: index === values.length - 1
							? 'var(--accent)'
							: 'var(--accent-200)'}
					role="img"
					aria-label={shortDay(entry.key) + ': ' + formatChartValue(entry.value, metric)}
					onmouseenter={() => (hoveredKey = entry.key)}
					onmouseleave={() => (hoveredKey = '')}
					><title>{shortDay(entry.key)}: {formatChartValue(entry.value, metric)}</title></rect
				>
			{/each}
		</svg>
		<div
			class="mt-1.5 flex flex-wrap items-baseline justify-between gap-2 text-xs text-muted-foreground"
		>
			<span>
				{shortDay(values[0].key)} – {shortDay(values[values.length - 1].key)}
			</span>
			<span>
				{$t('ui.pages.usagePage.chartMax', {
					values: { value: formatChartValue(max, metric) }
				})}
			</span>
		</div>
	{/if}
</div>
