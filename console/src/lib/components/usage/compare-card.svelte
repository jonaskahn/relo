<script lang="ts">
	import { t } from 'svelte-i18n';

	import Segmented from '$lib/components/ui/segmented.svelte';
	import { barGeom, formatChartValue, type ChartBar, type ChartMetric } from '$lib/usage-charts';
	import type { IconName } from '$lib/components/ui/icon.svelte';

	// CompareCard ranks the rows of one grouping against each other: a column
	// each, with the list beneath stating the same figures.
	interface Props {
		bars: ChartBar[];
		metric: ChartMetric;
		metricLabel: string;
		options: Array<{ value: ChartMetric; label: string; icon: IconName }>;
		captionOf: (key: string) => string;
		titleOf: (key: string) => string | undefined;
		onmetric: (metric: ChartMetric) => void;
	}

	let { bars, metric, metricLabel, options, captionOf, titleOf, onmetric }: Props = $props();
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">
			{$t('ui.pages.usagePage.compareTitle', {
				values: { count: bars.length, metric: metricLabel }
			})}
		</h2>
		<div class="max-w-full overflow-x-auto">
			<Segmented
				value={metric}
				{options}
				ariaLabel={$t('ui.pages.usagePage.chartMetric')}
				onchange={onmetric}
			/>
		</div>
	</div>
	{#if bars.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<svg
			viewBox="0 0 640 84"
			class="h-24 w-full"
			preserveAspectRatio="none"
			role="img"
			aria-label={$t('ui.pages.usagePage.compareTitle', {
				values: { count: bars.length, metric: metricLabel }
			})}
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
			{#each bars as bar, index (bar.key)}
				{@const geom = barGeom(index, bars.length, bar.value, Math.max(1, bars[0]?.value ?? 1))}
				<rect
					x={geom.x}
					y={geom.y}
					width={geom.w}
					height={geom.h}
					rx="2"
					fill={index === 0 ? 'var(--accent)' : 'var(--accent-200)'}
					role="img"
					aria-label={captionOf(bar.key) + ': ' + formatChartValue(bar.value, metric)}
					><title>{captionOf(bar.key)}: {formatChartValue(bar.value, metric)}</title></rect
				>
			{/each}
		</svg>

		<ul class="mt-4 space-y-2">
			{#each bars as bar (bar.key)}
				<li class="relative overflow-hidden rounded-md">
					<span
						class="absolute inset-y-0 start-0 bg-accent/10"
						style="width: {bar.share}%"
						aria-hidden="true"
					></span>
					<div class="relative flex items-baseline justify-between gap-3 px-2 py-1.5">
						<span class="mono-data truncate text-sm" title={titleOf(bar.key)}
							>{captionOf(bar.key)}</span
						>
						<span class="shrink-0 text-sm font-medium text-foreground tabular-nums">
							{formatChartValue(bar.value, metric)}
						</span>
					</div>
				</li>
			{/each}
		</ul>
	{/if}
</div>
