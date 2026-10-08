<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';

	// FilterToggle is the one button that opens a page's filter fields: the
	// funnel mark, the label, the active count and the chevron. Pages that
	// split their toolbar (tabs centred, actions right) render this directly
	// instead of the whole FilterPanel toolbar.
	interface Props {
		open: boolean;
		activeCount?: number;
		controls: string;
		onclick: () => void;
	}

	let { open, activeCount = 0, controls, onclick }: Props = $props();
</script>

<Button variant="outline" size="sm" aria-expanded={open} aria-controls={controls} {onclick}>
	<Icon name="filter" size={14} />
	{$t('ui.common.filters')}
	{#if activeCount > 0}
		<span class="rounded-sm bg-accent-soft px-1.5 text-[0.65rem] font-semibold text-accent-ink">
			{activeCount}
		</span>
	{/if}
	<Icon
		name="chevron-down"
		size={14}
		class="text-muted-foreground transition-transform duration-150 {open ? 'rotate-180' : ''}"
	/>
</Button>
