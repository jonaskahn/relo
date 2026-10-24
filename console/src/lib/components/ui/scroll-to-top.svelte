<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { cubicOut } from 'svelte/easing';
	import type { TransitionConfig } from 'svelte/transition';

	import { buttonVariants } from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';

	// ScrollToTop is a floating control that appears once the page scrolls into
	// a long list and returns to the top on a click. It finds the scroll
	// container from its own position in the tree, so it works on every page
	// that rides the shell's scrolling main.

	// A list is "long" once the operator is a screen and a half in: earlier
	// than that, returning is a flick of the wheel rather than a button.
	const THRESHOLD_PX = 480;

	let root = $state<HTMLElement | null>(null);
	let scroller = $state<HTMLElement | null>(null);
	let visible = $state(false);
	let reduced = $state(false);

	// The nearest ancestor that scrolls, falling back to the document when the
	// page fills the window itself.
	function scrollParent(node: HTMLElement): HTMLElement {
		let current: HTMLElement | null = node.parentElement;
		while (current) {
			const { overflowY } = getComputedStyle(current);
			if (overflowY === 'auto' || overflowY === 'scroll' || overflowY === 'overlay') {
				return current;
			}
			current = current.parentElement;
		}
		return (document.scrollingElement as HTMLElement | null) ?? document.documentElement;
	}

	onMount(() => {
		reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		if (!root) return;
		const container = scrollParent(root);
		scroller = container;
		const update = () => (visible = container.scrollTop > THRESHOLD_PX);
		container.addEventListener('scroll', update, { passive: true });
		update();
		return () => container.removeEventListener('scroll', update);
	});

	function toTop() {
		scroller?.scrollTo({ top: 0, behavior: reduced ? 'auto' : 'smooth' });
	}

	// A rise in with a fade; reduced motion keeps the fade and drops the rise.
	function enter(_node: HTMLElement): TransitionConfig {
		if (reduced) return { duration: 120, css: (t) => `opacity: ${t}` };
		return {
			duration: 200,
			easing: cubicOut,
			css: (t) => `opacity: ${t}; transform: translateY(${(1 - t) * 8}px)`
		};
	}

	function leave(_node: HTMLElement): TransitionConfig {
		return { duration: 120, css: (t) => `opacity: ${t}` };
	}
</script>

<div bind:this={root} class="pointer-events-none fixed bottom-6 right-6 z-40">
	{#if visible}
		<button
			class={cn(
				buttonVariants({ variant: 'outline', size: 'icon' }),
				// A control and a half, so the corner control reads at a glance
				// without crowding the page.
				'scroll-top-button pointer-events-auto h-[calc(var(--control-h)*1.5)] w-[calc(var(--control-h)*1.5)] shadow-[var(--shadow-pop)]'
			)}
			aria-label={$t('ui.common.scrollToTop')}
			title={$t('ui.common.scrollToTop')}
			in:enter
			out:leave
			onclick={toTop}
		>
			<Icon name="arrow-up" size={20} class="scroll-top-arrow" />
		</button>
	{/if}
</div>

<style>
	/* The arrow idles with a slow bob so the corner control reads as live, and
	   lifts harder on hover so it invites the click that returns to the top. */
	.scroll-top-arrow {
		animation: scroll-top-bob 2.4s ease-in-out infinite;
	}
	.scroll-top-button:hover :global(.scroll-top-arrow) {
		animation: scroll-top-lift 0.4s ease-out forwards;
	}

	@keyframes scroll-top-bob {
		0%,
		100% {
			transform: translateY(1.5px);
		}
		50% {
			transform: translateY(-1.5px);
		}
	}

	@keyframes scroll-top-lift {
		to {
			transform: translateY(-3px);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.scroll-top-arrow,
		.scroll-top-button:hover :global(.scroll-top-arrow) {
			animation: none;
		}
	}
</style>
