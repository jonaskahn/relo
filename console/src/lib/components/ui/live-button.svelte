<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';

	// LiveButton is the one live/pause tail control every log list shares: the
	// request log, the daemon log and the dashboard recent list. The equalizer
	// reads as a now-playing wave next to the paused bars, so all three tails
	// show one state.
	interface Props {
		live: boolean;
		ontoggle: () => void;
	}

	let { live, ontoggle }: Props = $props();
</script>

<Button variant="outline" size="sm" class="min-w-24" aria-pressed={live} onclick={ontoggle}>
	{#if live}
		<svg class="live-eq" width="16" height="14" viewBox="0 0 24 14" fill="none" aria-hidden="true">
			<rect
				class="live-bar live-bar-1"
				x="2"
				y="1"
				width="3"
				height="12"
				rx="1.5"
				fill="currentColor"
			/>
			<rect
				class="live-bar live-bar-2"
				x="7"
				y="1"
				width="3"
				height="12"
				rx="1.5"
				fill="currentColor"
			/>
			<rect
				class="live-bar live-bar-3"
				x="12"
				y="1"
				width="3"
				height="12"
				rx="1.5"
				fill="currentColor"
			/>
			<rect
				class="live-bar live-bar-4"
				x="17"
				y="1"
				width="3"
				height="12"
				rx="1.5"
				fill="currentColor"
			/>
		</svg>
	{:else}
		<Icon name="player-pause" size={14} />
	{/if}
	{live ? $t('ui.pages.logsPage.live') : $t('ui.pages.logsPage.paused')}
</Button>

<style>
	/* The equalizer pumps while the tail listens: four rounded bars scale from
	   their centres on a staggered loop, which reads as a music wave rather
	   than a beat. The loop starts and ends on the same pose, so it restarts
	   with no visible seam. Transform only, never layout; the global
	   reduced-motion rule stills it, with the local guard below as the
	   backstop. */
	.live-eq {
		display: inline-flex;
		flex-shrink: 0;
	}
	.live-bar {
		transform-box: fill-box;
		transform-origin: center;
		animation-name: live-pump;
		animation-duration: 1.1s;
		animation-timing-function: ease-in-out;
		animation-iteration-count: infinite;
	}
	.live-bar-1 {
		animation-delay: 0s;
	}
	.live-bar-2 {
		animation-delay: 0.12s;
	}
	.live-bar-3 {
		animation-delay: 0.24s;
	}
	.live-bar-4 {
		animation-delay: 0.12s;
	}
	@keyframes live-pump {
		0%,
		100% {
			transform: scaleY(0.45);
		}
		50% {
			transform: scaleY(1);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.live-bar {
			animation: none;
		}
	}
</style>
