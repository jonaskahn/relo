<script lang="ts">
	import { t } from 'svelte-i18n';

	import RailLabel from './rail-label.svelte';
	import TunnelMark from './tunnel-mark.svelte';
	import { railShares, type RouteTunnel } from '$lib/route-tunnel';

	// RoutingUniverseList is the routing universe a card too narrow for a map draws:
	// the two rails as lists, each node with the share of the rail it carried.

	interface Props {
		tunnel: RouteTunnel;
	}

	let { tunnel }: Props = $props();

	const clients = $derived(railShares(tunnel.agents));
	const providers = $derived(railShares(tunnel.connections));
</script>

<div class="flex flex-col gap-5">
	<section>
		<h3>
			<RailLabel icon="device-desktop" label={$t('ui.pages.dashboard.universeClients')} />
		</h3>
		<ul class="mt-2 flex flex-col">
			{#each clients as share (share.node.id)}
				<li class="flex items-center gap-3 py-1.5">
					<span class="mark">
						<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
							<TunnelMark
								markId={share.node.markId}
								label={share.node.label}
								x={8}
								y={8}
								size={16}
							/>
						</svg>
					</span>
					<div class="min-w-0 flex-1">
						<div class="flex items-baseline justify-between gap-2">
							<span class="truncate text-sm font-medium text-ink" title={share.node.label}>
								{share.node.label}
							</span>
							<span class="mono-data shrink-0 text-[0.8125rem] text-muted-foreground">
								{share.node.count.toLocaleString()}
							</span>
						</div>
						<div class="track mt-1.5">
							<div class="fill" style="width: {(share.share * 100).toFixed(1)}%"></div>
						</div>
					</div>
				</li>
			{/each}
		</ul>
	</section>

	<section>
		<h3>
			<RailLabel icon="plug-connected" label={$t('ui.pages.dashboard.universeProviders')} />
		</h3>
		<ul class="mt-2 flex flex-col">
			{#each providers as share (share.node.id)}
				<li class="flex items-center gap-3 py-1.5">
					<span class="mark">
						<svg width="16" height="16" viewBox="0 0 16 16" aria-hidden="true">
							<TunnelMark
								markId={share.node.markId}
								label={share.node.label}
								x={8}
								y={8}
								size={16}
							/>
						</svg>
					</span>
					<div class="min-w-0 flex-1">
						<div class="flex items-baseline justify-between gap-2">
							<span class="truncate text-sm font-medium text-ink" title={share.node.label}>
								{share.node.label}
							</span>
							<span class="mono-data shrink-0 text-[0.8125rem] text-muted-foreground">
								{share.node.count.toLocaleString()}
							</span>
						</div>
						<div class="track mt-1.5">
							<div
								class="fill fill-provider"
								style="width: {(share.share * 100).toFixed(1)}%"
							></div>
						</div>
					</div>
				</li>
			{/each}
		</ul>
	</section>
</div>

<style>
	.mark {
		display: flex;
		width: 1.625rem;
		height: 1.625rem;
		flex: none;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--line);
		border-radius: 7px;
		background: var(--surface);
	}

	.track {
		height: 7px;
		border-radius: 999px;
		background: var(--accent-100);
		overflow: hidden;
	}

	.fill {
		height: 100%;
		border-radius: 999px;
		background: var(--accent);
	}

	.fill-provider {
		background: var(--accent-400);
	}
</style>
