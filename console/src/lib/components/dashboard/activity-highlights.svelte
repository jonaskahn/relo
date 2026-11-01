<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import { formatCount } from '$lib/format';
	import type { ActivityDay, ActivityStreak } from '$lib/dashboard';
	import { shortDay } from '$lib/usage-charts';

	// ActivityHighlights is the narrow card beside the activity calendar: the
	// busiest day of the year and the longest and current runs of consecutive
	// days that carried traffic. Each row names the date it refers to.
	interface Props {
		busiest: ActivityDay | null;
		longest: ActivityStreak | null;
		current: ActivityStreak | null;
		loading: boolean;
		pending: Snippet;
	}

	let { busiest, longest, current, loading, pending }: Props = $props();

	function range(streak: ActivityStreak): string {
		return streak.start === streak.end
			? shortDay(streak.start)
			: shortDay(streak.start) + ' – ' + shortDay(streak.end);
	}
</script>

<div class="section-surface flex h-full min-w-0 flex-col divide-y divide-border/60 p-4">
	{#if loading}
		{@render pending()}
	{:else}
		<div class="flex min-h-0 flex-1 flex-col justify-center gap-1 py-2 first:pt-0 last:pb-0">
			<p
				class="flex min-w-0 items-center gap-1.5 text-[0.8125rem] font-medium text-muted-foreground"
			>
				<Icon name="trending-up" size={14} class="shrink-0 text-faint" />
				<span class="truncate">{$t('ui.pages.dashboard.activityBusiest')}</span>
			</p>
			<div class="flex items-baseline justify-between gap-2">
				<p class="mono-data leading-none font-medium tabular-nums text-ink">
					<span class="text-[1.375rem]">{busiest ? formatCount(busiest.value) : '—'}</span>
					<span class="ms-1 text-xs font-normal text-muted-foreground"
						>{$t('ui.pages.usagePage.unitRequests')}</span
					>
				</p>
				<p class="shrink-0 text-xs text-muted-foreground">
					{busiest ? shortDay(busiest.key) : $t('ui.pages.dashboard.noData')}
				</p>
			</div>
		</div>

		<div class="flex min-h-0 flex-1 flex-col justify-center gap-1 py-2 first:pt-0 last:pb-0">
			<p
				class="flex min-w-0 items-center gap-1.5 text-[0.8125rem] font-medium text-muted-foreground"
			>
				<Icon name="activity" size={14} class="shrink-0 text-faint" />
				<span class="truncate">{$t('ui.pages.dashboard.activityStreak')}</span>
			</p>
			<div class="flex items-baseline justify-between gap-2">
				<p class="mono-data leading-none font-medium tabular-nums text-ink">
					<span class="text-[1.375rem]">{longest?.days ?? '—'}</span>
					<span class="ms-1 text-xs font-normal text-muted-foreground"
						>{$t('ui.pages.dashboard.activityDayUnit')}</span
					>
				</p>
				<p class="shrink-0 text-xs text-muted-foreground">
					{longest ? range(longest) : $t('ui.pages.dashboard.noData')}
				</p>
			</div>
		</div>

		<div class="flex min-h-0 flex-1 flex-col justify-center gap-1 py-2 first:pt-0 last:pb-0">
			<p
				class="flex min-w-0 items-center gap-1.5 text-[0.8125rem] font-medium text-muted-foreground"
			>
				<Icon name="activity" size={14} class="shrink-0 text-faint" />
				<span class="truncate">{$t('ui.pages.dashboard.activityCurrentStreak')}</span>
			</p>
			<div class="flex items-baseline justify-between gap-2">
				<p class="mono-data leading-none font-medium tabular-nums text-ink">
					<span class="text-[1.375rem]">{current?.days ?? 0}</span>
					<span class="ms-1 text-xs font-normal text-muted-foreground"
						>{$t('ui.pages.dashboard.activityDayUnit')}</span
					>
				</p>
				<p class="shrink-0 text-xs text-muted-foreground">
					{current
						? $t('ui.pages.dashboard.activityStreakSince', {
								values: { date: shortDay(current.start) }
							})
						: $t('ui.pages.dashboard.activityNoStreak')}
				</p>
			</div>
		</div>
	{/if}
</div>
