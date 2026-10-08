<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';

	// KpiTile is the shell every figure at the top of the dashboard wears: the
	// icon and the label, then what the read answered. The body is the card's
	// own, so a tile states its figure, its delta and its note in place.
	interface Props {
		icon: IconName;
		labelKey: string;
		loading: boolean;
		pending: Snippet;
		children: Snippet;
	}

	let { icon, labelKey, loading, pending, children }: Props = $props();
</script>

<div class="section-surface flex min-w-0 flex-col gap-3 p-4">
	<p class="flex items-center gap-1.5 text-[0.8125rem] font-medium text-muted-foreground">
		<Icon name={icon} size={14} class="text-faint" />{$t(labelKey)}
	</p>
	{#if loading}
		{@render pending()}
	{:else}
		{@render children()}
	{/if}
</div>
