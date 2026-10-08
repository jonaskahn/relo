<script lang="ts">
	import { t } from 'svelte-i18n';

	import { formatDuration } from '$lib/format';
	import {
		barGeom,
		formatChartValue,
		shortDay,
		trendTotal,
		type LatencyPoint
	} from '$lib/usage-charts';
	import type { UsageRollupRow } from '$lib/types';

	// LatencyCard draws each day's average, with the day's slowest request as a
	// tick above it.
	interface Props {
		days: UsageRollupRow[];
		points: LatencyPoint[];
	}

	let { days, points }: Props = $props();

	let hoveredKey = $state('');

	const scale = $derived(Math.max(1, ...points.map((point) => point.max)));
	const slowest = $derived(points.reduce((worst, point) => Math.max(worst, point.max), 0));
	const readout = $derived.by(() => {
		const hovered = points.find((point) => point.key === hoveredKey);
		if (hovered) {
			return (
				shortDay(hovered.key) +
				' · ' +
				(hovered.average > 0 ? formatDuration(Math.round(hovered.average)) : '—') +
				' · ' +
				$t('ui.pages.usagePage.columnMaxDuration').toLowerCase() +
				' ' +
				(hovered.max > 0 ? formatDuration(hovered.max) : '—')
			);
		}
		return (
			formatChartValue(trendTotal(days, 'latency'), 'latency') +
			' ' +
			$t('ui.pages.usagePage.summaryDurationPerRequest').toLowerCase() +
			' · ' +
			$t('ui.pages.usagePage.columnMaxDuration').toLowerCase() +
			' ' +
			(slowest > 0 ? formatDuration(slowest) : '—')
		);
	});
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t('ui.pages.usagePage.latencyTitle')}</h2>
		<p class="flex min-h-5 flex-wrap items-center gap-2 text-sm font-medium tabular-nums text-ink">
			{readout}
		</p>
	</div>
	{#if points.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<svg
			viewBox="0 0 640 84"
			class="h-24 w-full"
			preserveAspectRatio="none"
			role="img"
			aria-label={$t('ui.pages.usagePage.latencyTitle')}
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
			{#each points as point, index (point.key)}
				{@const bar = barGeom(index, points.length, point.average, scale)}
				{@const tickY = 84 - Math.max(point.max > 0 ? 2 : 0, (point.max / scale) * 84)}
				<g
					role="img"
					aria-label={shortDay(point.key) + ': ' + formatChartValue(point.average, 'latency')}
					onmouseenter={() => (hoveredKey = point.key)}
					onmouseleave={() => (hoveredKey = '')}
					opacity={hoveredKey === '' || hoveredKey === point.key ? 1 : 0.35}
				>
					<title>
						{shortDay(point.key)}: {point.average > 0
							? formatDuration(Math.round(point.average))
							: '—'}
						· {$t('ui.pages.usagePage.columnMaxDuration').toLowerCase()}
						{point.max > 0 ? formatDuration(point.max) : '—'}
					</title>
					<rect
						x={bar.x}
						y={bar.y}
						width={bar.w}
						height={bar.h}
						rx="2"
						fill={index === points.length - 1 ? 'var(--accent)' : 'var(--accent-200)'}
					/>
					{#if point.max > 0}
						<line
							x1={bar.x}
							y1={tickY}
							x2={bar.x + bar.w}
							y2={tickY}
							stroke="var(--line-strong)"
							stroke-width="2"
							vector-effect="non-scaling-stroke"
						/>
					{/if}
				</g>
			{/each}
		</svg>
	{/if}
	<p class="mt-2 text-xs text-muted-foreground">
		{$t('ui.pages.usagePage.noteSlowest', {
			values: { value: slowest > 0 ? formatDuration(slowest) : '—' }
		})}
	</p>
</div>
