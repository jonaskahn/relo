<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	import { cn } from '$lib/utils';

	let { class: className, children, ...rest }: HTMLAttributes<HTMLDivElement> = $props();
</script>

<div
	data-slot="card"
	class={cn(
		'bg-surface text-card-foreground flex flex-col gap-6 rounded-card border border-line py-6 shadow-1',
		className
	)}
	{...rest}
>
	{@render children?.()}
</div>
