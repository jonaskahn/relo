<script lang="ts">
	// The brand mark — one node, two arcs carrying traffic around it. The
	// shapes paint with currentColor, so wherever the mark appears it wears
	// the color the surrounding classes name, accent included.
	import { cn } from '$lib/utils';

	interface Props {
		class?: string;
	}

	let { class: className }: Props = $props();
</script>

<svg
	xmlns="http://www.w3.org/2000/svg"
	viewBox="0 0 512 512"
	fill="none"
	aria-hidden="true"
	class={cn('shrink-0', className)}
>
	<g stroke="currentColor" stroke-width="56" stroke-linecap="round" fill="none">
		<path d="M256 91 A165 165 0 0 1 421 256" />
		<path d="M256 421 A165 165 0 0 1 91 256" />
	</g>
	<circle cx="256" cy="256" r="64" fill="currentColor" />
</svg>
