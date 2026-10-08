<script lang="ts">
	import { t } from 'svelte-i18n';

	import { HEAT_FILLS, shortDay, type HeatCell } from '$lib/usage-charts';

	// HeatmapCard folds the range into Monday-first weeks, so a rhythm that
	// spans months still reads as one grid.
	interface Props {
		weeks: HeatCell[][];
	}

	let { weeks }: Props = $props();

	let hoveredKey = $state('');

	const total = $derived(
		weeks.reduce((sum, week) => sum + week.reduce((day, cell) => day + cell.value, 0), 0)
	);
	const readout = $derived.by(() => {
		for (const week of weeks) {
			const hovered = week.find((cell) => cell.key === hoveredKey);
			if (hovered) {
				return (
					shortDay(hovered.key) +
					' · ' +
					hovered.value.toLocaleString() +
					' ' +
					$t('ui.pages.usagePage.unitRequests')
				);
			}
		}
		return total.toLocaleString() + ' ' + $t('ui.pages.usagePage.unitRequests');
	});
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t('ui.pages.usagePage.heatmapTitle')}</h2>
	</div>
	{#if weeks.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<p class="mb-2 min-h-5 text-sm font-medium text-ink tabular-nums">{readout}</p>
		<div
			class="flex gap-1 overflow-x-auto pb-1"
			role="img"
			aria-label={$t('ui.pages.usagePage.heatmapTitle') + ' · ' + readout}
		>
			{#each weeks as week (week[0]?.key ?? '')}
				<div class="flex shrink-0 flex-col gap-1">
					{#each week as cell (cell.key)}
						<span
							class="size-3 rounded-[4px] {cell.level === 0 ? 'border border-line' : ''}"
							style="background:{HEAT_FILLS[cell.level]}"
							role="img"
							aria-label={shortDay(cell.key) +
								': ' +
								cell.value.toLocaleString() +
								' ' +
								$t('ui.pages.usagePage.unitRequests')}
							title={shortDay(cell.key) +
								': ' +
								cell.value.toLocaleString() +
								' ' +
								$t('ui.pages.usagePage.unitRequests')}
							onmouseenter={() => (hoveredKey = cell.key)}
							onmouseleave={() => (hoveredKey = '')}
						></span>
					{/each}
				</div>
			{/each}
		</div>
		<div class="mt-2 flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
			<span>{$t('ui.pages.usagePage.heatmapLess')}</span>
			{#each HEAT_FILLS as fill (fill)}
				<span
					class="size-3 rounded-[4px] border border-line"
					style="background:{fill}"
					aria-hidden="true"
				></span>
			{/each}
			<span>{$t('ui.pages.usagePage.heatmapMore')}</span>
		</div>
	{/if}
</div>
