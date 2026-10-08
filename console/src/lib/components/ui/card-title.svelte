<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	import { cn } from '$lib/utils';

	let { class: className, children, ...rest }: HTMLAttributes<HTMLDivElement> = $props();
</script>

<div
	data-slot="card-title"
	class={cn('font-semibold leading-none tracking-tight', className)}
	{...rest}
>
	{@render children?.()}
</div>
