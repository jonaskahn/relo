<script lang="ts">
	import { t } from 'svelte-i18n';
	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import { Popover } from 'bits-ui';
	import PopoverPanel from '$lib/components/ui/popover-panel.svelte';
	import {
		CONTEXT_PRESETS,
		contextLabel,
		parseContextInput,
		roundTokenCount
	} from '$lib/context-window';

	interface Props {
		id: string;
		label: string;
		value: string;
		error?: string;
		notice?: string;
		editLabel: string;
		presets?: boolean;
	}

	let {
		id,
		label,
		value = $bindable(),
		error = '',
		notice = '',
		editLabel,
		presets = false
	}: Props = $props();

	let open = $state(false);

	function normalize() {
		const parsed = parseContextInput(value);
		if (!parsed.ok) return;
		value = contextLabel(roundTokenCount(parsed.value));
	}

	function choose(preset: number) {
		value = contextLabel(preset);
		open = false;
	}
</script>

<div class="flex flex-col gap-1.5">
	<Field {id} {label} {error}>
		<div class="flex items-center gap-1.5">
			<Input {id} class="font-mono" inputmode="decimal" bind:value onblur={normalize} />
			{#if presets}
				<Popover.Root bind:open>
					<Popover.Trigger
						type="button"
						class="inline-flex size-control shrink-0 items-center justify-center rounded-md border border-border text-muted-foreground hover:border-border-strong hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
						aria-label={editLabel}
					>
						<Icon name="pencil" size={14} />
					</Popover.Trigger>
					<Popover.Portal>
						<PopoverPanel class="z-50 w-72 surface-pop p-3" align="end" sideOffset={6}>
							<p
								class="mb-2 flex items-center justify-between gap-2 rounded-md bg-muted/60 px-2 py-1.5 text-xs"
								role="status"
							>
								<span>{$t('ui.pages.providersPage.context.currentValue')}</span>
								<span class="font-mono"
									>{value.trim() || $t('ui.pages.providersPage.context.unset')}</span
								>
							</p>
							<p class="mb-1.5 text-xs font-semibold">
								{$t('ui.pages.providersPage.context.presets')}
							</p>
							<div class="flex flex-wrap gap-1.5">
								{#each CONTEXT_PRESETS as preset (preset)}
									<button
										type="button"
										data-plain
										class="badge border border-border font-mono text-muted-foreground hover:border-border-strong hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:scale-[0.98] motion-reduce:transform-none {value.trim() ===
										contextLabel(preset)
											? 'border-accent bg-accent-soft text-foreground'
											: ''}"
										aria-label={$t('ui.pages.providersPage.context.quickSet', {
											values: { value: contextLabel(preset) }
										})}
										onclick={() => choose(preset)}
									>
										{contextLabel(preset)}
									</button>
								{/each}
							</div>
						</PopoverPanel>
					</Popover.Portal>
				</Popover.Root>
			{/if}
		</div>
	</Field>
	{#if notice}
		<p class="flex items-start gap-1.5 text-xs text-muted-foreground">
			<Icon name="info-circle" size={13} class="mt-0.5 shrink-0" />
			<span>{notice}</span>
		</p>
	{/if}
</div>
