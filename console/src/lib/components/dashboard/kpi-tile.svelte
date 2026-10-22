<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';

	// KpiTile is the shell every figure at the top of the dashboard wears: the
	// icon and the label, the change the window brought, then what the read
	// answered. The body is the card's own, so a tile states its figure and its
	// note in place.
	interface Props {
		icon: IconName;
		labelKey: string;
		loading: boolean;
		pending: Snippet;
		children: Snippet;
		// delta is the change against the window before this one, as a signed
		// percentage. A tile without one shows no change at all.
		delta?: number | null;
	}

	let { icon, labelKey, loading, pending, children, delta = null }: Props = $props();
</script>

<div class="section-surface flex min-w-0 flex-col gap-3 p-4">
	<div class="flex items-start justify-between gap-2">
		<p class="flex min-w-0 items-center gap-1.5 text-[0.8125rem] font-medium text-muted-foreground">
			<Icon name={icon} size={14} class="shrink-0 text-faint" />
			<span class="truncate">{$t(labelKey)}</span>
		</p>
		{#if delta !== null && !loading}
			<span
				class="inline-flex shrink-0 items-center gap-1 rounded-full bg-sunken px-2 py-0.5 text-[0.7rem] font-medium text-muted-foreground"
			>
				<Icon name={delta >= 0 ? 'arrow-up' : 'arrow-down'} size={11} />
				{(delta >= 0 ? '+' : '') + delta}%
			</span>
		{/if}
	</div>
	{#if loading}
		{@render pending()}
	{:else}
		{@render children()}
	{/if}
</div>
