<script lang="ts">
	import { fade } from 'svelte/transition';
	import { t } from 'svelte-i18n';

	import { reducedMotion } from '$lib/card-layout-motion';
	import { modeDiagram, type DiagramLabels, type DiagramMode } from '$lib/mode-diagram';

	// How a request travels in the mode the client is wired for. The environment mode
	// sends everything to Relo; the proxy mode puts Relo's intercept in the path and
	// forks it. The lines stand still unless the wiring is applied: dots travel them
	// only while it is, so the picture never claims traffic that is not flowing.
	interface Props {
		mode: DiagramMode;
		/** Whether the wiring is applied, which is what lets the dots run. */
		live: boolean;
		/** A mode the operator is looking at before it is written. */
		preview?: boolean;
	}

	let { mode, live, preview = false }: Props = $props();

	let width = $state(0);

	// A product name is not translated; the words around it are.
	const labels = $derived<DiagramLabels>({
		client: { name: 'Claude Code', sub: $t('ui.pages.integrationsPage.modeDiagram.subClient') },
		intercept: {
			name: $t('ui.pages.integrationsPage.modeDiagram.intercept'),
			sub: '127.0.0.1'
		},
		relo: { name: 'Relo', sub: $t('ui.pages.integrationsPage.modeDiagram.subRelo') },
		providers: {
			name: $t('ui.pages.integrationsPage.modeDiagram.connections'),
			sub: $t('ui.pages.integrationsPage.modeDiagram.subConnections')
		},
		anthropic: {
			name: 'Anthropic',
			sub: $t(
				mode === 'env'
					? 'ui.pages.integrationsPage.modeDiagram.subNotUsed'
					: 'ui.pages.integrationsPage.modeDiagram.subAnthropic'
			)
		}
	});

	const diagram = $derived(modeDiagram({ mode, width, labels }));
	const caption = $derived(
		$t(
			mode === 'env'
				? 'ui.pages.integrationsPage.modeDiagram.captionEnv'
				: 'ui.pages.integrationsPage.modeDiagram.captionProxy'
		)
	);

	// A line that joins fades in and one that leaves fades out, the way the dashboard's map
	// takes new traffic; the lines two modes share morph in place.
	const JOIN_MS = 240;
	const LEAVE_MS = 160;

	function chipText(kind: string): string {
		return $t(
			kind === 'bound'
				? 'ui.pages.integrationsPage.modeDiagram.bound'
				: 'ui.pages.integrationsPage.modeDiagram.native'
		);
	}

	// The chip is as wide as its word, so a longer language never overflows it.
	const chipWidth = (text: string) => Math.max(40, text.length * 6.4 + 14);
</script>

