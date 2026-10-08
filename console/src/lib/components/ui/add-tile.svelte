<script lang="ts">
	import Icon from './icon.svelte';
	import { cn } from '$lib/utils';

	// AddTile is the last cell of a card grid: the empty panel's dashed,
	// accent-tinted outline reduced to one card, carrying the page's creation
	// action next to the cards an operator already has. The whole tile is one
	// button, so the target is as large as the cell it fills.
	interface Props {
		label: string;
		hint: string;
		onclick: () => void;
		class?: string;
	}

	let { label, hint, onclick, class: className }: Props = $props();
</script>

<button
	type="button"
	data-plain
	{onclick}
	class={cn(
		'flex min-h-56 w-full flex-col items-center justify-center gap-3 rounded-card border-[1.5px] border-dashed border-accent-border p-5 text-center transition-colors hover:bg-accent-soft focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
		className
	)}
>
	<span class="flex size-11 items-center justify-center rounded-full bg-accent-100 text-accent-ink">
		<Icon name="plus" size={20} />
	</span>
	<span class="text-sm font-medium text-accent-deep">{label}</span>
	<span class="text-xs text-muted-foreground">{hint}</span>
</button>
