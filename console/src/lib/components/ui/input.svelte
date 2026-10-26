<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';

	import { cn } from '$lib/utils';

	interface Props extends HTMLInputAttributes {
		class?: string;
	}

	let { class: className, value = $bindable(), disabled, ...rest }: Props = $props();
</script>

<input
	bind:value
	{disabled}
	class={cn(
		'placeholder:text-muted-foreground focus-visible:ring-ring/50 aria-invalid:ring-danger/20 flex h-control w-full min-w-0 rounded-control px-3 py-1 text-base text-ink outline-none file:inline-flex file:h-7 file:border-0 file:bg-transparent file:text-sm file:font-medium disabled:cursor-not-allowed md:text-sm focus-visible:ring-[3px]',
		className
	)}
	{...rest}
/>
