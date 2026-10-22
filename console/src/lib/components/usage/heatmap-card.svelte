<script lang="ts">
	import { t } from 'svelte-i18n';

	import { formatCount } from '$lib/format';
	import { HEAT_FILLS, shortDay, type HeatCell } from '$lib/usage-charts';

	// HeatmapCard is the range as a compact calendar: weekdays across the top,
	// one row per Monday-first week, a month name where a week opens a new one,
	// and the day of the month in every cell. The fill is the activity ladder,
	// so a quiet day is as visible as a busy one.
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
					formatCount(hovered.value) +
					' ' +
					$t('ui.pages.usagePage.unitRequests')
				);
			}
		}
		const first = weeks[0]?.[0]?.key;
		const last = weeks[weeks.length - 1]?.[6]?.key;
		const span = first && last ? ' · ' + shortDay(first) + ' – ' + shortDay(last) : '';
		return formatCount(total) + ' ' + $t('ui.pages.usagePage.unitRequests') + span;
	});
	const weekdays = $derived(
		Array.from({ length: 7 }, (_, day) =>
			new Date(Date.UTC(2026, 8, 28 + day)).toLocaleDateString(undefined, {
				weekday: 'short',
				timeZone: 'UTC'
			})
		)
	);
	// monthMarks names the week that opens a new month, so the gutter reads as
	// a calendar rather than seven anonymous rows.
	const monthMarks = $derived.by(() => {
		let previous = '';
		return weeks.map((week) => {
			const key = week[0]?.key;
			if (!key) return '';
			const date = new Date(key + 'T00:00:00Z');
			if (Number.isNaN(date.getTime())) return '';
			const label = date.toLocaleDateString(undefined, { month: 'short', timeZone: 'UTC' });
			if (label === previous) return '';
			previous = label;
			return label;
		});
	});

	function dayNumber(key: string): string {
		const match = /^\d{4}-\d{2}-(\d{2})$/.exec(key);
		return match ? String(Number(match[1])) : '';
	}

	function cellInk(level: HeatCell['level']): string {
		if (level >= 3) return 'var(--on-accent)';
		if (level > 0) return 'var(--accent-ink)';
		return 'var(--muted)';
	}
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t('ui.pages.usagePage.heatmapTitle')}</h2>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.usagePage.unitRequests')}</p>
	</div>
	{#if weeks.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<p class="mb-3 min-h-5 text-sm font-medium text-ink tabular-nums">{readout}</p>
		<div class="overflow-x-auto pb-1">
			<div
				class="grid w-full items-center justify-items-center gap-1"
				style="grid-template-columns:2.75rem repeat(7,minmax(0,1fr))"
				role="group"
				aria-label={$t('ui.pages.usagePage.heatmapTitle') + ' · ' + readout}
			>
				<span aria-hidden="true"></span>
				{#each weekdays as weekday, day (day)}
					<span class="text-center text-xs text-muted-foreground" aria-hidden="true">{weekday}</span
					>
				{/each}
				{#each weeks as week, index (week[0]?.key ?? '')}
					<span
						class="flex items-center justify-self-start text-[0.6875rem] font-medium text-muted-foreground"
						aria-hidden="true">{monthMarks[index]}</span
					>
					{#each week as cell (cell.key)}
						<span
							class="flex aspect-square w-full max-w-7 min-w-0 items-center justify-center rounded-[5px] text-[0.6875rem] font-medium tabular-nums transition-[filter] hover:brightness-95 {cell.level ===
							0
								? 'border border-line'
								: ''}"
							style="background:{HEAT_FILLS[cell.level]};color:{cellInk(cell.level)}"
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
							onmouseleave={() => (hoveredKey = '')}>{dayNumber(cell.key)}</span
						>
					{/each}
				{/each}
			</div>
		</div>
		<div class="mt-3 flex items-center justify-end gap-1.5 text-xs text-muted-foreground">
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
