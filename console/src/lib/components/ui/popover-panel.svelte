<script lang="ts">
	import type { Snippet } from 'svelte';
	import { Popover, type WithoutChildrenOrChild } from 'bits-ui';

	import { popoverMotion } from '$lib/popover-motion';

	// The one popover panel: portaled and pinned by Bits UI as before, but it
	// opens and closes with the shared popover motion instead of appearing.
	// `data-reduced-motion-fade` keeps a 140ms fade where the global reduce
	// rule would otherwise cut the transition to nothing.
	interface Props extends WithoutChildrenOrChild<Popover.ContentProps> {
		children?: Snippet;
	}

	let { class: className, children, ...rest }: Props = $props();
</script>

<Popover.Content forceMount {...rest} class={className}>
	{#snippet child({ props, wrapperProps, open })}
		{#if open}
			<div {...wrapperProps}>
				<div {...props} data-reduced-motion-fade transition:popoverMotion>
					{@render children?.()}
				</div>
			</div>
		{/if}
	{/snippet}
</Popover.Content>
