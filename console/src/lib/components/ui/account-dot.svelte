<script lang="ts">
	import Icon from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';

	// AccountDot is the round control of the console: one numbered account in
	// the line a card scrolls through. The round shape lives here rather than
	// in a page, so every surface picks the same size and the same states.
	interface Props {
		index: number;
		label: string;
		selected?: boolean;
		// warn marks an account that needs a new sign-in. The lock replaces the
		// number, so the cue is not color alone and nothing hangs off the circle.
		warn?: boolean;
		onclick: () => void;
	}

	let { index, label, selected = false, warn = false, onclick }: Props = $props();
</script>

<button
	type="button"
	data-plain
	aria-pressed={selected}
	aria-label={label}
	class={cn(
		'inline-flex size-7 shrink-0 items-center justify-center rounded-control-sm bg-sunken font-mono text-xs tabular-nums text-muted-foreground shadow-float-1 transition-[background-color,box-shadow,color] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring hover:bg-control-hover hover:text-foreground',
		warn
			? selected
				? 'bg-amber-500/15 font-semibold text-amber-800 ring-1 ring-amber-600 dark:text-amber-300 dark:ring-amber-400'
				: 'text-amber-700 hover:text-amber-800 dark:text-amber-400'
			: selected
				? 'bg-accent-soft font-semibold text-accent-ink ring-1 ring-accent-border'
				: ''
	)}
	{onclick}
>
	{#if warn}
		<Icon name="lock" size={14} />
	{:else}
		{index}
	{/if}
</button>
