<script lang="ts">
	import { tick } from 'svelte';
	import { flip } from 'svelte/animate';
	import { fly } from 'svelte/transition';
	import { t } from 'svelte-i18n';

	import { prefersReducedMotion } from '$lib/tab-motion';
	import RequestRow from '$lib/components/logs/request-row.svelte';
	import { formatTokens } from '$lib/format';
	import { groupRequestsByHour } from '$lib/process-logs';
	import type { UsageLogRow } from '$lib/types';

	// RequestList is one bordered panel of requests: a shared header, hour
	// bands, and one hairline row per request. The panel is the border; the
	// rows inside it do not each grow their own.

	interface Props {
		logs: UsageLogRow[];
		maxTokens?: number;
		nextCursor?: string;
		loadingMore?: boolean;
		onloadMore: () => void;
		onopen: (row: UsageLogRow) => void;
	}

	let {
		logs,
		maxTokens = 0,
		nextCursor = '',
		loadingMore = false,
		onloadMore,
		onopen
	}: Props = $props();

	// The sentinel watches the foot of the list. It re-arms after every page
	// (loadingMore and the row count change), so a sentinel still in view asks
	// for the next page until the cursor runs out. A failed page sets no new
	// rows, so it does not loop; Retry lives with the error.
	function watchMore(node: HTMLElement) {
		$effect(() => {
			void logs.length;
			if (loadingMore) return;
			const observer = new IntersectionObserver((entries) => {
				if (entries.some((entry) => entry.isIntersecting)) onloadMore();
			});
			observer.observe(node);
			return () => observer.disconnect();
		});
	}

	const groups = $derived(groupRequestsByHour(logs));
	// The rows on screen when the list first paints are the baseline. Only a
	// request newer than every one of them is a newcomer: it rises in and a
	// light runs once around its border. Older pages and filter reloads stay
	// still. Reduced motion skips all of it.
	let baseline = $state(Number.POSITIVE_INFINITY);
	const motion = $derived(!prefersReducedMotion());

	$effect(() => {
		if (logs.length === 0 || baseline !== Number.POSITIVE_INFINITY) return;
		baseline = Math.max(...logs.map((row) => row.ID));
	});

	const isFresh = (row: UsageLogRow) => motion && row.ID > baseline;

	const cols =
		'grid-cols-[6.5rem_minmax(0,2fr)_auto_5.5rem_5rem] sm:grid-cols-[6.5rem_minmax(0,2fr)_minmax(0,1fr)_auto_6rem_5rem_4.5rem]';
</script>

<div
	class="overflow-hidden rounded-card border border-line bg-surface max-sm:border-transparent max-sm:bg-transparent"
	role="table"
	aria-label={$t('ui.pages.logsPage.tabAgent')}
>
	<div
		class="hidden h-10 items-center gap-3.5 bg-canvas px-5 text-xs font-medium text-muted-foreground sm:grid {cols}"
		role="row"
	>
		<span>{$t('ui.pages.logsPage.columnTime')}</span>
		<span>{$t('ui.pages.logsPage.columnModel')}</span>
		<span class="hidden sm:block">{$t('ui.pages.logsPage.columnClient')}</span>
		<span>{$t('ui.pages.logsPage.columnStatus')}</span>
		<span class="hidden sm:block">{$t('ui.pages.logsPage.columnTokens')}</span>
		<span class="hidden text-right sm:block">{$t('ui.pages.logsPage.columnDuration')}</span>
		<span class="text-right">{$t('ui.pages.logsPage.columnCost')}</span>
	</div>

	{#each groups as group (group.key)}
		<div
			class="flex items-center justify-between bg-canvas px-5 py-1.5 text-xs font-medium text-muted-foreground max-sm:bg-transparent max-sm:px-0.5"
		>
			<span>{group.hourLabel}</span>
			<span class="font-normal">
				{group.requests.length}
				{$t('ui.pages.usagePage.unitRequests')} · {formatTokens(group.totalTokens)}
				{$t('ui.pages.usagePage.unitTokens')}
			</span>
		</div>
		{#each group.requests as row (row.ID)}
			<div
				animate:flip={{ duration: isFresh(row) || baseline !== Number.POSITIVE_INFINITY ? 200 : 0 }}
			>
				<div
					class="relative"
					class:row-fresh={isFresh(row)}
					in:fly={{ y: 16, duration: isFresh(row) ? 200 : 0 }}
				>
					<RequestRow {row} {maxTokens} {cols} {onopen} />
				</div>
			</div>
		{/each}
	{/each}

	{#if nextCursor}
		<div
			class="flex items-center justify-center px-5 py-4 text-xs text-muted-foreground"
			{@attach watchMore}
		>
			{loadingMore ? $t('ui.common.loading') : ''}
		</div>
	{/if}
</div>

<style>
	@property --run {
		syntax: '<angle>';
		inherits: false;
		initial-value: 0deg;
	}

	/* One light runs a full circle around a newcomer's border, then is gone.
	   The ring is a masked conic gradient, so only the 1px border shows. */
	.row-fresh::after {
		content: '';
		position: absolute;
		inset: 0;
		padding: 1px;
		pointer-events: none;
		background: conic-gradient(
			from var(--run),
			transparent 0 70%,
			color-mix(in srgb, var(--accent) 30%, transparent) 85%,
			var(--accent) 100%
		);
		mask:
			linear-gradient(#000 0 0) content-box,
			linear-gradient(#000 0 0);
		mask-composite: exclude;
		animation: row-run 1400ms linear forwards;
	}

	@keyframes row-run {
		from {
			--run: 0deg;
			opacity: 1;
		}
		85% {
			opacity: 1;
		}
		to {
			--run: 360deg;
			opacity: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.row-fresh::after {
			animation: none;
			display: none;
		}
	}
</style>
