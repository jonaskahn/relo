<script lang="ts">
	import { t } from 'svelte-i18n';

	import TunnelMark from './tunnel-mark.svelte';
	import { tunnelGeometry, type RouteTunnel, type TunnelPlacement } from '$lib/route-tunnel';

	// RouteTunnel is the map of the last routes: the agents on one rail, Relo in the
	// middle, the connections on the other. Every route is one line: it runs flat
	// beside the pill that names it and then curves into Relo, where the lines gather
	// into one bundle. The newest request's route is drawn over the lines it shares.

	interface Props {
		tunnel: RouteTunnel;
		width: number;
		height: number;
		live?: boolean;
	}

	let { tunnel, width, height, live = false }: Props = $props();

	const geometry = $derived(tunnelGeometry(tunnel, { width, height }));

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
	<!-- Every line is drawn twice: a quiet base that carries the traffic, and the accent
	     layer that moves while the tail runs. -->
	{#each geometry.legs as leg (leg.id)}
		<path
			class="leg"
			d={leg.path}
			fill="none"
			stroke="var(--accent-200)"
			stroke-width={leg.width}
			stroke-linecap="round"
		/>
		<path
			class="leg-flow"
			class:leg-moving={live}
			d={leg.path}
			fill="none"
			stroke="var(--accent)"
			stroke-width={leg.width}
			stroke-linecap="round"
		/>
	{/each}

	<!-- The newest request's route, drawn over the lines it shares. -->
	{#if geometry.active.path}
		<path
			class="route-line"
			d={geometry.active.path}
			fill="none"
			stroke="var(--accent)"
			stroke-width="3"
			stroke-linecap="round"
		/>
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
	<g>
		<rect
			x={geometry.hub.x}
			y={geometry.hub.y}
			width={geometry.hub.width}
			height={geometry.hub.height}
			rx={geometry.hub.radius}
			fill="var(--ink)"
		/>
		<text
			x={geometry.hub.x + geometry.hub.width / 2}
			y={geometry.hub.y + 34}
			text-anchor="middle"
			font-size="13"
			font-weight="700"
			fill="var(--canvas)">Relo</text
		>
		<line
			x1={geometry.hub.x + geometry.hub.width / 2 - 14}
			y1={geometry.hub.y + 48}
			x2={geometry.hub.x + geometry.hub.width / 2 + 14}
			y2={geometry.hub.y + 48}
			stroke="var(--canvas)"
			stroke-opacity="0.35"
			stroke-width="1"
		/>
		<text
			class="hub-total mono-data"
			x={geometry.hub.x + geometry.hub.width / 2}
			y={geometry.hub.y + 78}
			text-anchor="middle"
			fill="var(--canvas)">{geometry.total.toLocaleString()}</text
		>
		<text
			x={geometry.hub.x + geometry.hub.width / 2}
			y={geometry.hub.y + 96}
			text-anchor="middle"
			font-size="11"
			fill="var(--canvas)"
			fill-opacity="0.7">{$t('ui.pages.dashboard.universeRequests')}</text
		>
	</g>
</svg>

{#snippet node(placement: TunnelPlacement)}
	<g class="node">
		<title>{placement.node.label}</title>
		{#if pulse === placement.node.id}
			{#key pulseKey}
				<rect
					class="node-halo"
					x={placement.pill.tile.x}
					y={placement.pill.tile.y}
					width={placement.pill.tile.size}
					height={placement.pill.tile.size}
					rx={placement.pill.tile.radius}
				/>
			{/key}
		{/if}
		<rect
			class="pill"
			x={placement.pill.x}
			y={placement.pill.y}
			width={placement.pill.width}
			height={placement.pill.height}
			rx={placement.pill.radius}
			fill="var(--surface)"
			stroke="var(--line)"
		/>
		<rect
			x={placement.pill.tile.x}
			y={placement.pill.tile.y}
			width={placement.pill.tile.size}
			height={placement.pill.tile.size}
			rx={placement.pill.tile.radius}
			fill="var(--surface)"
			stroke="var(--line)"
		/>
		<TunnelMark
			markId={placement.node.markId}
			label={placement.node.label}
			x={placement.pill.mark.x}
			y={placement.pill.mark.y}
			size={placement.pill.mark.size}
		/>
		<text class="pill-name" x={placement.pill.textX} y={placement.pill.baseline}
			>{placement.text}</text
		>
		<text class="pill-count mono-data" x={placement.pill.countX} y={placement.pill.baseline}
			>{placement.node.count}</text
		>
	</g>
{/snippet}

<style>
	/* A pill carries the surface behind its letters, so the line that runs past it never
	   reads through the name. */
	.pill-name {
		font-size: 13px;
		font-weight: 500;
		fill: var(--ink);
	}

	.pill-count {
		font-size: 12px;
		fill: var(--muted);
	}

	.hub-total {
		font-size: 14px;
	}

	/* The pill sits on the recessed canvas, so it needs the lift a card has. */
	.pill {
		filter: drop-shadow(0 1px 2px var(--shadow));
	}

	/* A line that takes more traffic thickens instead of jumping, and a line that joins
	   a rail fades in with the node it belongs to. */
	.leg {
		transition: stroke-width 200ms ease-out;
		animation: tunnel-in 240ms ease-out;
	}

	/* The accent layer of a line is a line of dashes that travels along it, so the
	   direction of the traffic reads while the tail runs. */
	.leg-flow {
		stroke-dasharray: 6 18;
	}

	.leg-moving {
		animation: leg-flow 2.4s linear infinite;
	}

	@keyframes leg-flow {
		to {
			stroke-dashoffset: -24;
		}
	}

	/* The live route is a line of dots that travels along the path the newest request
	   took. It runs while the tail runs, and stops where the tail stops. */
	.route-flow {
		stroke: var(--surface);
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
		.leg-moving,
		.route-moving {
			animation: none;
		}

		.node-halo {
			display: none;
		}
	}
</style>
