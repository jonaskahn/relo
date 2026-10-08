<script lang="ts">
	import { providerLogoIsColored, providerLogoSrc } from '$lib/provider-logo';

	interface Props {
		id: string;
		label: string;
		size?: 'xs' | 'sm' | 'md';
		round?: boolean;
	}

	let { id, label, size = 'md', round = false }: Props = $props();

	const box: Record<string, string> = { xs: 'size-6', sm: 'size-9', md: 'size-11' };
	const glyph: Record<string, string> = { xs: 'size-3.5', sm: 'size-6', md: 'size-6' };

	const logo = $derived(providerLogoSrc(id));
	// The small mark keeps both radii it shipped with. A round mark is a circle
	// and does not also carry the square radii.
	const radius = $derived(
		round ? 'rounded-full' : size === 'xs' ? 'rounded-xl rounded-lg' : 'rounded-xl'
	);
	// A mark drawn with an <img> cannot inherit the page colour, because
	// currentColor inside a separate document resolves against that document's
	// own colour and not the console's. Masking the file with background-color
	// is what lets one asset serve both themes: the silhouette takes the ink
	// the tile already uses. A mark whose own inks are part of the drawing is
	// the exception and stays an image.
	const mask = $derived(logo && !providerLogoIsColored(id) ? `url(${logo})` : '');
</script>

<span
	class="flex shrink-0 items-center justify-center border border-border bg-card text-sm font-semibold text-muted-foreground {box[
		size
	]} {radius}"
	aria-hidden="true"
>
	{#if mask}
		<span
			class="{glyph[size]} bg-foreground"
			style="mask-image:{mask};mask-size:contain;mask-repeat:no-repeat;mask-position:center;-webkit-mask-image:{mask};-webkit-mask-size:contain;-webkit-mask-repeat:no-repeat"
		></span>
	{:else if logo}
		<img src={logo} alt="" class="{glyph[size]} object-contain" loading="lazy" />
	{:else}
		{label.slice(0, 1).toUpperCase()}
	{/if}
</span>
