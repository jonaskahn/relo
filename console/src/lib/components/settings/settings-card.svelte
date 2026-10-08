<script lang="ts">
	import type { Snippet } from 'svelte';

	// SettingsCard is one level-1 card of the settings page: a title, a
	// one-line description, and the rows it holds. Two descriptions reserve the
	// same height so paired cards start their first row on the same line, and
	// the body grows so a paired card's footer — its save row — sits on the
	// shared baseline at the bottom.
	interface Props {
		id: string;
		title: string;
		description?: string;
		children: Snippet;
		footer?: Snippet;
	}

	let { id, title, description = '', children, footer }: Props = $props();
</script>

<section
	class="section-surface flat-surface flex h-full flex-col p-[var(--card-pad)]"
	aria-labelledby={id}
>
	<header class="max-w-[72ch]">
		<h2 {id} class="section-heading">{title}</h2>
		{#if description}
			<p class="mt-1 min-h-[2lh] text-sm leading-relaxed text-muted-foreground">
				{description}
			</p>
		{/if}
	</header>
	<div class="mt-5 flex flex-1 flex-col">
		{@render children()}
		{#if footer}
			<div class="mt-auto">{@render footer()}</div>
		{/if}
	</div>
</section>
