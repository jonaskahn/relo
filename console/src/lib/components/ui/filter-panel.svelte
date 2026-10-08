<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { slide } from 'svelte/transition';

	import { browserStorage, filterPanelId, readPanelOpen } from '$lib/filter-panel';

	// FilterPanel is the fields grid behind one toggle: a page's filter row
	// keeps its own toolbar (the toggle, its clear action, the centered tab
	// strip, the live or range controls) and drops this grid under it. Which
	// way the toggle stood is remembered in this browser, so a page opens the
	// way it was left; the page owns the toggle and binds open.
	interface Props {
		storageKey: string;
		// columns is how many fields share a row on a wide screen, chosen per
		// page so its fields fill their rows without a lone one.
		columns?: 3 | 4;
		children: Snippet;
		open?: boolean;
	}

	let { storageKey, columns = 4, children, open = $bindable(false) }: Props = $props();

	const panelId = $derived(filterPanelId(storageKey));
	let reducedMotion = $state(false);

	// A page's toggle writes this itself when it flips open, so the panel only
	// has to read the settled state once.
	onMount(() => {
		open = readPanelOpen(browserStorage(), storageKey);
		const media = window.matchMedia('(prefers-reduced-motion: reduce)');
		const update = () => (reducedMotion = media.matches);
		update();
		media.addEventListener('change', update);
		return () => media.removeEventListener('change', update);
	});
</script>

{#if open}
	<div
		id={panelId}
		class="filter-grid filter-grid-{columns}"
		transition:slide={{ duration: reducedMotion ? 0 : 180, easing: cubicOut }}
	>
		{@render children()}
	</div>
{/if}
