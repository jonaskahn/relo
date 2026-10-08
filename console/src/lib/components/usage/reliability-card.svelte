<script lang="ts">
	import { t } from 'svelte-i18n';

	import { barGeom, shortDay, type ReliabilityPoint } from '$lib/usage-charts';

	// ReliabilityCard is what did not answer: the errors and the retries behind
	// each day's requests.
	interface Props {
		points: ReliabilityPoint[];
	}

	let { points }: Props = $props();

	let hoveredKey = $state('');

	const max = $derived(Math.max(1, ...points.flatMap((point) => [point.errors, point.retried])));
	const requests = $derived(points.reduce((sum, point) => sum + point.requests, 0));
	const errors = $derived(points.reduce((sum, point) => sum + point.errors, 0));
	const readout = $derived.by(() => {
		const hovered = points.find((point) => point.key === hoveredKey);
		if (hovered) {
			return (
				shortDay(hovered.key) +
				' · ' +
				hovered.errors.toLocaleString() +
				' ' +
				$t('ui.pages.usagePage.unitErrors') +
				' · ' +
				hovered.retried.toLocaleString() +
				' ' +
				$t('ui.pages.usagePage.columnRetried').toLowerCase()
			);
		}
		const rate = requests > 0 ? ((errors / requests) * 100).toFixed(1) + '%' : '—';
		return (
			errors.toLocaleString() +
			' ' +
			$t('ui.pages.usagePage.unitErrors') +
			' · ' +
			$t('ui.pages.usagePage.summaryErrorRate') +
			' ' +
			rate
		);
	});
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t('ui.pages.usagePage.reliabilityTitle')}</h2>
		<p class="flex min-h-5 flex-wrap items-center gap-2 text-sm font-medium tabular-nums text-ink">
			{readout}
		</p>
	</div>
	{#if points.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<svg
			viewBox="0 0 640 84"
			class="h-24 w-full"
			preserveAspectRatio="none"
			role="img"
			aria-label={$t('ui.pages.usagePage.reliabilityTitle')}
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
				{@const slot = barGeom(index, points.length, max, max)}
				{@const half = Math.max(2, (slot.w - 3) / 2)}
				{@const hErr = Math.max(point.errors > 0 ? 3 : 0, (point.errors / max) * 80)}
				{@const hRet = Math.max(point.retried > 0 ? 3 : 0, (point.retried / max) * 80)}
				<g
					role="img"
					aria-label={shortDay(point.key) +
						': ' +
						point.errors +
						' ' +
						$t('ui.pages.usagePage.unitErrors')}
					onmouseenter={() => (hoveredKey = point.key)}
					onmouseleave={() => (hoveredKey = '')}
					opacity={hoveredKey === '' || hoveredKey === point.key ? 1 : 0.35}
				>
					<title>
						{shortDay(point.key)}: {point.errors.toLocaleString()}
						{$t('ui.pages.usagePage.unitErrors')} · {point.retried.toLocaleString()}
						{$t('ui.pages.usagePage.columnRetried').toLowerCase()}
					</title>
					<rect x={slot.x} y={84 - hErr} width={half} height={hErr} rx="1" fill="var(--danger)" />
					<rect
						x={slot.x + half + 3}
						y={84 - hRet}
						width={half}
						height={hRet}
						rx="1"
						fill="var(--accent-400)"
					/>
				</g>
			{/each}
		</svg>
	{/if}
	<ul class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
		<li class="inline-flex items-center gap-1.5">
			<span
				class="inline-block size-2 rounded-sm"
				style="background:var(--danger)"
				aria-hidden="true"
			></span>
			{$t('ui.pages.usagePage.columnErrors')}
		</li>
		<li class="inline-flex items-center gap-1.5">
			<span
				class="inline-block size-2 rounded-sm"
				style="background:var(--accent-400)"
				aria-hidden="true"
			></span>
			{$t('ui.pages.usagePage.columnRetried')}
		</li>
	</ul>
</div>
