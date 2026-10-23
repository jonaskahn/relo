<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import { formatCount } from '$lib/format';
	import { HEAT_FILLS, shortDay, type HeatCell } from '$lib/usage-charts';
	import { newSnake, SNAKE_ROWS, stepSnake } from '$lib/snake';

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

	// Once the data is in, turns repeat: a snake crawls the grid and eats the active days,
	// the whole grid dissolves pixel by pixel, then the real heatmap fades back in and the
	// next turn starts. Reduced motion skips all of it.
	const SNAKE_DELAY_MS = 1500;
	const SNAKE_STEP_MS = 90;
	// The snake hunts greedily, so a sparse year could run long; cap each turn.
	const SNAKE_MAX_STEPS = 320;
	const DISSOLVE_TICK_MS = 45;
	const DISSOLVE_TICKS = 16;
	const FADE_IN_MS = 700;
	const TURN_PAUSE_MS = 2500;
	type Phase = 'idle' | 'play' | 'dissolve' | 'hidden' | 'fadein';
	let phase = $state<Phase>('idle');
	let body = $state<number[]>([]);
	let eaten = $state<Set<number>>(new Set());
	let tick = $state(0);
	let vanishAt: number[] = [];

	$effect(() => {
		if (loading || weeks.length === 0) return;
		if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;
		const open: boolean[] = [];
		const food = new Set<number>();
		weeks.forEach((week, col) =>
			week.forEach((cell, row) => {
				const index = col * SNAKE_ROWS + row;
				open[index] = today === '' || cell.key <= today;
				if (open[index] && cell.value > 0) food.add(index);
			})
		);
		if (food.size === 0) return;
		let timer: ReturnType<typeof setTimeout> | undefined;
		const later = (fn: () => void, ms: number) => (timer = setTimeout(fn, ms));

		const turn = () => {
			const game = newSnake(open, food, weeks.length, Math.random() < 0.5);
			let steps = 0;
			phase = 'play';
			const move = () => {
				stepSnake(game);
				steps += 1;
				body = [...game.body];
				eaten = new Set(game.eaten);
				if (game.done || steps >= SNAKE_MAX_STEPS) later(dissolve, 500);
				else later(move, SNAKE_STEP_MS);
			};
			move();
		};
		const dissolve = () => {
			vanishAt = Array.from({ length: weeks.length * SNAKE_ROWS }, () =>
				Math.floor(Math.random() * DISSOLVE_TICKS)
			);
			tick = 0;
			phase = 'dissolve';
			const advance = () => {
				tick += 1;
				if (tick >= DISSOLVE_TICKS) {
					phase = 'hidden';
					body = [];
					eaten = new Set();
					later(() => {
						phase = 'fadein';
						later(() => {
							phase = 'idle';
							later(turn, TURN_PAUSE_MS);
						}, FADE_IN_MS);
					}, 150);
				} else later(advance, DISSOLVE_TICK_MS);
			};
			later(advance, DISSOLVE_TICK_MS);
		};

		later(turn, SNAKE_DELAY_MS);
		return () => {
			clearTimeout(timer);
			phase = 'idle';
			body = [];
			eaten = new Set();
		};
	});

	function fill(cell: HeatCell, col: number, row: number): string {
		if (body.length === 0) return HEAT_FILLS[cell.level];
		const at = col * SNAKE_ROWS + row;
		if (at === body[0]) return 'var(--accent)';
		if (body.includes(at)) return 'var(--accent-400)';
		return eaten.has(at) ? HEAT_FILLS[0] : HEAT_FILLS[cell.level];
	}

	// A cell vanishes all at once, never fades, so the dissolve reads as pixels.
	function cellStyle(cell: HeatCell, col: number, row: number): string {
		const at = col * SNAKE_ROWS + row;
		const gone = phase === 'hidden' || (phase === 'dissolve' && tick >= (vanishAt[at] ?? 0));
		const fade = phase === 'fadein' ? 'opacity ' + FADE_IN_MS + 'ms ease-out' : 'none';
		return (
			'background:' + fill(cell, col, row) + ';opacity:' + (gone ? 0 : 1) + ';transition:' + fade
		);
	}

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
		if (first && last) return shortDay(first) + ' – ' + shortDay(last);
		return '';
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
	const weekRows = [0, 1, 2, 3, 4, 5, 6];
</script>

<div class="section-surface flex h-full min-w-0 flex-col p-4 max-sm:hidden sm:p-5">
	<div class="mb-2 flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
		<div class="min-w-0">
			<div class="flex flex-wrap items-baseline gap-x-3 gap-y-0.5">
				<h2 class="section-heading flex items-center gap-2">
					<Icon name="activity" size={15} class="shrink-0 text-faint" />
					{$t('ui.pages.dashboard.activityTitle')}
				</h2>
				{#if !loading && weeks.length > 0}
					<p class="mono-data min-w-0 truncate text-xs text-muted-foreground tabular-nums">
						{readout}
					</p>
				{/if}
			</div>
			<p class="text-xs text-muted-foreground">{$t('ui.pages.dashboard.activityDesc')}</p>
		</div>
		{#if !loading && weeks.length > 0}
			<span
				class="shrink-0 rounded-full bg-accent-soft px-2.5 py-1 text-xs font-medium text-accent-ink"
			>
				{formatCount(total)}
				{$t('ui.pages.usagePage.unitRequests')}
			</span>
		{/if}
	</div>
	{#if loading}
		{@render pending()}
	{:else if weeks.length === 0}
		<div class="flex h-20 items-center justify-center text-xs text-muted-foreground">
			{$t('ui.pages.dashboard.noData')}
		</div>
	{:else}
		<div class="flex flex-1 flex-col">
			<div class="min-h-0 flex-1 overflow-x-auto pb-1">
				<div
					class="grid h-full w-full gap-x-px gap-y-1"
					style="grid-template-columns:repeat({weeks.length},minmax(0,1fr));grid-template-rows:repeat(7,minmax(0,1fr)) auto"
					role="group"
					aria-label={$t('ui.pages.dashboard.activityTitle') + ' · ' + readout}
				>
					{#each weekRows as day (day)}
						{#each weeks as week, col (week[0]?.key ?? '')}
							{@const cell = week[day]}
							{#if today === '' || cell.key <= today}
								<span
									class="h-full min-h-2.5 w-full min-w-0 rounded-[3px]"
									style={cellStyle(cell, col, day)}
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
								<span class="h-full min-h-2.5 w-full" aria-hidden="true"></span>
							{/if}
						{/each}
					{/each}
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
					<span class="size-2.5 rounded-[4px]" style="background:{fill}" aria-hidden="true"></span>
				{/each}
				<span>{$t('ui.pages.usagePage.heatmapMore')}</span>
			</div>
		</div>
	{/if}
</div>