<section class="flex min-w-0 flex-col gap-3 rounded-lg bg-sunken p-4">
	<div class="min-w-0">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.modeDiagram.title')}</h3>
		{#if preview}
			<p class="text-xs text-warn">{$t('ui.pages.integrationsPage.modeDiagram.preview')}</p>
		{:else if !live}
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.integrationsPage.modeDiagram.idle')}
			</p>
		{/if}
	</div>

	<div class="min-w-0" bind:clientWidth={width}>
		{#if width > 0}
			<svg
				viewBox="0 0 {diagram.width} {diagram.height}"
				width={diagram.width}
				height={diagram.height}
				class="block max-w-full"
				role="img"
				aria-label={caption}
			>
				{#each diagram.edges as edge (edge.id)}
					<g
						in:fade={{ duration: reducedMotion() ? 0 : JOIN_MS }}
						out:fade={{ duration: reducedMotion() ? 0 : LEAVE_MS }}
					>
						<path
							class="line line-{edge.kind}"
							d={edge.path}
							style="d: path('{edge.path}')"
							fill="none"
							stroke-linecap="round"
						/>
						{#if live && !preview && edge.flows}
							<path
								class="flow flow-{edge.kind}"
								d={edge.path}
								style="d: path('{edge.path}')"
								fill="none"
								stroke-linecap="round"
							/>
						{/if}
					</g>
				{/each}

				{#each diagram.nodes as node (node.id)}
					<g
						class="node"
						class:dim={node.dim}
						style="transform: translate({node.x}px, {node.y}px)"
						in:fade={{ duration: reducedMotion() ? 0 : JOIN_MS }}
						out:fade={{ duration: reducedMotion() ? 0 : LEAVE_MS }}
					>
						<rect
							class="box box-{node.id}"
							width={node.width}
							height={node.height}
							rx="12"
							stroke-dasharray={node.dim ? '4 3' : undefined}
						/>
						<text class="name name-{node.id}" x={node.width / 2} y="19" text-anchor="middle"
							>{node.name}</text
						>
						<text class="sub mono-data sub-{node.id}" x={node.width / 2} y="34" text-anchor="middle"
							>{node.sub}</text
						>
					</g>
				{/each}

				{#each diagram.edges as edge (edge.id)}
					{#if edge.chip && (edge.kind === 'bound' || edge.kind === 'native')}
						{@const text = chipText(edge.kind)}
						{@const size = chipWidth(text)}
						<g
							class="chip chip-{edge.kind}"
							style="transform: translate({edge.chip.x - size / 2}px, {edge.chip.y - 9}px)"
						>
							<rect width={size} height="18" rx="9" />
							<text x={size / 2} y="12.5" text-anchor="middle">{text}</text>
						</g>
					{/if}
				{/each}
			</svg>
		{/if}
	</div>

	<p class="text-xs text-muted-foreground">{caption}</p>
</section>

<style>
	/* A line stands still: it is the path. Its weight says which side carries the model Relo
	   serves. The shared lines redraw into the next mode's path. */
	.line {
		transition: d 400ms cubic-bezier(0.2, 0.8, 0.2, 1);
		stroke-width: 3;
	}

	.line-plain,
	.line-bound {
		stroke: var(--accent);
	}

	.line-native {
		stroke: var(--line-strong);
	}

	.line-off {
		stroke: var(--line-strong);
		stroke-width: 2;
		stroke-dasharray: 4 5;
		opacity: 0.7;
	}

	/* Dots travel the line while the wiring is applied, the same flow the dashboard draws on
	   its newest route. Reduced motion draws the line alone. */
	.flow {
		stroke: var(--surface);
		stroke-width: 3;
		stroke-dasharray: 0.1 12;
		transition: d 400ms cubic-bezier(0.2, 0.8, 0.2, 1);
		animation: mode-flow 1.4s linear infinite;
	}

	.flow-native {
		animation-delay: -0.7s;
	}

	@keyframes mode-flow {
		to {
			stroke-dashoffset: -12.1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.flow {
			display: none;
		}
	}

	.node {
		transition: transform 400ms cubic-bezier(0.2, 0.8, 0.2, 1);
	}

	.node.dim {
		opacity: 0.5;
	}

	.box {
		fill: var(--surface);
		stroke: var(--line-strong);
		stroke-width: 1;
		filter: drop-shadow(0 1px 2px var(--shadow));
	}

	/* Relo is the one node drawn solid, as it is the hub on the dashboard's map. */
	.box-relo {
		fill: var(--ink);
		stroke: var(--ink);
	}

	.box-intercept {
		stroke: var(--accent);
	}

	.name {
		font-size: 12px;
		font-weight: 600;
		fill: var(--ink);
	}

	.name-relo {
		fill: var(--canvas);
	}

	.sub {
		font-size: 10px;
		fill: var(--muted);
	}

	.sub-relo {
		fill: var(--canvas);
		fill-opacity: 0.7;
	}

	.chip {
		pointer-events: none;
		transition: transform 400ms cubic-bezier(0.2, 0.8, 0.2, 1);
	}

	.chip rect {
		fill: var(--surface);
		stroke: var(--line-strong);
	}

	.chip-bound rect {
		stroke: var(--accent);
	}

	.chip text {
		font-size: 10px;
		font-weight: 600;
		fill: var(--ink);
	}
</style>
