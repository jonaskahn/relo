<script lang="ts">
	import type { Snippet } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { t } from 'svelte-i18n';

	import { reducedMotion } from '$lib/card-layout-motion';
	import { formatTokens } from '$lib/format';
	import { donutFill, donutSegments, rankedShares, tokenMixSeries } from '$lib/usage-charts';
	import type { UsageRollupRow } from '$lib/types';

	// TokenMixDonut is the window's tokens split the way the ledger stores
	// them: a ring, and a legend carrying the share and the count. The rows
	// arrive as the days the trend draws, so the two cards beside each other
	// state one series.
	interface Props {
		rows: UsageRollupRow[];
		loading: boolean;
		pending: Snippet;
	}

	let { rows, loading, pending }: Props = $props();

	let hoveredKey = $state('');
	let drawn = $state(false);
	let played = false;

	const shares = $derived.by(() => {
		const points = tokenMixSeries(rows);
		const sum = (pick: (point: ReturnType<typeof tokenMixSeries>[number]) => number) =>
			points.reduce((total, point) => total + pick(point), 0);
		return rankedShares([
			{ key: 'summaryTokensIn', value: sum((point) => point.input) },
			{ key: 'summaryTokensOut', value: sum((point) => point.output) },
			{ key: 'cacheReadShort', value: sum((point) => point.cacheRead) },
			{ key: 'cacheWriteShort', value: sum((point) => point.cacheWrite) }
		]);
	});
	const segments = $derived(donutSegments(shares));
	const total = $derived(shares.reduce((sum, share) => sum + share.value, 0));
	const readout = $derived.by(() => {
		const hovered = shares.find((share) => share.key === hoveredKey);
		if (hovered)
			return $t('ui.pages.usagePage.' + hovered.key) + ' · ' + formatTokens(hovered.value);
		return formatTokens(total);
	});
	// SeriesKey names the shares the ring carries, so the readout fades when
	// the data changes while a hover swap stays instant.
	const seriesKey = $derived(
		shares.map((share) => share.key + ':' + share.share.toFixed(1)).join('|')
	);
	const hoveredSegment = $derived(segments.find((segment) => segment.key === hoveredKey));

	// The readout fades in once, with the first series the card is handed.
	$effect(() => {
		if (played || loading || segments.length === 0) return;
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
	<h2 class="section-heading">{$t('ui.pages.usagePage.tokenMixTitle')}</h2>
	{#if loading}
		{@render pending()}
	{:else if segments.length === 0}
		<div class="flex h-20 items-center justify-center text-xs text-muted-foreground">
			{$t('ui.pages.dashboard.noData')}
		</div>
	{:else}
		{#key seriesKey}
			<p
				class="mb-2 mt-3 min-h-5 text-sm font-medium tabular-nums text-ink"
				in:fade={{ duration: reducedMotion() || !drawn ? 0 : 120 }}
			>
				{readout}
			</p>
		{/key}
		<div class="flex flex-wrap items-center gap-4">
			<svg
				viewBox="0 0 140 140"
				class="h-36 w-36 shrink-0"
				role="img"
				aria-label={$t('ui.pages.usagePage.tokenMixTitle') + ' · ' + readout}
			>
				{#each segments as segment, index (segment.key)}
					<circle
						class="mix-segment"
						cx="70"
						cy="70"
						r="54"
						fill="none"
						stroke={donutFill(index)}
						stroke-width={hoveredKey === segment.key ? 20 : 15}
						pathLength={100}
						stroke-dasharray="{segment.sweep} 100"
						stroke-dashoffset={25 - segment.start}
						opacity={hoveredKey === '' || hoveredKey === segment.key ? 1 : 0.4}
						role="img"
						aria-label={$t('ui.pages.usagePage.' + segment.key) +
							': ' +
							segment.share.toFixed(1) +
							'% · ' +
							formatTokens(segment.value)}
						onmouseenter={() => (hoveredKey = segment.key)}
						onmouseleave={() => (hoveredKey = '')}
						><title
							>{$t('ui.pages.usagePage.' + segment.key)}: {segment.share.toFixed(1)}% · {formatTokens(
								segment.value
							)}</title
						></circle
					>
				{/each}
				<text x="70" y="68" text-anchor="middle" font-size="15" font-weight="650" fill="var(--ink)"
					>{formatTokens(hoveredSegment?.value ?? total)}</text
				>
				<text x="70" y="84" text-anchor="middle" font-size="10" fill="var(--muted)"
					>{$t('ui.pages.dashboard.trendTokens')}</text
				>
			</svg>
			<ul class="min-w-0 flex-1 space-y-1.5">
				{#each segments as segment, index (segment.key)}
					<li
						class="flex items-center gap-2 text-sm"
						animate:flip={{ duration: reducedMotion() ? 0 : 250 }}
						role="img"
						aria-label={$t('ui.pages.usagePage.' + segment.key) +
							': ' +
							segment.share.toFixed(1) +
							'%'}
						onmouseenter={() => (hoveredKey = segment.key)}
						onmouseleave={() => (hoveredKey = '')}
					>
						<span
							class="size-2.5 shrink-0 rounded-[4px]"
							style="background:{donutFill(index)}"
							aria-hidden="true"
						></span>
						<span class="min-w-0 flex-1 truncate">{$t('ui.pages.usagePage.' + segment.key)}</span>
						<span class="shrink-0 text-xs text-muted-foreground tabular-nums"
							>{segment.share.toFixed(1)}%</span
						>
						<span class="shrink-0 font-medium text-foreground tabular-nums"
							>{formatTokens(segment.value)}</span
						>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
</div>

<style>
	/* The token-mix donut glides to a new share when the window changes; the
	   hover widen is part of the same record. */
	.mix-segment {
		transition:
			stroke-dasharray 220ms cubic-bezier(0.2, 0.8, 0.2, 1),
			stroke-dashoffset 220ms cubic-bezier(0.2, 0.8, 0.2, 1),
			stroke-width 150ms ease,
			opacity 150ms ease;
	}

	@media (prefers-reduced-motion: reduce) {
		.mix-segment {
			transition: none;
		}
	}
</style>
