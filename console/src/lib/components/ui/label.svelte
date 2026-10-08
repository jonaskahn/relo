<script lang="ts">
	import type { HTMLLabelAttributes } from 'svelte/elements';

	import { cn } from '$lib/utils';

	interface Props extends HTMLLabelAttributes {
		class?: string;
	}

	let { class: className, children, ...rest }: Props = $props();
</script>

<label
	class={cn('flex items-center gap-2 text-sm font-medium leading-none select-none', className)}
	{...rest}
>
	{@render children?.()}
</label>
