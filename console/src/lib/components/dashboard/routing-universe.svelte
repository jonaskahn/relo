<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import RailLabel from './rail-label.svelte';
	import RouteTunnel from './route-tunnel.svelte';
	import RoutingUniverseList from './routing-universe-list.svelte';
	import {
		MIN_TUNNEL_WIDTH,
		RAIL_LIMIT,
		routeTunnel,
		tunnelHeight,
		type TunnelTraffic
	} from '$lib/route-tunnel';
	import type { AccessKey, Provider } from '$lib/types';
	import { cn } from '$lib/utils';

	// RoutingUniverse is the map of where the window's traffic went: the agents that sent
	// the requests on one rail, Relo in the middle, and the connections that served them
	// on the other. The newest request's route is the one the tunnel follows.

	interface Props {
		traffic: TunnelTraffic;
		keys: AccessKey[];
		providers: Provider[];
		live?: boolean;
		loading?: boolean;
		pending?: Snippet;
		onadd: () => void;
		class?: string;
	}

	let {
		traffic,
		keys,
		providers,
		live = false,
		loading = false,
		pending,
		onadd,
		class: className
	}: Props = $props();

	// The tunnel is drawn at the width it is given, so the names on it keep the size
	// the card's type is. The card's own width is read once the browser has laid it out.
	let width = $state(0);
	// The card gives the tunnel the height it has left, so the map fills the space
	// beside the figures.
	let height = $state(0);

	const tunnel = $derived(
		routeTunnel(traffic, {
			keys,
			providers,
			console: $t('ui.pages.dashboard.universeConsole'),
			limit: RAIL_LIMIT
		})
	);

	// A card too narrow for two rails draws the rails as lists instead of a map.
	const narrow = $derived(width > 0 && width < MIN_TUNNEL_WIDTH);
</script>

<div class={cn('section-surface flex flex-col p-4 sm:p-5', className)}>
	<div class="mb-4 flex items-start justify-between gap-3">
		<div class="min-w-0">
			<h2 class="section-heading flex items-center gap-2">
				<Icon name="route" size={15} class="shrink-0 text-faint" />
				{$t('ui.pages.dashboard.universeTitle')}
			</h2>
			<p class="text-xs text-muted-foreground">{$t('ui.pages.dashboard.universeDesc')}</p>
		</div>
		{#if !loading && tunnel.total > 0}
			<span class="mono-data shrink-0 text-[0.8125rem] text-muted-foreground">
				{$t('ui.pages.dashboard.universeTotal', { values: { count: tunnel.total } })}
			</span>
		{/if}
	</div>
	<div bind:clientWidth={width} class="flex min-w-0 flex-1 flex-col">
		{#if loading}
			{@render pending?.()}
		{:else if providers.length === 0}
			<div
				class="flex w-full flex-1 flex-col items-center justify-center gap-3 rounded-panel border-[1.5px] border-dashed border-accent-border bg-accent-soft px-4 py-10 text-center"
			>
				<p class="max-w-xs text-sm text-muted-foreground">
					{$t('ui.pages.dashboard.universeEmpty')}
				</p>
				<Button size="sm" onclick={onadd}>
					<Icon name="plus" size={14} />
					{$t('ui.pages.providersPage.header.addProvider')}
				</Button>
			</div>
		{:else if tunnel.agents.length === 0 && tunnel.connections.length === 0}
			<p
				class="flex flex-1 items-center justify-center py-8 text-center text-sm text-muted-foreground"
			>
				{$t('ui.pages.dashboard.universeNoTraffic')}
			</p>
		{:else if narrow}
			<RoutingUniverseList {tunnel} />
		{:else}
			<div class="mb-2 flex items-center justify-between gap-3">
				<RailLabel icon="device-desktop" label={$t('ui.pages.dashboard.universeClients')} />
				<RailLabel icon="plug-connected" label={$t('ui.pages.dashboard.universeProviders')} />
			</div>
			<div
				bind:clientHeight={height}
				class="canvas min-w-0 flex-1"
				style="min-height: {tunnelHeight(tunnel)}px"
			>
				{#if width > 0}
					<RouteTunnel {tunnel} {width} {height} {live} />
				{/if}
			</div>
			<p class="mt-2.5 text-xs text-muted-foreground">
				{$t('ui.pages.dashboard.universeLegend')}
			</p>
		{/if}
	</div>
</div>

<style>
	/* The map stands on a recessed panel, so the dots of its grid read as depth
	   rather than as a texture laid over the card. */
	.canvas {
		background-color: var(--sunken);
		background-image: radial-gradient(var(--line) 1px, transparent 1px);
		background-size: 20px 20px;
		border-radius: var(--radius-panel);
		overflow: hidden;
	}
</style>
