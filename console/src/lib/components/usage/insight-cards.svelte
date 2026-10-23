<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { formatDuration } from '$lib/format';
	import type { UsageInsight } from '$lib/usage-list';

	// InsightCards is the short reading above a grouped list: the group that
	// holds the most, the one with the highest rate, and the one that answers
	// slowest. The page decides which findings exist; a card only states one.
	interface Props {
		insights: UsageInsight[];
		shareLabel: string;
	}

	let { insights, shareLabel }: Props = $props();

	function iconOf(insight: UsageInsight): IconName {
		if (insight.kind === 'error_rate') return 'circle-alert';
		if (insight.kind === 'latency') return 'clock';
		if (insight.kind === 'requests') return 'list';
		return 'currency-dollar';
	}

	function labelOf(insight: UsageInsight): string {
		if (insight.kind === 'error_rate') return $t('ui.pages.usagePage.insightErrorRate');
		if (insight.kind === 'latency') return $t('ui.pages.usagePage.insightSlowest');
		if (insight.kind === 'requests') return $t('ui.pages.usagePage.insightMostRequests');
		return $t('ui.pages.usagePage.insightHighest', { values: { value: shareLabel } });
	}

	function valueOf(insight: UsageInsight): string {
		if (insight.kind === 'error_rate') return (insight.rate * 100).toFixed(1) + '%';
		if (insight.kind === 'latency') return formatDuration(Math.round(insight.averageMs));
		if (insight.kind === 'requests') return insight.entry.row.Requests.toLocaleString();
		return insight.entry.spend.text;
	}

	function detailOf(insight: UsageInsight): string {
		if (insight.kind === 'error_rate') {
			return (
				insight.entry.row.Errors.toLocaleString() +
				' ' +
				$t('ui.pages.usagePage.unitErrors') +
				' / ' +
				insight.entry.row.Requests.toLocaleString() +
				' ' +
				$t('ui.pages.usagePage.unitRequests')
			);
		}
		if (insight.kind === 'latency') {
			return $t('ui.pages.usagePage.noteSlowest', {
				values: { value: formatDuration(Math.max(0, Number(insight.entry.row.DurationMaxMs))) }
			});
		}
		if (insight.kind === 'requests') return $t('ui.pages.usagePage.unitRequests');
		return (insight.share * 100).toFixed(1) + '% ' + $t('ui.pages.usagePage.ofTotal');
	}
</script>

{#if insights.length > 0}
	<ul
		class="flex snap-x snap-mandatory gap-3 overflow-x-auto pb-1 max-sm:-mx-4 max-sm:px-4 [-ms-overflow-style:none] [scrollbar-width:none] sm:grid sm:grid-cols-3 sm:overflow-visible sm:pb-0 max-lg:grid-cols-1"
	>
		{#each insights as insight (insight.kind)}
			<li
				class="section-surface flex min-w-0 flex-[0_0_78%] snap-start items-start gap-3 p-4 sm:flex-auto"
			>
				<span
					class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent-soft text-accent-ink"
					aria-hidden="true"
				>
					<Icon name={iconOf(insight)} size={16} />
				</span>
				<div class="min-w-0">
					<p class="text-xs text-muted-foreground">{labelOf(insight)}</p>
					<p class="mt-0.5 break-words font-semibold leading-snug text-ink">
						{insight.entry.label}
					</p>
					<p class="mt-1 text-xs text-muted-foreground">
						<span class="mono-data font-medium text-ink">{valueOf(insight)}</span>
						· {detailOf(insight)}
					</p>
				</div>
			</li>
		{/each}
	</ul>
{/if}
