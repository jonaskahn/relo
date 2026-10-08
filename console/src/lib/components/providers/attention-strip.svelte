<script lang="ts">
	import { onMount } from 'svelte';
	import { slide } from 'svelte/transition';
	import { t } from 'svelte-i18n';
	import Icon from '$lib/components/ui/icon.svelte';
	import { formatCountdown } from '$lib/dashboard';
	import { quotaWindowLabelKey } from '$lib/provider-quota';
	import type { AttentionItem } from '$lib/provider-attention';

	// The strip is one quiet status line, not a banner: a recessed band with
	// the count, then text items that read as links. Tone is carried by a 6px
	// dot and the words themselves, never by filled pills.
	interface Props {
		items: AttentionItem[];
		unpriced: number;
		onopen: (providerId: string) => void;
		onunpriced: () => void;
		// variant inline lays the same items in one scrolling row for a page
		// toolbar; band is the full block above it.
		variant?: 'band' | 'inline';
	}

	let { items, unpriced, onopen, onunpriced, variant = 'band' }: Props = $props();

	const count = $derived(items.length + (unpriced > 0 ? 1 : 0));
	const danger = $derived(items.some((item) => item.tone === 'danger'));
	let reducedMotion = $state(false);

	// The entrance eases the search beside it, so reduced motion parks it.
	onMount(() => {
		const media = window.matchMedia('(prefers-reduced-motion: reduce)');
		const update = () => (reducedMotion = media.matches);
		update();
		media.addEventListener('change', update);
		return () => media.removeEventListener('change', update);
	});

	function windowLabel(window: string): string {
		const key = quotaWindowLabelKey(window);
		return key === '' ? window : $t(key);
	}

	function resetIn(resetAtMs: number): string {
		if (resetAtMs <= 0) return '';
		return formatCountdown(resetAtMs - Date.now());
	}

	function chipText(item: AttentionItem): string {
		const window = windowLabel(item.window);
		const reset = resetIn(item.resetAtMs);
		if (item.tone === 'danger') {
			return reset
				? $t('ui.pages.providersPage.list.limitChip', {
						values: { window, reset }
					})
				: $t('ui.pages.providersPage.list.limitChipNoReset', { values: { window } });
		}
		return reset
			? $t('ui.pages.providersPage.list.nearChip', {
					values: { window, percent: item.percent, reset }
				})
			: $t('ui.pages.providersPage.list.nearChipNoReset', {
					values: { window, percent: item.percent }
				});
	}
</script>

{#if count > 0}
	{#if variant === 'inline'}
		<div
			transition:slide={{ duration: reducedMotion ? 0 : 180, axis: 'x' }}
			class="section-surface flat-surface attention-marquee flex min-w-0 flex-1 basis-48 items-center gap-3 px-3 py-2 max-sm:hidden"
			role="status"
		>
			<span class="flex shrink-0 items-center gap-2 text-sm font-semibold whitespace-nowrap">
				<Icon name="alert-triangle" size={15} class={danger ? 'text-danger' : 'text-warn'} />
				{$t('ui.pages.providersPage.list.attentionCount', { values: { count } })}
			</span>
			<div class="attention-viewport">
				<ul class="attention-track">
					{#each items as item (item.providerId)}
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
								<span class="max-w-40 truncate font-medium text-ink">{item.label}</span>
								<span class="text-muted-foreground group-hover:text-ink">{chipText(item)}</span>
							</button>
						</li>
					{/each}
					{#if unpriced > 0}
						<li class="shrink-0">
							<button
								type="button"
								data-plain
								class="group flex items-center gap-2 rounded-control text-sm whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
								onclick={onunpriced}
							>
								<span class="size-1.5 shrink-0 rounded-full bg-warn" aria-hidden="true"></span>
								<span class="text-muted-foreground group-hover:text-ink">
									{$t('ui.pages.providersPage.list.unpricedChip', { values: { count: unpriced } })}
								</span>
							</button>
						</li>
					{/if}
					{#each items as item ('copy-' + item.providerId)}
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
								<span class="max-w-40 truncate font-medium text-ink">{item.label}</span>
								<span class="text-muted-foreground group-hover:text-ink">{chipText(item)}</span>
							</button>
						</li>
					{/each}
					{#if unpriced > 0}
						<li class="shrink-0" aria-hidden="true">
							<button
								type="button"
								data-plain
								tabindex="-1"
								class="group flex items-center gap-2 rounded-control text-sm whitespace-nowrap focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
								onclick={onunpriced}
							>
								<span class="size-1.5 shrink-0 rounded-full bg-warn" aria-hidden="true"></span>
								<span class="text-muted-foreground group-hover:text-ink">
									{$t('ui.pages.providersPage.list.unpricedChip', { values: { count: unpriced } })}
								</span>
							</button>
						</li>
					{/if}
				</ul>
			</div>
		</div>
	{:else}
		<div
			class="section-surface flat-surface flex flex-wrap items-center gap-x-6 gap-y-2 px-4 py-2.5 max-sm:hidden"
			role="status"
		>
			<span class="flex shrink-0 items-center gap-2 text-sm font-semibold">
				<Icon name="alert-triangle" size={15} class={danger ? 'text-danger' : 'text-warn'} />
				{$t('ui.pages.providersPage.list.attentionCount', { values: { count } })}
			</span>
			<ul class="flex min-w-0 flex-1 flex-wrap items-center gap-x-6 gap-y-1.5">
				{#each items as item (item.providerId)}
					<li>
						<button
							type="button"
							data-plain
							class="group flex items-center gap-2 rounded-control text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
							onclick={() => onopen(item.providerId)}
						>
							<span
								class="size-1.5 shrink-0 rounded-full {item.tone === 'danger'
									? 'bg-danger'
									: 'bg-warn'}"
								aria-hidden="true"
							></span>
							<span class="font-medium text-ink">{item.label}</span>
							<span class="text-muted-foreground group-hover:text-ink">{chipText(item)}</span>
						</button>
					</li>
				{/each}
				{#if unpriced > 0}
					<li>
						<button
							type="button"
							data-plain
							class="group flex items-center gap-2 rounded-control text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
							onclick={onunpriced}
						>
							<span class="size-1.5 shrink-0 rounded-full bg-warn" aria-hidden="true"></span>
							<span class="text-muted-foreground group-hover:text-ink">
								{$t('ui.pages.providersPage.list.unpricedChip', { values: { count: unpriced } })}
							</span>
						</button>
					</li>
				{/if}
			</ul>
		</div>
	{/if}
{/if}
