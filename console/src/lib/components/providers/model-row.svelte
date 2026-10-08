<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Model } from '$lib/types';
	import { DropdownMenu } from 'bits-ui';
	import MenuPanel from '$lib/components/ui/menu-panel.svelte';
	import { isCardOpenClick } from '$lib/card-target';
	import { Switch } from 'bits-ui';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import { isPriced } from '$lib/model-filters';
	import CapabilityPicker from './capability-picker.svelte';
	import ContextPicker from './context-picker.svelte';

	// One model is one row-card: the switch decides a request, the name opens
	// the model, the middle reads its limits and capabilities, and the menu
	// holds the rest.
	interface Props {
		model: Model;
		providerLabel?: string;
		showProvider?: boolean;
		ontoggle: (model: Model, enabled: boolean) => void;
		onopen: (model: Model) => void;
		onedit: (model: Model) => void;
		onclone?: (model: Model) => void;
		onsavecontext: (model: Model, value: number | null) => Promise<void>;
		onsavemaxoutput: (model: Model, value: number | null) => Promise<void>;
		onsavecapability: (
			model: Model,
			field: 'tools' | 'reasoning' | 'vision',
			value: boolean | null
		) => Promise<void>;
	}

	let {
		model,
		providerLabel = '',
		showProvider = false,
		ontoggle,
		onopen,
		onedit,
		onclone,
		onsavecontext,
		onsavemaxoutput,
		onsavecapability
	}: Props = $props();

	const priced = $derived(isPriced(model.prices));
	const upstreamDiffers = $derived(
		model.upstream_model_id !== '' && model.upstream_model_id !== model.model_id
	);
	const activeAccounts = $derived(model.active_accounts ?? 0);
	const servingAccounts = $derived(model.serving_accounts ?? 0);
	// A model some accounts cannot reach is worth naming: the request goes to
	// the accounts that hold it.
	const partialCoverage = $derived(
		model.routable && activeAccounts > 0 && servingAccounts < activeAccounts
	);
</script>

<!-- The row-card is a pointer shortcut: the title button is the keyboard
     target that opens the same model, so the shell itself is not focusable. -->
<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<article
	data-model-card={model.provider_id + '/' + model.model_id}
	class="section-surface flex flex-wrap items-center gap-x-4 gap-y-3 px-4 py-3 transition-colors card-press {model.enabled
		? ''
		: '[&>*:not([data-row-menu])]:opacity-55'}"
	onclick={(event) => {
		if (isCardOpenClick(event)) onopen(model);
	}}
