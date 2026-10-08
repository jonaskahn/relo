<script lang="ts">
	import { t } from 'svelte-i18n';

	import { formatTokens } from '$lib/format';
	import { barGeom, shortDay, type TokenMixPoint } from '$lib/usage-charts';

	// TokenMixCard splits each day's tokens into what was sent, what came back,
	// and what a cache carried.
	interface Props {
		points: TokenMixPoint[];
	}

	let { points }: Props = $props();

	let hoveredKey = $state('');

	const max = $derived(Math.max(1, ...points.map((point) => point.total)));
	const total = $derived(points.reduce((sum, point) => sum + point.total, 0));
	const readout = $derived.by(() => {
		const hovered = points.find((point) => point.key === hoveredKey);
		if (hovered) return shortDay(hovered.key) + ' · ' + formatTokens(hovered.total);
		return formatTokens(total) + ' ' + $t('ui.pages.usagePage.unitTokens');
	});
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t('ui.pages.usagePage.tokenMixTitle')}</h2>
		<p class="flex min-h-5 flex-wrap items-center gap-2 text-sm font-medium tabular-nums text-ink">
			{readout}
		</p>
	</div>
	{#if points.length === 0 || total === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<svg
			viewBox="0 0 640 84"
			class="h-24 w-full"
			preserveAspectRatio="none"
			role="img"
			aria-label={$t('ui.pages.usagePage.tokenMixTitle')}
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
				{@const bar = barGeom(index, points.length, point.total, max)}
				{@const frac = point.total > 0 ? point.total : 1}
				{@const hIn = (point.input / frac) * bar.h}
				{@const hOut = (point.output / frac) * bar.h}
				{@const hRead = (point.cacheRead / frac) * bar.h}
				{@const hWrite = Math.max(0, bar.h - hIn - hOut - hRead)}
				<g
					role="img"
					aria-label={shortDay(point.key) + ': ' + formatTokens(point.total)}
					onmouseenter={() => (hoveredKey = point.key)}
					onmouseleave={() => (hoveredKey = '')}
					opacity={hoveredKey === '' || hoveredKey === point.key ? 1 : 0.35}
				>
					<title>
						{shortDay(point.key)}: {formatTokens(point.total)} ({formatTokens(point.input)}
						{$t('ui.pages.usagePage.summaryTokensIn').toLowerCase()} · {formatTokens(point.output)}
						{$t('ui.pages.usagePage.summaryTokensOut').toLowerCase()})
					</title>
					<rect
						x={bar.x}
						y={84 - hIn}
						width={bar.w}
						height={Math.max(0, hIn)}
						fill="var(--accent-200)"
					/>
					<rect
						x={bar.x}
						y={84 - hIn - hOut}
						width={bar.w}
						height={Math.max(0, hOut)}
						fill="var(--accent-400)"
					/>
					<rect
						x={bar.x}
						y={84 - hIn - hOut - hRead}
						width={bar.w}
						height={Math.max(0, hRead)}
						fill="var(--accent-600)"
					/>
					<rect
						x={bar.x}
						y={84 - hIn - hOut - hRead - hWrite}
						width={bar.w}
						height={hWrite}
						fill="var(--accent-800)"
					/>
				</g>
			{/each}
		</svg>
	{/if}
	<ul class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
		<li class="inline-flex items-center gap-1.5">
			<span
				class="inline-block size-2 rounded-sm"
				style="background:var(--accent-200)"
				aria-hidden="true"
			></span>
			{$t('ui.pages.usagePage.summaryTokensIn')}
		</li>
		<li class="inline-flex items-center gap-1.5">
			<span
				class="inline-block size-2 rounded-sm"
				style="background:var(--accent-400)"
				aria-hidden="true"
			></span>
			{$t('ui.pages.usagePage.summaryTokensOut')}
		</li>
		<li class="inline-flex items-center gap-1.5">
			<span
				class="inline-block size-2 rounded-sm"
				style="background:var(--accent-600)"
				aria-hidden="true"
			></span>
			{$t('ui.pages.usagePage.cacheReadShort')}
		</li>
		<li class="inline-flex items-center gap-1.5">
			<span
				class="inline-block size-2 rounded-sm"
				style="background:var(--accent-800)"
				aria-hidden="true"
			></span>
			{$t('ui.pages.usagePage.cacheWriteShort')}
		</li>
	</ul>
</div>
