<script lang="ts">
	import type { HTMLSelectAttributes } from 'svelte/elements';

	import Icon from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';

	interface Props extends HTMLSelectAttributes {
		class?: string;
		value?: unknown;
	}

	// NativeSelect is the platform select styled like Input, so a row of an
	// input, a select and a button shares one height and one corner.
	let { class: className, id, value = $bindable(), children, ...rest }: Props = $props();
</script>

<div class={cn('relative grid items-center', className)}>
	<select
		{id}
		bind:value
		class="border-line focus-visible:border-ring focus-visible:ring-ring/50 h-control w-full min-w-0 appearance-none rounded-control border bg-surface py-1 pr-8 pl-3 text-sm shadow-1 outline-none transition-[color,box-shadow] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 focus-visible:ring-[3px]"
		{...rest}
	>
		{@render children?.()}
	</select>
	<Icon
		name="chevron-down"
		size={14}
		class="pointer-events-none absolute right-2.5 text-muted-foreground"
	/>
</div>
