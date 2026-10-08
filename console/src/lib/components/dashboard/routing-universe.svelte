<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import {
		providerLogoIsColored,
		providerLogoPath,
		providerLogoSrc,
		providerMonogram
	} from '$lib/provider-logo';
	import { providersByTraffic } from '$lib/dashboard';
	import type { Provider, UsageRollupRow } from '$lib/types';

	// RoutingUniverse is the map of where traffic can go: Relo at the centre,
	// up to six configured connections around it, and a hairline per route
	// whose weight is what the last day actually carried. The accent is spent
	// on the traffic pulse alone.

	// The map keeps an orbit of six; anything past that is named as an
	// ellipsis rather than a seventh node.
	const MAX_NODES = 6;
	const CX = 120,
		CY = 120,
		R_ORBIT = 80,
		R_CENTER = 22,
		R_NODE = 23,
		R_LOGO = 13;

	interface Props {
		providers: Provider[];
		traffic: UsageRollupRow[];
		pulsing?: ReadonlySet<string>;
		loading?: boolean;
		pending?: Snippet;
		onadd: () => void;
	}

	let {
		providers,
		traffic,
		pulsing = new Set<string>(),
		loading = false,
		pending,
		onadd
	}: Props = $props();

	const nodes = $derived(providersByTraffic(providers, traffic).slice(0, MAX_NODES));
	const otherCount = $derived(Math.max(0, providers.length - nodes.length));

	function nodePos(i: number, total: number): [number, number] {
		const angle = (2 * Math.PI * i) / total - Math.PI / 2;
		return [CX + R_ORBIT * Math.cos(angle), CY + R_ORBIT * Math.sin(angle)];
	}
</script>

