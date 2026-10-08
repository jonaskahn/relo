<script lang="ts">
	import type { Snippet } from 'svelte';
	import { fade } from 'svelte/transition';
	import { t } from 'svelte-i18n';

	import Segmented from '$lib/components/ui/segmented.svelte';
	import type { IconName } from '$lib/components/ui/icon.svelte';
	import { reducedMotion } from '$lib/card-layout-motion';
	import {
		barGeom,
		formatChartValue,
		metricValueOf,
		shortDay,
		trendTotal,
		type ChartMetric
	} from '$lib/usage-charts';
	import type { UsageRollupRow } from '$lib/types';

	// TrendChart is the range at day grain: one column per day, tallest first
	// read, with the series the operator picked beneath the header. The days
	// arrive already bucketed, so the readout states the same rows it draws.
	// The bars grow from the baseline once, together, when the first series
	// arrives, then re-shape rather than replay on a window change.
	interface Props {
		days: UsageRollupRow[];
		metric: ChartMetric;
		options: Array<{ value: ChartMetric; label: string; icon: IconName }>;
		loading: boolean;
		pending: Snippet;
		onmetric: (metric: ChartMetric) => void;
	}

	let { days, metric, options, loading, pending, onmetric }: Props = $props();

	let hoveredKey = $state('');
	let drawn = $state(false);
	let played = false;

	const values = $derived(days.map((row) => ({ key: row.Key, value: metricValueOf(row, metric) })));
	const max = $derived(Math.max(1, ...values.map((entry) => entry.value)));
	// SeriesKey names the days the bars carry, so the readout fades when the
	// series changes while a hover swap stays instant.
	const seriesKey = $derived(metric + '|' + values.map((entry) => entry.key).join(','));
	const readout = $derived.by(() => {
		const hovered = values.find((entry) => entry.key === hoveredKey);
		if (hovered) return shortDay(hovered.key) + ' · ' + formatChartValue(hovered.value, metric);
		return formatChartValue(trendTotal(days, metric), metric);
	});

	// The bars grow once, on the first series the chart is handed.
	$effect(() => {
		if (played || loading || days.length === 0) return;
		played = true;
		if (reducedMotion()) {
			drawn = true;
			return;
		}
		const frame = requestAnimationFrame(() => (drawn = true));
		return () => cancelAnimationFrame(frame);
	});
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t('ui.pages.dashboard.trendsTitle')}</h2>
		<Segmented
			value={metric}
			{options}
			ariaLabel={$t('ui.pages.dashboard.trendsTitle')}
			onchange={(next) => {
				hoveredKey = '';
				onmetric(next);
			}}
		/>
	</div>
	{#if loading}
		{@render pending()}
	{:else if days.length === 0}
		<div class="flex h-20 items-center justify-center text-xs text-muted-foreground">
			{$t('ui.pages.dashboard.noData')}
		</div>
	{:else}
		{#key seriesKey}
			<p
				class="mb-2 flex min-h-5 items-center gap-2 text-sm font-medium tabular-nums text-ink"
				in:fade={{ duration: reducedMotion() || !drawn ? 0 : 120 }}
			>
				{readout}
			</p>
		{/key}
		<svg
			viewBox="0 0 640 84"
			class="h-24 w-full"
			preserveAspectRatio="none"
			role="img"
			aria-label={$t('ui.pages.dashboard.trendsTitle')}
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
					class="trend-bar"
					class:trend-ready={drawn}
					in:fade={{ duration: reducedMotion() || !drawn ? 0 : 200 }}
					out:fade={{ duration: reducedMotion() ? 0 : 160 }}
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
		<div class="mt-1.5 flex justify-between gap-1.5 text-[0.6875rem] text-muted-foreground">
			{#each values as entry (entry.key)}
				<span class="mono-data min-w-0 flex-1 truncate text-center">{shortDay(entry.key)}</span>
			{/each}
		</div>
	{/if}
</div>

<style>
	/* The trend bars grow from the baseline once, together, when the first
	   series arrives, and re-shape on a window change. Flip cannot drive the
	   bars: it animates transform, which the grow animation already owns, so
	   the geometry is transitioned instead and degrades to an instant redraw. */
	.trend-bar {
		transform-box: fill-box;
		transform-origin: bottom;
		transform: scaleY(0);
		transition:
			transform 400ms cubic-bezier(0.2, 0.8, 0.2, 1),
			x 400ms cubic-bezier(0.2, 0.8, 0.2, 1),
			y 400ms cubic-bezier(0.2, 0.8, 0.2, 1),
			width 400ms cubic-bezier(0.2, 0.8, 0.2, 1),
			height 400ms cubic-bezier(0.2, 0.8, 0.2, 1),
			fill 150ms ease;
	}

	.trend-ready {
		transform: scaleY(1);
	}

	@media (prefers-reduced-motion: reduce) {
		.trend-bar {
			transform: none;
			transition: fill 150ms ease;
		}
	}
</style>
