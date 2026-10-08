<script lang="ts">
	import { t } from 'svelte-i18n';

	import { donutFill, type DonutSegment } from '$lib/usage-charts';

	// DonutCard ranks connections around one ring, with the list beside it
	// stating the same shares. The two donuts on the page differ only in what
	// they rank and how the figure reads.
	interface Props {
		titleKey: string;
		segments: DonutSegment[];
		totalText: string;
		captionOf: (key: string) => string;
		valueOf: (value: number) => string;
		centerKey: string;
	}

	let { titleKey, segments, totalText, captionOf, valueOf, centerKey }: Props = $props();

	let hoveredKey = $state('');

	const hovered = $derived(segments.find((segment) => segment.key === hoveredKey));
	const readout = $derived(
		hovered ? captionOf(hovered.key) + ' · ' + valueOf(hovered.value) : totalText
	);
</script>

<div class="section-surface p-4 sm:p-5">
	<div class="mb-3 flex flex-wrap items-center justify-between gap-3">
		<h2 class="section-heading">{$t(titleKey)}</h2>
	</div>
	{#if segments.length === 0}
		<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.empty')}</p>
	{:else}
		<p class="mb-2 min-h-5 text-sm font-medium text-ink tabular-nums">{readout}</p>
		<div class="flex flex-wrap items-center gap-4">
			<svg
				viewBox="0 0 140 140"
				class="h-36 w-36 shrink-0"
				role="img"
				aria-label={$t(titleKey) + ' · ' + readout}
			>
				{#each segments as segment, index (segment.key)}
					{@const caption = captionOf(segment.key)}
					<circle
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
						aria-label={caption + ': ' + segment.share.toFixed(1) + '% · ' + valueOf(segment.value)}
						onmouseenter={() => (hoveredKey = segment.key)}
						onmouseleave={() => (hoveredKey = '')}
						><title>{caption}: {segment.share.toFixed(1)}% · {valueOf(segment.value)}</title
						></circle
					>
				{/each}
				<text x="70" y="68" text-anchor="middle" font-size="15" font-weight="650" fill="var(--ink)"
					>{hovered ? valueOf(hovered.value) : totalText}</text
				>
				<text
					x="70"
					y="84"
					text-anchor="middle"
					font-size="10"
					fill="var(--muted)"
					textLength={hovered ? 76 : undefined}
					lengthAdjust="spacingAndGlyphs">{hovered ? captionOf(hovered.key) : $t(centerKey)}</text
				>
			</svg>
			<ul class="min-w-0 flex-1 space-y-1.5">
				{#each segments as segment, index (segment.key)}
					{@const caption = captionOf(segment.key)}
					<li
						class="flex items-center gap-2 text-sm"
						role="img"
						aria-label={caption + ': ' + segment.share.toFixed(1) + '% · ' + valueOf(segment.value)}
						onmouseenter={() => (hoveredKey = segment.key)}
						onmouseleave={() => (hoveredKey = '')}
					>
						<span
							class="size-2.5 shrink-0 rounded-[4px]"
							style="background:{donutFill(index)}"
							aria-hidden="true"
						></span>
						<span
							class="mono-data min-w-0 flex-1 truncate"
							title={caption !== segment.key ? segment.key : undefined}>{caption}</span
						>
						<span class="shrink-0 text-xs text-muted-foreground tabular-nums"
							>{segment.share.toFixed(1)}%</span
						>
						<span class="shrink-0 font-medium text-foreground tabular-nums"
							>{valueOf(segment.value)}</span
						>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
</div>