>
	<Switch.Root
		checked={model.enabled}
		onCheckedChange={(value: boolean) => ontoggle(model, value)}
		aria-label={$t(
			model.enabled
				? 'ui.pages.providersPage.modelsTable.turnOff'
				: 'ui.pages.providersPage.modelsTable.turnOn',
			{ values: { id: model.model_id } }
		)}
		title={$t(
			model.enabled
				? 'ui.pages.providersPage.modelsTable.turnOff'
				: 'ui.pages.providersPage.modelsTable.turnOn',
			{
				values: { id: model.model_id }
			}
		)}
	/>

	<div class="min-w-40 flex-1">
		<button
			type="button"
			data-plain
			class="block min-w-0 max-w-full text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
			aria-label={$t('ui.common.openCard', { values: { name: model.name || model.model_id } })}
			onclick={() => onopen(model)}
		>
			<span class="block max-w-[30ch] truncate text-sm" title={model.name || model.model_id}>
				{model.name || model.model_id}
			</span>
			<span class="block truncate font-mono text-[0.7rem] text-muted-foreground">
				{showProvider && providerLabel ? providerLabel + ' / ' : ''}{model.model_id}
			</span>
		</button>
		<div class="mt-1 flex flex-wrap items-center gap-1.5">
			{#if showProvider}
				<ProviderLogo id={model.provider_id} label={providerLabel || model.provider_id} size="sm" />
			{/if}
			{#if model.cloned_from}
				<span class="badge border border-border text-muted-foreground">
					<Icon name="copy-plus" size={11} />
					{$t('ui.pages.providersPage.modelsTable.tagCloneOf', {
						values: { id: model.cloned_from }
					})}
				</span>
			{/if}
			{#if upstreamDiffers}
				<span class="badge border border-border font-mono text-muted-foreground">
					{$t('ui.pages.providersPage.modelsTable.tagUpstream', {
						values: { id: model.upstream_model_id }
					})}
				</span>
			{/if}
			{#if model.status === 'deprecated'}
				<span class="badge border border-warn/40 text-warn">
					{$t('ui.pages.providersPage.models.tagDeprecated')}
				</span>
			{/if}
			{#if model.available === false}
				<span class="badge border border-border text-muted-foreground">
					{$t('ui.pages.providersPage.models.tagUnavailable')}
				</span>
			{/if}
			{#if partialCoverage}
				<span class="badge border border-border text-muted-foreground">
					{$t('ui.pages.providersPage.models.coverage', {
						values: {
							serving: servingAccounts,
							active: activeAccounts
						}
					})}
				</span>
			{/if}
			{#if !priced}
				<span class="badge border border-warn/40 text-warn">
					{$t('ui.pages.providersPage.models.tagUnpriced')}
				</span>
			{/if}
		</div>
	</div>

	<div class="flex shrink-0 flex-col gap-0.5">
		<span class="text-[0.65rem] text-faint">{$t('ui.pages.providersPage.models.context')}</span>
		<ContextPicker {model} onsave={(value) => onsavecontext(model, value)} />
	</div>
	<div class="flex shrink-0 flex-col gap-0.5">
		<span class="text-[0.65rem] text-faint"
			>{$t('ui.pages.providersPage.modelsTable.columnMaxOutput')}</span
		>
		<ContextPicker {model} field="output" onsave={(value) => onsavemaxoutput(model, value)} />
	</div>

	<CapabilityPicker
		{model}
		field="tools"
		labelKey="ui.pages.providersPage.modelsTable.columnTools"
		onsave={(value) => onsavecapability(model, 'tools', value)}
	/>
	<CapabilityPicker
		{model}
		field="reasoning"
		labelKey="ui.pages.providersPage.modelsTable.columnReasoning"
		onsave={(value) => onsavecapability(model, 'reasoning', value)}
	/>
	<CapabilityPicker
		{model}
		field="vision"
		labelKey="ui.pages.providersPage.modelsTable.columnVision"
		onsave={(value) => onsavecapability(model, 'vision', value)}
	/>

	<div class="flex shrink-0 items-center gap-0.5" data-row-menu>
		<DropdownMenu.Root>
			<DropdownMenu.Trigger
				class="inline-flex size-control shrink-0 items-center justify-center rounded-control border border-line text-muted-foreground hover:border-line-strong hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:border-line-strong data-[state=open]:text-ink"
				aria-label={$t('ui.pages.providersPage.models.actions')}
				title={$t('ui.pages.providersPage.models.actions')}
			>
				<Icon name="dots" size={16} />
			</DropdownMenu.Trigger>
			<MenuPanel class="z-50 min-w-52 surface-pop p-1" align="end">
				<DropdownMenu.Item
					class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none data-highlighted:bg-hover"
					onSelect={() => onopen(model)}
				>
					<Icon name="eye" size={14} />
					{$t('ui.pages.providersPage.modelsTable.view')}
				</DropdownMenu.Item>
				<DropdownMenu.Item
					class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none data-highlighted:bg-hover"
					onSelect={() => onedit(model)}
				>
					<Icon name="pencil" size={14} />
					{$t('ui.pages.providersPage.models.edit')}
				</DropdownMenu.Item>
				{#if onclone}
					<DropdownMenu.Item
						class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none data-highlighted:bg-hover"
						onSelect={() => onclone(model)}
					>
						<Icon name="copy-plus" size={14} />
						{$t('ui.pages.providersPage.modelsTable.clone')}
					</DropdownMenu.Item>
				{/if}
			</MenuPanel>
		</DropdownMenu.Root>
	</div>
</article>
