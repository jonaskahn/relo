<script lang="ts">
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';

	// RailLabel names one rail of the routing universe: the clients that sent the
	// requests, or the connections that served them.

	interface Props {
		icon: IconName;
		label: string;
	}

	let { icon, label }: Props = $props();
</script>

<span class="rail-label">
	<Icon name={icon} size={14} />
	{label}
</span>

<style>
	.rail-label {
		display: inline-flex;
		align-items: center;
		gap: 0.375rem;
		font-size: 0.75rem;
		font-weight: 500;
		color: var(--muted);
	}
</style>
