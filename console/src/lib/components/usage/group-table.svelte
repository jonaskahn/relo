<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import { formatDuration, formatTokens, formatTokensExact } from '$lib/format';
	import type { SpendReadout } from '$lib/usage-metrics';
	import type { UsageRollupRow } from '$lib/types';

	// GroupTable is the roll-up under a group-by tab: one card per group, its
	// share of the largest bar, and what it carried. The page decides the order
	// and what each row's spend states.
	interface Props {
		rows: UsageRollupRow[];
		captionOf: (key: string) => string;
		titleOf: (key: string) => string | undefined;
		spendOf: (row: UsageRollupRow) => SpendReadout;
	}

	let { rows, captionOf, titleOf, spendOf }: Props = $props();

	const maxWeight = $derived(rows.reduce((worst, row) => Math.max(worst, spendOf(row).weight), 0));

	function shareOf(row: UsageRollupRow): number {
		if (maxWeight <= 0) return 0;
		return Math.max(2, (spendOf(row).weight / maxWeight) * 100);
	}

	function averageDuration(row: UsageRollupRow): string {
		return row.Requests > 0 ? formatDuration(Math.round(row.DurationMs / row.Requests)) : '—';
	}
</script>

<ul class="flex flex-col gap-2">
	{#each rows as row (row.Key)}
		{@const spend = spendOf(row)}
		<li class="section-surface p-4">
			<div class="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
				<span
					class="mono-data min-w-0 flex-1 truncate text-sm font-medium"
					title={titleOf(row.Key)}
				>
					{captionOf(row.Key)}
				</span>
				<span class="mono-data shrink-0 text-sm font-medium tabular-nums">{spend.text}</span>
			</div>
			<div
				class="mt-2 h-[7px] w-full rounded-full"
				style="background:var(--accent-100)"
				aria-hidden="true"
			>
				<div
					class="h-full rounded-full"
					style="width: {shareOf(row)}%;background:var(--accent)"
				></div>
			</div>
			<div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
				<span>
					<span class="mono-data text-foreground tabular-nums">{row.Requests.toLocaleString()}</span
					>
					{$t('ui.pages.usagePage.unitRequests')}
				</span>
				{#if row.Errors > 0}
					<span class="inline-flex items-center gap-1 text-destructive">
						<Icon name="circle-alert" size={12} />
						<span class="mono-data tabular-nums">{row.Errors.toLocaleString()}</span>
						{$t('ui.pages.usagePage.summaryErrors').toLowerCase()}
					</span>
				{:else}
					<span>
						<span class="mono-data tabular-nums">0</span>
						{$t('ui.pages.usagePage.summaryErrors').toLowerCase()}
					</span>
				{/if}
				<span>
					<span
						class="mono-data text-foreground tabular-nums"
						title={formatTokensExact(row.InputTokens)}>{formatTokens(row.InputTokens)}</span
					>
					{$t('ui.pages.usagePage.summaryTokensIn').toLowerCase()}
					{' · '}
					<span
						class="mono-data text-foreground tabular-nums"
						title={formatTokensExact(row.OutputTokens)}>{formatTokens(row.OutputTokens)}</span
					>
					{$t('ui.pages.usagePage.summaryTokensOut').toLowerCase()}
				</span>
				<span>
					<span
						class="mono-data text-foreground tabular-nums"
						title={formatTokensExact(row.CacheReadTokens)}>{formatTokens(row.CacheReadTokens)}</span
					>
					{' / '}
					<span
						class="mono-data text-foreground tabular-nums"
						title={formatTokensExact(row.CacheWriteTokens)}
						>{formatTokens(row.CacheWriteTokens)}</span
					>
					{$t('ui.pages.usagePage.summaryCache').toLowerCase()}
				</span>
				<span>
					<span class="mono-data text-foreground tabular-nums">{averageDuration(row)}</span>
					{$t('ui.pages.usagePage.columnAvgDuration').toLowerCase()}
					{' · '}
					<span class="mono-data text-foreground tabular-nums"
						>{row.DurationMaxMs > 0 ? formatDuration(row.DurationMaxMs) : '—'}</span
					>
					{$t('ui.pages.usagePage.columnMaxDuration').toLowerCase()}
				</span>
				{#if row.Attempts > 0}
					<span>
						<span class="mono-data text-foreground tabular-nums"
							>{row.Attempts.toLocaleString()}</span
						>
						{$t('ui.pages.usagePage.unitAttempts')}
						{#if row.RetriedRequests > 0}
							{' · '}
							<span class="mono-data text-foreground tabular-nums"
								>{row.RetriedRequests.toLocaleString()}</span
							>
							{$t('ui.pages.usagePage.columnRetried').toLowerCase()}
						{/if}
					</span>
				{/if}
			</div>
		</li>
	{/each}
</ul>
