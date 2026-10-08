<script lang="ts">
	import { t } from 'svelte-i18n';

	import type { Snippet } from 'svelte';

	// PageHeader is the heading band every page opens with: the title and one-line
	// subtitle on the left, and optional status and actions on the right. It sits
	// in the shared page column; only spacing separates it from the content below.
	interface Props {
		title: string;
		description: string;
		meta?: Snippet;
		titleSuffix?: Snippet;
		actions?: Snippet;
	}

	let { title, description, meta, titleSuffix, actions }: Props = $props();
</script>

<header class="page-frame flex flex-wrap items-start justify-between gap-x-6 gap-y-3 pt-8 pb-5">
	<div class="min-w-0 space-y-2">
		<div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
			<h1 class="page-title text-[1.875rem] leading-[1.1] font-semibold">{$t(title)}</h1>
			{#if titleSuffix}
				{@render titleSuffix()}
			{/if}
		</div>
		<p class="max-w-[56ch] text-[0.9375rem] leading-normal text-muted-foreground max-sm:hidden">
			{$t(description)}
		</p>
	</div>
	{#if meta || actions}
		<div class="ms-auto flex min-w-0 flex-wrap items-center gap-x-3.5 gap-y-2">
			{#if meta}
				<div class="text-[0.78125rem] text-muted-foreground">{@render meta()}</div>
			{/if}
			{#if actions}
				<div class="flex min-w-0 items-center gap-2">{@render actions()}</div>
			{/if}
		</div>
	{/if}
</header>
