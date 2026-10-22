<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import { formatCountdown, type AccountAttention } from '$lib/dashboard';
	import { quotaWindowLabelKey } from '$lib/provider-quota';

	// AttentionBand is the line of accounts that need an operator, read before the
	// figures below: a sign-in that expired, a window that is full, a window that is
	// nearly full. Tone is carried by a 6px dot and the words themselves, never by a
	// filled pill. The accounts run in one scrolling row, so the band stays a single
	// line however many accounts need an operator.

	interface Props {
		items: AccountAttention[];
		onopen: (providerId: string) => void;
	}

	let { items, onopen }: Props = $props();

	const danger = $derived(items.some((item) => item.tone === 'danger'));

	function windowLabel(window: string): string {
		const key = quotaWindowLabelKey(window);
		return key === '' ? window : $t(key);
	}

	function reason(item: AccountAttention): string {
		if (item.reason === 'needs_signin') return $t('ui.pages.dashboard.attentionNeedsSignin');
		if (item.reason === 'paused') return $t('ui.pages.dashboard.attentionPaused');
		if (item.reason === 'limited') return $t('ui.pages.dashboard.attentionLimited');
		const window = item.window;
		if (!window) return '';
		const label = windowLabel(window.window);
		if (item.reason === 'near') {
			return $t('ui.pages.dashboard.attentionNear', {
				values: { window: label, percent: Math.round(window.used_percent) }
			});
		}
		const reset = formatCountdown(window.reset_at_ms - Date.now());
		return reset
			? $t('ui.pages.dashboard.attentionReached', { values: { window: label, time: reset } })
			: $t('ui.pages.dashboard.attentionReachedNoReset', { values: { window: label } });
	}
</script>

{#if items.length > 0}
	<div
		class="section-surface flat-surface attention-marquee flex min-w-0 items-center gap-x-6 px-4 py-2.5 max-sm:hidden"
		role="status"
	>
		<span class="flex shrink-0 items-center gap-2 text-sm font-semibold whitespace-nowrap">
			<Icon name="alert-triangle" size={15} class={danger ? 'text-danger' : 'text-warn'} />
			{$t('ui.pages.dashboard.attentionCount', { values: { count: items.length } })}
		</span>
		<div class="attention-viewport">
			<ul class="attention-track">
				{#each items as item (item.account.id)}
					<li class="shrink-0">
						<button
							type="button"
							data-plain
							class="group flex items-center gap-2 rounded-control text-sm whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
							onclick={() => onopen(item.providerId)}
						>
							<span
								class="size-1.5 shrink-0 rounded-full {item.tone === 'danger'
									? 'bg-danger'
									: 'bg-warn'}"
								aria-hidden="true"
							></span>
							<span class="max-w-40 truncate font-medium text-ink">{item.account.label}</span>
							<span class="text-muted-foreground group-hover:text-ink">{reason(item)}</span>
						</button>
					</li>
				{/each}
				{#each items as item ('copy-' + item.account.id)}
					<li class="shrink-0" aria-hidden="true">
						<button
							type="button"
							data-plain
							tabindex="-1"
							class="group flex items-center gap-2 rounded-control text-sm whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
							onclick={() => onopen(item.providerId)}
						>
							<span
								class="size-1.5 shrink-0 rounded-full {item.tone === 'danger'
									? 'bg-danger'
									: 'bg-warn'}"
								aria-hidden="true"
							></span>
							<span class="max-w-40 truncate font-medium text-ink">{item.account.label}</span>
							<span class="text-muted-foreground group-hover:text-ink">{reason(item)}</span>
						</button>
					</li>
				{/each}
			</ul>
		</div>
	</div>
{/if}
