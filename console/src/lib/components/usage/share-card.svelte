<script lang="ts">
	import { t } from 'svelte-i18n';

	import Segmented from '$lib/components/ui/segmented.svelte';
	import { donutFill } from '$lib/usage-charts';
	import { SHARE_OTHER_KEY, type UsageShareSegment } from '$lib/usage-list';
	import type { IconName } from '$lib/components/ui/icon.svelte';

	// ShareCard compares the groups of one tab on a single horizontal strip.
	// Shares are taken against the whole grouping, so the tail is named rather
	// than silently dropped.
	interface Props {
		segments: UsageShareSegment[];
		total: string;
		metricLabel: string;
		metricValue: 'requests' | 'tokens' | 'spend' | 'errors';
		options: Array<{
			value: 'requests' | 'tokens' | 'spend' | 'errors';
			label: string;
			icon: IconName;
		}>;
		formatValue: (value: number, metric: 'requests' | 'tokens' | 'spend' | 'errors') => string;
		groupLabel: string;
		onmetric: (metric: 'requests' | 'tokens' | 'spend' | 'errors') => void;
	}

	let {
		segments,
		total,
		metricLabel,
		metricValue,
		options,
		formatValue,
		groupLabel,
		onmetric
	}: Props = $props();

	function labelOf(segment: UsageShareSegment): string {
		if (segment.key !== SHARE_OTHER_KEY) return segment.label;
		return $t('ui.pages.usagePage.shareOther', { values: { count: segment.count ?? 0 } });
	}

	function shareText(share: number): string {
		if (share > 0 && share < 0.1) return '<0.1%';
		return share.toFixed(share < 10 ? 1 : 0) + '%';
	}
</script>

<section class="section-surface p-4 sm:p-5" aria-labelledby="usage-share-title">
	<div class="mb-3 flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0">
			<h2 id="usage-share-title" class="section-heading">
				{$t('ui.pages.usagePage.shareTitle', {
					values: { metric: metricLabel.toLowerCase(), group: groupLabel.toLowerCase() }
				})}
			</h2>
			<p class="mt-1 text-xs text-muted-foreground">
				{$t('ui.pages.usagePage.shareCaption', { values: { total } })}
			</p>
		</div>
		<div class="max-w-full overflow-x-auto">
			<Segmented
				value={metricValue}
				{options}
				ariaLabel={$t('ui.pages.usagePage.shareMetric')}
				onchange={onmetric}
			/>
		</div>
	</div>

	{#if segments.length === 0}
		<p class="text-sm text-muted-foreground">{$t('ui.pages.usagePage.shareEmpty')}</p>
	{:else}
		<div
			class="flex h-7 gap-0.5 overflow-hidden rounded-control bg-sunken"
			role="img"
			aria-label={$t('ui.pages.usagePage.shareTitle', {
				values: { metric: metricLabel.toLowerCase(), group: groupLabel.toLowerCase() }
			})}
		>
			{#each segments as segment, index (segment.key)}
				<span
					class="min-w-[3px] first:rounded-s-control last:rounded-e-control"
					style="flex:{segment.value};background:{donutFill(index)}"
					title={labelOf(segment) +
						': ' +
						formatValue(segment.value, metricValue) +
						' · ' +
						shareText(segment.share)}
				></span>
			{/each}
		</div>

		<ul class="mt-4 grid gap-x-7 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
			{#each segments as segment, index (segment.key)}
				<li class="grid min-w-0 grid-cols-[10px_minmax(0,1fr)_auto] items-baseline gap-x-2.5">
					<span class="size-2.5 rounded-sm" style="background:{donutFill(index)}" aria-hidden="true"
					></span>
					<span class="truncate font-medium" title={labelOf(segment)}>{labelOf(segment)}</span>
					<span class="mono-data text-xs text-muted-foreground">{shareText(segment.share)}</span>
					<span class="col-span-2 col-start-2 text-xs text-muted-foreground">
						<span class="mono-data">{formatValue(segment.value, metricValue)}</span>
					</span>
				</li>
			{/each}
		</ul>
	{/if}
</section>
