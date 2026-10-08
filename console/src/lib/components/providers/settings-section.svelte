<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		title: string;
		description?: string;
		section?: string;
		children: Snippet;
	}

	let { title, description = '', section = '', children }: Props = $props();
</script>

<section class="settings-row" data-section={section}>
	<div>
		<h2 class="section-heading">{title}</h2>
		{#if description}
			<p class="mt-1 text-sm leading-relaxed text-muted-foreground">{description}</p>
		{/if}
	</div>
	<div class="min-w-0">
		{@render children()}
	</div>
</section>