<div class="section-surface p-4 sm:p-5">
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
	{:else}
		<div class="flex flex-col items-center gap-5 sm:flex-row sm:items-center">
			<svg
				viewBox="0 0 240 240"
				class="h-[240px] w-[240px] shrink-0"
				role="img"
				aria-label={$t('ui.pages.dashboard.universeTitle')}
			>
				<defs>
					{#each nodes as node, i (node.id)}
						{@const [nx, ny] = nodePos(i, nodes.length)}
						<clipPath id={'universe-node-' + node.id}>
							<circle cx={nx} cy={ny} r={R_NODE - 1.5} />
						</clipPath>
					{/each}
				</defs>
				<circle
					cx={CX}
					cy={CY}
					r={R_ORBIT}
					fill="none"
					stroke="var(--line)"
					stroke-width="1"
					stroke-dasharray="3 5"
				/>
				{#each nodes as node, i (node.id)}
					{@const [nx, ny] = nodePos(i, nodes.length)}
					{@const day = traffic.find((row) => row.Key === node.id)}
					{@const active = Number(day?.Requests ?? 0) > 0}
					{@const isPulsing = pulsing.has(node.id)}
					<line
						class="route-link"
						class:route-pulse={isPulsing}
						x1={CX}
						y1={CY}
						x2={nx}
						y2={ny}
						stroke="var(--line-strong)"
						stroke-width={active ? 2 : 1}
						stroke-dasharray={active ? '' : '4 4'}
						opacity={active ? 0.85 : 0.45}
						vector-effect="non-scaling-stroke"
					/>
				{/each}
				{#each nodes as node, i (node.id)}
					{@const [nx, ny] = nodePos(i, nodes.length)}
					{@const connected = node.counts.accounts > 0}
					{@const day = traffic.find((row) => row.Key === node.id)}
					{@const hasError = Number(day?.Errors ?? 0) > 0}
					{@const isPulsing = pulsing.has(node.id)}
					{@const markId = node.template_id || node.id}
					{@const path = providerLogoPath(markId)}
					{@const src = providerLogoSrc(markId)}
					{#if isPulsing}
						<circle class="node-halo" cx={nx} cy={ny} r={R_NODE} />
					{/if}
					<circle
						cx={nx}
						cy={ny}
						r={R_NODE}
						fill="var(--surface)"
						stroke={hasError ? 'var(--danger)' : connected ? 'var(--line-strong)' : 'var(--line)'}
						stroke-width={hasError ? 1.5 : connected ? 1.25 : 1}
					/>
					{#if path}
						<!-- A mark with geometry is a path rather than an <image>:
						     currentColor does not resolve inside an SVG document and a
						     CSS mask cannot reach an SVG child, so only an inline path
						     can take the theme's ink. -->
						<g
							transform="translate({nx} {ny}) scale({R_LOGO /
								path.width}) translate({-path.viewBox.split(' ')[0]} {-path.viewBox.split(' ')[1]})"
						>
							<!-- One element per path, as the mark ships them: merged
							     into a single path, even-odd filling would let an
							     overlapping shape punch a hole in the one below. -->
							{#each path.d as segment (segment)}
								<path d={segment} fill="var(--ink)" fill-rule="evenodd" clip-rule="evenodd" />
							{/each}
						</g>
					{:else if src}
						<!-- A mark that is only a file cannot take the theme's ink,
						     because currentColor resolves inside its own document and a
						     mask cannot reach an SVG child. Its silhouette is black, so
						     dark mode inverts it to sit on the dark node. A mark that
						     carries its own inks is left alone: inverting those would
						     invent colours the vendor never shipped. -->
						<image
							href={src}
							x={nx - R_LOGO}
							y={ny - R_LOGO}
							width={R_LOGO * 2}
							height={R_LOGO * 2}
							preserveAspectRatio="xMidYMid meet"
							clip-path={'url(#universe-node-' + node.id + ')'}
							class={providerLogoIsColored(markId) ? '' : 'dark:invert'}
						/>
					{:else}
						<text
							x={nx}
							y={ny + 1}
							text-anchor="middle"
							dominant-baseline="middle"
							font-size="8"
							font-weight="600"
							fill={connected ? 'var(--ink)' : 'var(--muted)'}>{providerMonogram(node.label)}</text
						>
					{/if}
				{/each}
				<circle
					cx={CX}
					cy={CY}
					r={R_CENTER + 6}
					fill="none"
					stroke="var(--accent-border)"
					stroke-width="1.5"
				/>
				<circle cx={CX} cy={CY} r={R_CENTER} fill="var(--ink)" />
				<text
					x={CX}
					y={CY + 1}
					text-anchor="middle"
					dominant-baseline="middle"
					font-size="10"
					font-weight="700"
					fill="var(--canvas)">R</text
				>
			</svg>
			<div class="w-full min-w-0 space-y-0.5">
				{#each nodes as node (node.id)}
					{@const day = traffic.find((row) => row.Key === node.id)}
					{@const reqs = Number(day?.Requests ?? 0)}
					<div
						class="flex items-center gap-2 rounded-panel px-2 py-1.5 text-xs transition-colors hover:bg-hover"
					>
						<span
							class="size-1.5 shrink-0 rounded-full {node.counts.accounts > 0
								? 'bg-ok'
								: 'bg-faint/40'}"
						></span>
						<span class="min-w-0 flex-1 truncate text-ink">{node.label}</span>
						<span class="mono-data shrink-0 text-muted-foreground"
							>{reqs > 0 ? reqs.toLocaleString() : '—'}</span
						>
					</div>
				{/each}
				{#if otherCount > 0}
					<p
						class="px-2 pt-1 text-xs text-muted-foreground"
						title={$t('ui.pages.dashboard.universeMore', { values: { count: otherCount } })}
					>
						…
					</p>
				{/if}
			</div>
		</div>
	{/if}
</div>

<style>
	/* A live event lights the traffic line for 600ms and leaves a halo that
	   expands off the node. */
	.route-link {
		transition:
			stroke 150ms ease,
			stroke-width 150ms ease,
			opacity 150ms ease;
	}

	.route-pulse {
		stroke: var(--accent);
		animation: route-pulse 600ms ease-out;
	}

	@keyframes route-pulse {
		from {
			stroke-width: 4;
			opacity: 1;
		}
		to {
			stroke-width: 2;
			opacity: 1;
		}
	}

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
		.node-halo {
			display: none;
		}
	}
</style>
