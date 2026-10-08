<script lang="ts">
	import { onMount } from 'svelte';
	import { Tooltip } from 'bits-ui';
	import { buttonVariants } from '$lib/components/ui/button.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';

	// IconAction is the one icon button for row actions: always labelled for
	// assistive tech, with a portal tooltip that never opens on a touch
	// surface, where hover does not exist. The tooltip trigger itself carries
	// the button styling, so there is no nested control.
	interface Props {
		icon: IconName;
		label: string;
		onclick?: () => void;
		disabled?: boolean;
		variant?: 'ghost' | 'outline';
		pressed?: boolean;
		tone?: 'default' | 'accent' | 'destructive';
		spin?: boolean;
		iconSize?: number;
		class?: string;
	}

	let {
		icon,
		label,
		onclick,
		disabled = false,
		variant = 'ghost',
		pressed,
		tone = 'default',
		spin = false,
		iconSize = 15,
		class: className = ''
	}: Props = $props();

	let coarse = $state(false);
	onMount(() => {
		const media = window.matchMedia('(pointer: coarse)');
		const update = () => (coarse = media.matches);
		update();
		media.addEventListener('change', update);
		return () => media.removeEventListener('change', update);
	});

	const classes = $derived(
		cn(
			buttonVariants({ variant, size: 'icon' }),
			tone === 'accent'
				? 'text-accent-ink hover:text-accent-strong'
				: tone === 'destructive'
					? 'text-destructive hover:text-destructive'
					: 'text-muted-foreground hover:text-foreground',
			className
		)
	);
</script>

<Tooltip.Provider delayDuration={250} disableHoverableContent>
	<Tooltip.Root>
		{#if disabled}
			<Tooltip.Trigger
				class={cn(classes, 'cursor-not-allowed opacity-60')}
				data-slot="button"
				aria-label={label}
				aria-disabled="true"
				tabindex={-1}
			>
				<Icon name={icon} size={iconSize} {spin} />
			</Tooltip.Trigger>
		{:else}
			<Tooltip.Trigger
				class={classes}
				data-slot="button"
				aria-label={label}
				aria-pressed={pressed}
				{onclick}
			>
				<Icon name={icon} size={iconSize} {spin} />
			</Tooltip.Trigger>
		{/if}
		{#if !coarse}
			<Tooltip.Portal>
				<Tooltip.Content class="z-50 surface-pop px-2.5 py-1.5 text-xs">
					{label}
				</Tooltip.Content>
			</Tooltip.Portal>
		{/if}
	</Tooltip.Root>
</Tooltip.Provider>
