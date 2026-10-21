<script lang="ts">
	import { t } from 'svelte-i18n';

	import TunnelMark from './tunnel-mark.svelte';
	import { tunnelGeometry, type RouteTunnel, type TunnelPlacement } from '$lib/route-tunnel';

	// RouteTunnel is the map of the last routes: the agents on one side, Relo in the
	// middle, the connections on the other. Every route is one line: it runs flat
	// beside the name and then curves into Relo, where the newest request's route
	// carries a line of travelling dots.

	interface Props {
		tunnel: RouteTunnel;
		width: number;
		height: number;
		vertical: boolean;
		live?: boolean;
	}

	let { tunnel, width, height, vertical, live = false }: Props = $props();

	const geometry = $derived(tunnelGeometry(tunnel, { width, height, vertical }));

	// The destination of a new route pulses once, the way a live event lights the line it
	// took. The first route the card draws is not an event, so it does not pulse.
	let pulse = $state('');
	let pulseKey = $state(0);
	let seen = '';
	let played = false;

	$effect(() => {
		const id = tunnel.active?.connection ?? '';
		if (id === '' || id === seen) return;
		seen = id;
		if (!played) {
			played = true;
			return;
		}
		pulse = id;
		pulseKey += 1;
	});
</script>

<svg
	viewBox="0 0 {geometry.width} {geometry.height}"
	width={geometry.width}
	height={geometry.height}
	class="block"
	role="img"
	aria-label={$t('ui.pages.dashboard.universeTitle')}
>
	{#each geometry.legs as leg (leg.id)}
		<path
			class="leg"
			d={leg.path}
			fill="none"
			stroke="var(--line-strong)"
			stroke-width={leg.width}
			stroke-linecap="round"
		/>
	{/each}

	<!-- The newest request's route, drawn over the lines it shares. -->
	{#if geometry.active.path}
		<path
			class="route-flow"
			class:route-moving={live}
			d={geometry.active.path}
			fill="none"
			stroke-width="3"
			stroke-linecap="round"
		/>
	{/if}

	{#each geometry.agents as placement (placement.node.id)}
		{@render node(placement)}
	{/each}

	{#each geometry.connections as placement (placement.node.id)}
		{@render node(placement)}
	{/each}

	<!-- Relo: the hub every route passes through. -->
	<rect
		x={geometry.hub.x}
		y={geometry.hub.y}
		width={geometry.hub.width}
		height={geometry.hub.height}
		rx={geometry.hub.height / 2}
		fill="var(--ink)"
	/>
	<text
		x={geometry.hub.x + geometry.hub.width / 2}
		y={geometry.hub.y + geometry.hub.height / 2 + 1}
		text-anchor="middle"
		dominant-baseline="middle"
		font-size="13"
		font-weight="600"
		fill="var(--canvas)">Relo</text
	>
</svg>

{#snippet node(placement: TunnelPlacement)}
	<g class="node">
		<title>{placement.node.label}</title>
		{#if pulse === placement.node.id}
			{#key pulseKey}
				<rect
					class="node-halo"
					x={placement.tile.x}
					y={placement.tile.y}
					width={placement.tile.size}
					height={placement.tile.size}
					rx={placement.tile.radius}
				/>
			{/key}
		{/if}
		<rect
			x={placement.tile.x}
			y={placement.tile.y}
			width={placement.tile.size}
			height={placement.tile.size}
			rx={placement.tile.radius}
			fill="var(--surface)"
			stroke="var(--line)"
		/>
		<TunnelMark
			markId={placement.node.markId}
			label={placement.node.label}
			x={placement.tile.x + placement.tile.size / 2}
			y={placement.tile.y + placement.tile.size / 2}
			size={placement.tile.mark}
		/>
		<text
			class="tunnel-label"
			x={placement.label.x}
			y={placement.label.y}
			text-anchor={placement.label.anchor}
			font-size="12"
			fill="var(--ink)"
			>{placement.text}<tspan class="mono-data" dx="6" fill="var(--faint)"
				>{placement.node.count}</tspan
			></text
		>
	</g>
{/snippet}

<style>
	/* A name stands over the line that runs to Relo, so it carries the surface behind
	   its letters: the line passes behind the name instead of through it. */
	.tunnel-label {
		paint-order: stroke fill;
		stroke: var(--surface);
		stroke-width: 5px;
		stroke-linejoin: round;
	}

	/* A line that takes more traffic thickens instead of jumping, and a line that joins
	   a rail fades in with the node it belongs to. */
	.leg {
		transition: stroke-width 200ms ease-out;
		animation: tunnel-in 240ms ease-out;
	}

	.node {
		animation: tunnel-in 240ms ease-out;
	}

	@keyframes tunnel-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}

	/* The live route is a line of dots that travels along the path the newest request
	   took. It runs while the tail runs, and stops where the tail stops. */
	.route-flow {
		stroke: var(--accent);
		stroke-dasharray: 0.1 12;
	}

	.route-moving {
		animation: route-flow 1.4s linear infinite;
	}

	@keyframes route-flow {
		to {
			stroke-dashoffset: -12.1;
		}
	}

	/* A live event lights the line it took and leaves a halo that expands off the
	   node. */
	.node-halo {
		fill: none;
		stroke: var(--accent);
		transform-box: fill-box;
		transform-origin: center;
		pointer-events: none;
		animation: node-halo 600ms ease-out forwards;
	}

	@keyframes node-halo {
		from {
			transform: scale(1);
			opacity: 0.5;
			stroke-width: 1.5;
		}
		to {
			transform: scale(1.8);
			opacity: 0;
			stroke-width: 1.5;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.route-moving {
			animation: none;
		}

		.node-halo {
			display: none;
		}
	}
</style>
