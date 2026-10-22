<script lang="ts">
	import {
		logoPathTransform,
		providerLogoIsColored,
		providerLogoPath,
		providerLogoSrc,
		providerMonogram
	} from '$lib/provider-logo';

	// TunnelMark is one mark inside a node of the routing tunnel: the provider's own
	// mark, the client's own mark, or the letters of the name when neither exists.
	// A mark with geometry is drawn as a path so it takes the theme's ink; a mark that is
	// only a file is drawn as an image, which cannot.

	interface Props {
		markId: string;
		label: string;
		x: number;
		y: number;
		size: number;
		muted?: boolean;
	}

	let { markId, label, x, y, size, muted = false }: Props = $props();

	const path = $derived(providerLogoPath(markId));
	const src = $derived(path ? null : providerLogoSrc(markId));
	const ink = $derived(muted ? 'var(--muted)' : 'var(--ink)');
</script>

{#if path}
	<!-- The mark is drawn from its own geometry, so it is scaled into the box and
	     centred on the node rather than hung from its origin. -->
	<g transform={logoPathTransform(path, x, y, size)}>
		{#each path.d as segment (segment)}
			<path d={segment} fill={ink} fill-rule="evenodd" clip-rule="evenodd" />
		{/each}
	</g>
{:else if src}
	<!-- A mark that is only a file cannot take the theme's ink, because currentColor
	     resolves inside its own document. Its silhouette is black, so dark mode inverts
	     it to sit on the dark node. A mark that carries its own inks is left alone. -->
	<image
		href={src}
		x={x - size / 2}
		y={y - size / 2}
		width={size}
		height={size}
		preserveAspectRatio="xMidYMid meet"
		class={providerLogoIsColored(markId) ? '' : 'dark:invert'}
	/>
{:else}
	<text
		{x}
		y={y + 1}
		text-anchor="middle"
		dominant-baseline="middle"
		font-size={size * 0.62}
		font-weight="600"
		fill={ink}>{providerMonogram(label)}</text
	>
{/if}
