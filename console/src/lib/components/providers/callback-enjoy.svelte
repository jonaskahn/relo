<script lang="ts">
	import { cubicOut } from 'svelte/easing';
	import { t } from 'svelte-i18n';
	import type { TransitionConfig } from 'svelte/transition';

	import Icon from '$lib/components/ui/icon.svelte';
	import { prefersReducedMotion } from '$lib/tab-motion';

	interface Props {
		/** Whether the login stored an account. Every other outcome reads as the
		 *  case where the operator starts again. */
		ok: boolean;
		/** The account a finished login stored, when it named one. */
		account?: string;
	}

	let { ok, account = '' }: Props = $props();

	/** The panel arrives on a page the wind has already cleared, so there is
	 *  nothing to cross-fade against: it rises into an empty canvas. The scale
	 *  is what makes it read as arriving rather than appearing.
	 *
	 *  Over the 400ms every other moment is capped at, because there is no edge
	 *  here to read and no competing motion — what it needs is enough time for
	 *  the rise and the scale to finish rather than be cut off. */
	function enjoyMotion(_node: HTMLElement): TransitionConfig {
		if (prefersReducedMotion()) return { duration: 140, css: (t) => `opacity: ${t}` };
		return {
			duration: 1000,
			easing: cubicOut,
			css: (t) => `opacity: ${t}; transform: translateY(${(1 - t) * 8}px) scale(${0.96 + t * 0.04})`
		};
	}
</script>

<!-- What the page says once the wind has taken the result away: one mark, the
     account if there is one, and either the fact that it worked or the one
     thing left to do. The tab closes itself a moment later either way, so
     nothing here is an invitation to stay. -->
<div in:enjoyMotion class="flex flex-col items-center gap-3 text-center">
	<Icon name={ok ? 'celebrate' : 'refresh'} size={28} class={ok ? 'text-ok' : 'text-danger'} />
	<h1 class="text-base font-semibold text-ink">{$t('ui.callback.enjoyTitle')}</h1>
	{#if ok && account}
		<p class="max-w-[46ch] text-sm leading-relaxed text-muted-foreground">
			{$t('ui.callback.enjoyBody', { values: { account } })}
		</p>
	{:else if !ok}
		<p class="max-w-[46ch] text-sm leading-relaxed text-muted-foreground">
			{$t('ui.callback.enjoyRetry')}
		</p>
	{/if}
</div>
