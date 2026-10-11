<script lang="ts">
	import type { Snippet } from 'svelte';

	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';

	// DetailDisclosure is the one disclosure row a request detail is built
	// from: a chevron that turns with the row, the section name, and the body
	// underneath. The summary keeps its own hit area so the body can hold
	// buttons without opening the row. A badge, when given, sits after the name.

	interface Props {
		open?: boolean;
		title: string;
		badge?: string;
		children: Snippet;
	}

	let { open = false, title, badge, children }: Props = $props();
</script>

<details {open} class="group rounded-lg border border-border">
	<summary
		class="flex cursor-pointer select-none items-center gap-2 px-4 py-3 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
	>
		<Icon
			name="chevron-right"
			size={12}
			class="shrink-0 transition-transform duration-200 group-open:rotate-90"
		/>
		{title}
		{#if badge}
			<StatusBadge kind="muted" label={badge} />
		{/if}
	</summary>
	{@render children()}
</details>
