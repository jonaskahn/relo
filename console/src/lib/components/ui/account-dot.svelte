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
		'inline-flex size-7 shrink-0 items-center justify-center rounded-full border font-mono text-xs tabular-nums transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
		warn
			? selected
				? 'border-amber-600 bg-amber-500/15 font-semibold text-amber-800 dark:border-amber-400 dark:text-amber-300'
				: 'border-amber-600 text-amber-700 hover:border-amber-700 hover:text-amber-800 dark:border-amber-400 dark:text-amber-400'
			: selected
				? 'border-accent-line bg-accent-soft font-semibold text-accent-ink'
				: 'border-border text-muted-foreground hover:border-border-strong hover:text-foreground'
	)}
	{onclick}
>
	{#if warn}
		<Icon name="lock" size={14} />
	{:else}
		{index}
	{/if}
</button>
