<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import { formatCount } from '$lib/format';
	import { HEAT_FILLS, shortDay, type HeatCell } from '$lib/usage-charts';

	// ActivityCalendar is the dashboard's year of traffic as one GitHub-style
	// strip: 53 Monday-first week columns of 10px-tall cells, a month under the week
	// it opens, and a four-step accent ladder for what each day carried.
	interface Props {
		weeks: HeatCell[][];
		loading: boolean;
		pending: Snippet;
		/** The day the card should fill to, so the trailing week is not cut early. */
		today: string;
	}

	let { weeks, loading, pending, today }: Props = $props();

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
					formatCount(hovered.value) +
					' ' +
					$t('ui.pages.usagePage.unitRequests')
				);
			}
		}
		return formatCount(total) + ' ' + $t('ui.pages.usagePage.unitRequests');
	});
	// A month label spans the weeks it covers, so a label never has to fit in
	// one 10px column.
	const monthSpans = $derived.by(() => {
		const spans: Array<{ label: string; span: number }> = [];
		for (const week of weeks) {
			const key = week[0]?.key;
			if (!key) continue;
			const date = new Date(key + 'T00:00:00Z');
			if (Number.isNaN(date.getTime())) continue;
			const label = date.toLocaleDateString(undefined, { month: 'short', timeZone: 'UTC' });
			const last = spans[spans.length - 1];
			if (last && last.label === label) last.span += 1;
			else spans.push({ label, span: 1 });
		}
		return spans;
	});
	const weekdays = $derived(
		Array.from({ length: 7 }, (_, day) =>
			new Date(Date.UTC(2026, 8, 28 + day)).toLocaleDateString(undefined, {
				weekday: 'narrow',
				timeZone: 'UTC'
			})
		)
	);
</script>

<div class="section-surface flex min-w-0 flex-col p-4 max-sm:hidden sm:p-5">
	<div class="mb-2">
		<h2 class="section-heading flex items-center gap-2">
			<Icon name="activity" size={15} class="shrink-0 text-faint" />
			{$t('ui.pages.dashboard.activityTitle')}
		</h2>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.dashboard.activityDesc')}</p>
	</div>
	{#if loading}
		{@render pending()}
	{:else if weeks.length === 0}
		<div class="flex h-20 items-center justify-center text-xs text-muted-foreground">
			{$t('ui.pages.dashboard.noData')}
		</div>
	{:else}
		<p class="mono-data mb-2 min-h-5 font-medium text-ink tabular-nums">{readout}</p>
		<div class="overflow-x-auto pb-1">
			<div
				class="grid w-full gap-x-px gap-y-0.5"
				style="grid-template-columns:0.75rem repeat({weeks.length},minmax(0,1fr))"
				role="group"
				aria-label={$t('ui.pages.dashboard.activityTitle') + ' · ' + readout}
			>
				{#each weekdays as weekday, day (day)}
					<span class="text-xs leading-none text-muted-foreground" aria-hidden="true"
						>{weekday}</span
					>
					{#each weeks as week (week[0]?.key ?? '')}
						{@const cell = week[day]}
						{#if today === '' || cell.key <= today}
							<span
								class="h-2.5 w-full rounded-[2px] {cell.level === 0 ? 'border border-line' : ''}"
								style="background:{HEAT_FILLS[cell.level]}"
								role="img"
								aria-label={shortDay(cell.key) +
									': ' +
									formatCount(cell.value) +
									' ' +
									$t('ui.pages.usagePage.unitRequests')}
								title={shortDay(cell.key) +
									': ' +
									formatCount(cell.value) +
									' ' +
									$t('ui.pages.usagePage.unitRequests')}
								onmouseenter={() => (hoveredKey = cell.key)}
								onmouseleave={() => (hoveredKey = '')}
							></span>
						{:else}
							<span class="h-2.5 w-full" aria-hidden="true"></span>
						{/if}
					{/each}
				{/each}
				<span aria-hidden="true"></span>
				{#each monthSpans as month, index (index)}
					<span
						class="truncate ps-0.5 text-xs leading-none text-muted-foreground"
						style="grid-column: span {month.span}"
						aria-hidden="true">{month.label}</span
					>
				{/each}
			</div>
		</div>
		<div class="mt-2 flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
			<span>{$t('ui.pages.usagePage.heatmapLess')}</span>
			{#each HEAT_FILLS as fill (fill)}
				<span
					class="size-2.5 rounded-[3px] border border-line"
					style="background:{fill}"
					aria-hidden="true"
				></span>
			{/each}
			<span>{$t('ui.pages.usagePage.heatmapMore')}</span>
		</div>
	{/if}
</div>
