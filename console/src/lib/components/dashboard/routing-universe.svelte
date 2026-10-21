<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import RouteTunnel from './route-tunnel.svelte';
	import { PHONE_RAIL_LIMIT, RAIL_LIMIT, routeTunnel, type TunnelTraffic } from '$lib/route-tunnel';
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
	// beside the metrics.
	let height = $state(0);

	// A card too narrow for two rails draws the tunnel upright instead: the agents
	// above Relo and the connections below it.
	const wide = $derived(width >= 620);

	const tunnel = $derived(
		routeTunnel(traffic, {
			keys,
			providers,
			console: $t('ui.pages.dashboard.universeConsole'),
			limit: wide ? RAIL_LIMIT : PHONE_RAIL_LIMIT
		})
	);
</script>

<div class={cn('section-surface flex flex-col p-4 sm:p-5', className)}>
	<div class="mb-4 flex items-center justify-between gap-3">
		<div>
			<h2 class="section-heading">{$t('ui.pages.dashboard.universeTitle')}</h2>
			<p class="text-xs text-muted-foreground">{$t('ui.pages.dashboard.universeDesc')}</p>
		</div>
		{#if !loading && providers.length === 0}
			<Button size="sm" onclick={onadd}>
				<Icon name="plus" size={14} />
				{$t('ui.pages.providersPage.header.addProvider')}
			</Button>
		{/if}
	</div>
	{#if loading}
		{@render pending?.()}
	{:else if providers.length === 0}
		<div
			class="flex flex-col items-center justify-center gap-3 rounded-panel border-[1.5px] border-dashed border-accent-border bg-accent-soft px-4 py-10 text-center"
		>
			<p class="max-w-xs text-sm text-muted-foreground">
				{$t('ui.pages.dashboard.universeEmpty')}
			</p>
		</div>
	{:else if tunnel.agents.length === 0 && tunnel.connections.length === 0}
		<p class="py-8 text-center text-sm text-muted-foreground">
			{$t('ui.pages.dashboard.universeNoTraffic')}
		</p>
	{:else}
		<div bind:clientWidth={width} bind:clientHeight={height} class="min-w-0 flex-1 overflow-hidden">
			{#if width > 0}
				<RouteTunnel {tunnel} {width} {height} vertical={!wide} {live} />
			{/if}
		</div>
	{/if}
</div>
