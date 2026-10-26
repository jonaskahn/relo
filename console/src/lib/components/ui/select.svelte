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
			'focus-visible:ring-ring/50 relative flex h-control w-full min-w-0 items-center rounded-control bg-control py-1 pr-7 pl-3 text-left text-sm text-ink shadow-field outline-none transition-[background-color,box-shadow] disabled:cursor-not-allowed focus-visible:ring-[3px] hover:bg-control-hover hover:shadow-field-hover',
			className
		)}
	>
		<span class={cn('block w-full truncate', !selected && 'text-muted-foreground')}>
			{selected?.label ?? placeholder ?? ''}
		</span>
		<Icon
			name="chevron-down"
			size={14}
			class="pointer-events-none absolute right-2 text-muted-foreground"
		/>
	</Select.Trigger>

	<Select.Portal>
		<Select.Content
			class="z-50 w-(--bits-select-anchor-width) min-w-0 rounded-popover bg-raised p-1 shadow-pop outline-none"
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
											'relative flex cursor-default items-center gap-2 rounded-control px-2.5 py-1.5 text-left text-sm outline-none select-none',
											'data-[selected]:bg-accent-100 data-[selected]:text-accent-ink data-[highlighted]:bg-accent-50 data-[highlighted]:text-accent-ink hover:bg-accent-50 hover:text-accent-ink',
											'data-[disabled]:cursor-not-allowed'
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
