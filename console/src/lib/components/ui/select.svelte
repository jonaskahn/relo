<script lang="ts">
	import { Select } from 'bits-ui';

	import Icon from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';
	import { popoverMotion } from '$lib/popover-motion';

	export type SelectOption = {
		value: string;
		label: string;
		disabled?: boolean;
		hint?: string;
	};

	interface Props {
		options: SelectOption[];
		value?: string;
		class?: string;
		placeholder?: string;
		disabled?: boolean;
		id?: string;
		ariaLabel?: string;
		onValueChange?: (value: string) => void;
	}

	let {
		options,
		value = $bindable(''),
		class: className,
		placeholder,
		disabled = false,
		id,
		ariaLabel,
		onValueChange
	}: Props = $props();

	const selected = $derived(options.find((o) => o.value === value));
</script>

<Select.Root
	type="single"
	bind:value
	{disabled}
	onValueChange={(next) => onValueChange?.(next ?? '')}
>
	<Select.Trigger
		{id}
		aria-label={ariaLabel}
		class={cn(
			'border-line focus-visible:border-ring focus-visible:ring-ring/50 flex h-control w-full min-w-0 items-center justify-between gap-2 rounded-control border bg-surface py-1 pr-8 pl-3 text-left text-sm shadow-1 outline-none transition-[color,box-shadow] disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50 focus-visible:ring-[3px]',
			className
		)}
	>
		<span class={cn('truncate', !selected && 'text-muted-foreground')}>
			{selected?.label ?? placeholder ?? ''}
		</span>
		<Icon name="chevron-down" size={14} class="shrink-0 text-muted-foreground" />
	</Select.Trigger>

	<Select.Portal>
		<Select.Content
			class="z-50 min-w-[var(--bits-select-trigger-width)] rounded-popover border border-line bg-raised p-1 shadow-3 outline-none"
			sideOffset={4}
		>
			{#snippet child({ props, wrapperProps, open })}
				{#if open}
					<div {...wrapperProps}>
						<div {...props} data-reduced-motion-fade transition:popoverMotion>
							<Select.Viewport class="max-h-64 overflow-y-auto p-0.5">
								{#each options as option (option.value)}
									<Select.Item
										value={option.value}
										disabled={option.disabled}
										class={cn(
											'relative flex cursor-default items-center gap-2 rounded-control px-2.5 py-1.5 text-sm outline-none select-none',
											'data-[highlighted]:bg-accent-50 data-[highlighted]:text-accent-ink',
											'data-[disabled]:pointer-events-none data-[disabled]:opacity-50'
										)}
									>
										{#snippet child({ props: itemProps, selected: isSelected })}
											<div {...itemProps}>
												<span class="flex-1 truncate">{option.label}</span>
												{#if option.hint}
													<span class="text-xs text-muted-foreground">{option.hint}</span>
												{/if}
												{#if isSelected}
													<Icon name="check" size={14} class="text-accent" />
												{/if}
											</div>
										{/snippet}
									</Select.Item>
								{/each}
							</Select.Viewport>
						</div>
					</div>
				{/if}
			{/snippet}
		</Select.Content>
	</Select.Portal>
</Select.Root>
