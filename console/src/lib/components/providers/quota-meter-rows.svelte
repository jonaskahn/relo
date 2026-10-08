<script lang="ts">
	import { locale, t } from 'svelte-i18n';
	import type { Snippet } from 'svelte';
	import type { QuotaWindow } from '$lib/types';
	import { consoleState } from '$lib/console-state.svelte';
	import { formatCountdown } from '$lib/dashboard';
	import {
		QUOTA_RINGS,
		quotaValueKey,
		quotaWindowLabelKey,
		ringsOf,
		shownPercent
	} from '$lib/provider-quota';

	// QuotaMeterRows is the compact load reading shared by the connection
	// overview and the Accounts tab. It always paints four tracks so a card
	// with one window is the same height as a card with four.
	interface Props {
		windows?: QuotaWindow[];
		// showSource adds the probe-or-traffic note the Accounts tab keeps
		// under each window. The overview card already names the account.
		showSource?: boolean;
		// empty sits on the reserved tracks when no window is drawn: a missing
		// account, a sign-in action, or a window that has not arrived yet.
		empty?: Snippet;
	}

	let { windows = [], showSource = false, empty }: Props = $props();

	type Track =
		| { key: string; type: 'window'; window: QuotaWindow }
		| { key: string; type: 'extra'; extra: number }
		| { key: string; type: 'empty' };

	const rings = $derived(ringsOf(windows));
	const tracks = $derived.by((): Track[] => {
		const extra = rings.extra;
		const shown = extra > 0 ? rings.shown.slice(0, QUOTA_RINGS - 1) : rings.shown;
		return Array.from({ length: QUOTA_RINGS }, (_, index): Track => {
			if (extra > 0 && index === QUOTA_RINGS - 1) {
				return { key: 'extra', type: 'extra', extra };
			}
			const window = shown[index];
			return window
				? { key: window.window, type: 'window', window }
				: { key: 'empty-' + index, type: 'empty' };
		});
	});
	const hasMeters = $derived(tracks.some((track) => track.type === 'window'));

	function windowLabel(window: string): string {
		const key = quotaWindowLabelKey(window);
		return key === '' ? window : $t(key);
	}

	function resetIn(resetAtMs: number): string {
		if (resetAtMs <= 0) return '';
		return formatCountdown(resetAtMs - Date.now());
	}

	function sourceLabel(source: string): string {
		return source === 'header'
			? $t('ui.pages.providersPage.accounts.quotaFromTraffic')
			: $t('ui.pages.providersPage.accounts.quotaFromProbe');
	}

	// Under 2% still paints a 2% sliver, and a true zero paints nothing.
	function fillWidth(percent: number): string {
		if (percent <= 0) return '0%';
		return Math.max(2, percent).toFixed(2) + '%';
	}

	// Quota colour steps with the value, per cards-and-quota: accent under
	// 90%, warn to 99%, danger at a full window. The status pill says the
	// same thing in words, so colour is never the only signal.
	function barTone(usedPercent: number): string {
		if (usedPercent >= 100) return 'bg-danger';
		if (usedPercent >= 90) return 'bg-warn';
		return 'bg-primary';
	}

	function numberTone(usedPercent: number): string {
		if (usedPercent >= 100) return 'text-danger';
		if (usedPercent >= 90) return 'text-warn';
		return '';
	}

	function money(amount: number, currency: string): string {
		return new Intl.NumberFormat($locale ?? 'en', {
			style: 'currency',
			currency,
			maximumFractionDigits: 2
		}).format(amount);
	}
</script>

<div class="relative flex flex-col gap-1.5">
	{#each tracks as track (track.key)}
		{#if track.type === 'window'}
			<div data-quota-track class="flex min-h-4 flex-col gap-0.5">
				{#if track.window.amount !== undefined && track.window.currency}
					<div class="flex min-h-4 items-center gap-2">
						<span class="w-16 shrink-0 truncate text-muted-foreground"
							>{windowLabel(track.window.window)}</span
						>
						<span
							class="min-w-0 flex-1 text-end font-mono text-[0.7rem] font-semibold tabular-nums"
						>
							{money(track.window.amount, track.window.currency)}
						</span>
					</div>
				{:else}
					{@const percent = shownPercent(track.window.used_percent, consoleState.quotaDisplay)}
					{@const remaining = resetIn(track.window.reset_at_ms)}
					{@const resetsIn = remaining
						? $t('ui.pages.providersPage.accounts.quotaResetsIn', { values: { time: remaining } })
						: ''}
					{@const reading = $t(quotaValueKey(consoleState.quotaDisplay), {
						values: { percent: Math.round(percent) }
					})}
					<div class="flex min-h-4 items-center gap-2">
						<span class="w-16 shrink-0 truncate text-muted-foreground"
							>{windowLabel(track.window.window)}</span
						>
						<span
							class="w-8 shrink-0 text-end font-mono text-[0.7rem] font-semibold tabular-nums {numberTone(
								track.window.used_percent
							)}"
						>
							{Math.round(percent)}%
						</span>
						<div
							class="h-[7px] min-w-0 flex-1 overflow-hidden rounded-full bg-accent-line"
							role="progressbar"
							aria-valuenow={Math.round(percent)}
							aria-valuemin={0}
							aria-valuemax={100}
							aria-label={resetsIn ? reading + ' · ' + resetsIn : reading}
						>
							<div
								class="quota-fill h-full rounded-full {barTone(track.window.used_percent)}"
								style="--pct: {fillWidth(percent)}"
							></div>
						</div>
						<span
							class="w-12 shrink-0 text-end text-[0.7rem] whitespace-nowrap text-muted-foreground"
							title={resetsIn}
						>
							{remaining || '\u00a0'}
						</span>
					</div>
				{/if}
				{#if showSource}
					<div
						class="flex min-h-3.5 flex-wrap items-center gap-x-2 text-[0.7rem] text-muted-foreground"
					>
						<span>{sourceLabel(track.window.source)}</span>
					</div>
				{/if}
			</div>
		{:else if track.type === 'extra'}
			<div data-quota-track class="flex min-h-4 flex-col gap-0.5">
				<div class="flex min-h-4 items-center">
					<span class="text-[0.7rem] text-muted-foreground">
						{$t('ui.pages.providersPage.accounts.quotaMoreWindows', {
							values: { count: track.extra }
						})}
					</span>
				</div>
				{#if showSource}
					<div class="min-h-3.5" aria-hidden="true">&nbsp;</div>
				{/if}
			</div>
		{:else}
			<div data-quota-track class="flex min-h-4 flex-col gap-0.5" aria-hidden="true">
				<div class="flex min-h-4 items-center gap-2">
					<span class="w-16 shrink-0">&nbsp;</span>
					<span class="w-8 shrink-0">&nbsp;</span>
					<div class="h-1.5 min-w-0 flex-1"></div>
					<span class="w-12 shrink-0">&nbsp;</span>
				</div>
				{#if showSource}
					<div class="min-h-3.5">&nbsp;</div>
				{/if}
			</div>
		{/if}
	{/each}
	{#if empty && !hasMeters}
		<div class="absolute inset-0 flex flex-col items-center justify-center">
			{@render empty()}
		</div>
	{/if}
</div>

<style>
	/* The fill runs once, when the meter mounts, so a reading that changes
	   under a live update moves without replaying the reveal. */
	.quota-fill {
		width: 0;
		animation: quota-fill 400ms cubic-bezier(0.2, 0.8, 0.2, 1) forwards;
	}

	@keyframes quota-fill {
		to {
			width: var(--pct);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.quota-fill {
			width: var(--pct);
			animation: none;
		}
	}
</style>
