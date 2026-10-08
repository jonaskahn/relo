<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import { Popover } from 'bits-ui';
	import PopoverPanel from '$lib/components/ui/popover-panel.svelte';
	import { cn } from '$lib/utils';
	import type { Model } from '$lib/types';

	// CapabilityPicker is one capability cell of a model row: the icon is the
	// value the agent will see, and the popover pins default, on, or off
	// without touching the rest of the model's override.
	interface Props {
		model: Model;
		field: 'tools' | 'reasoning' | 'vision';
		labelKey: string;
		onsave: (value: boolean | null) => Promise<void>;
		disabled?: boolean;
	}

	let { model, field, labelKey, onsave, disabled = false }: Props = $props();

	let open = $state(false);
	let busy = $state(false);
	let error = $state('');

	const value = $derived(model.capabilities?.[field] ?? null);
	const forced = $derived((model.capability_override?.[field] ?? null) !== null);
	const icon = $derived(
		value === true ? 'circle-check' : value === false ? 'circle-x' : 'circle-off'
	);
	const label = $derived($t(labelKey));
	const stateLabel = $derived(
		value === true
			? $t('ui.pages.providersPage.modelsTable.capabilityOn')
			: value === false
				? $t('ui.pages.providersPage.modelsTable.capabilityOff')
				: $t('ui.pages.providersPage.modelsTable.capabilityDefault')
	);

	async function save(next: boolean | null) {
		busy = true;
		error = '';
		try {
			await onsave(next);
			open = false;
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}
</script>

<div class="flex shrink-0 flex-col items-start gap-0.5">
	<span class="text-[0.65rem] text-faint">{label}</span>
	<span
		class="inline-flex shrink-0 items-center gap-0.5 {disabled ? 'opacity-60' : ''}"
		role="img"
		aria-label={label + ': ' + stateLabel}
	>
		<span
			class={cn(
				'flex size-6 items-center justify-center',
				value === true ? 'text-foreground' : 'text-muted-foreground',
				value === null && 'opacity-40'
			)}
		>
			<Icon name={icon} size={15} />
		</span>
		<span class="w-[1ch] text-center text-xs font-semibold text-accent-strong" aria-hidden="true">
			{forced ? '*' : ''}
		</span>
	</span>
	<Popover.Root bind:open>
		<Popover.Trigger
			{disabled}
			class={cn(
				'inline-flex size-6 shrink-0 items-center justify-center rounded-md text-faint transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
				disabled ? 'cursor-not-allowed opacity-60' : 'hover:bg-muted hover:text-ink'
			)}
			aria-label={$t('ui.pages.providersPage.modelsTable.capabilityEdit', {
				values: { name: label }
			})}
			title={$t('ui.pages.providersPage.modelsTable.capabilityEdit', { values: { name: label } })}
		>
			<Icon name="dots" size={15} />
		</Popover.Trigger>
		<!-- Portaled: the model list scrolls on its own, so a popover opened
		     from a row near its end would be cut off inside it. -->
		<Popover.Portal>
			<PopoverPanel class="z-50 w-44 surface-pop p-2" align="start" sideOffset={6}>
				<p class="mb-1 px-2 py-1 text-xs font-semibold">{label}</p>
				{#each [{ id: 'default', value: null, key: 'capabilityDefault' }, { id: 'on', value: true, key: 'capabilityOn' }, { id: 'off', value: false, key: 'capabilityOff' }] as choice (choice.id)}
					{@const selected = choice.value === null ? !forced : forced && value === choice.value}
					<button
						type="button"
						data-plain
						disabled={busy}
						class={cn(
							'flex w-full items-center rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
							selected && 'bg-accent-soft text-foreground'
						)}
						onclick={() => void save(choice.value)}
					>
						{$t('ui.pages.providersPage.modelsTable.' + choice.key)}
					</button>
				{/each}
				{#if error}
					<p class="mt-1 px-2 text-xs text-destructive" role="alert">{error}</p>
				{/if}
			</PopoverPanel>
		</Popover.Portal>
	</Popover.Root>
</div>
