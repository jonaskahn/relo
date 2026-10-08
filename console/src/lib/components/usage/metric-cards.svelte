<script lang="ts">
	import { t } from 'svelte-i18n';

	import { metricLabelKey, type MetricValue } from '$lib/usage-metrics';

	// MetricCards is the chosen figures at the top of the overview. The page
	// owns which metrics are chosen and in what order; a card only states what
	// one of them reads.
	interface Props {
		cards: MetricValue[];
	}

	let { cards }: Props = $props();

	function cardLabel(card: MetricValue, unit: string): string {
		return card.unitKey ? card.exact + ' ' + unit : card.exact;
	}
</script>

{#if cards.length === 0}
	<div class="empty-panel section-surface border-dashed">
		{$t('ui.pages.usagePage.metricsEmpty')}
	</div>
{:else}
	<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
		{#each cards.slice(0, 12) as card (card.id)}
			{@const unit = card.unitKey ? $t('ui.pages.usagePage.' + card.unitKey) : ''}
			<div class="section-surface p-4" aria-label={cardLabel(card, unit)}>
				<p class="text-[0.8125rem] font-medium text-muted-foreground">
					{$t(metricLabelKey(card.id))}
				</p>
				<p
					class="mono-data mt-1 font-medium tabular-nums"
					style="font-size:1.625rem;line-height:1.2"
				>
					<span class:text-destructive={card.danger} title={cardLabel(card, unit)}>
						{card.value}
					</span>
					{#if card.unitKey}<span class="text-xs font-normal text-muted-foreground"
							>{$t('ui.pages.usagePage.' + card.unitKey)}</span
						>{/if}
				</p>
				{#if card.noteKey}
					<p class="mt-1 text-xs text-muted-foreground">
						{$t('ui.pages.usagePage.' + card.noteKey, { values: card.noteValues })}
					</p>
				{/if}
			</div>
		{/each}
	</div>
{/if}
