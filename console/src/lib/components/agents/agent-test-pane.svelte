<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import { cn } from '$lib/utils';
	import { contextLabelOf, filterModels, providerOf, providersOf } from '$lib/integration-models';
	import type { IntegrationChat, IntegrationModel, IntegrationView } from '$lib/types';

	// The models one agent's own key reaches, read in the shape its protocol
	// answers with, and one turn sent through the same key. Both travel the data
	// plane the agent is pointed at, so what this pane shows is what the client
	// sees.
	interface Props {
		agent: IntegrationView;
		models: IntegrationModel[];
		state: 'idle' | 'loading' | 'ready' | 'failed';
		message: string;
		query: string;
		provider: string;
		selected: string;
		prompt: string;
		testing: boolean;
		answer: IntegrationChat | null;
		verify: { running: boolean; ok: boolean; copy: string } | null;
		onquery: (value: string) => void;
		onprovider: (value: string) => void;
		onselect: (value: string) => void;
		onprompt: (value: string) => void;
		onsend: () => void;
	}

	let {
		agent,
		models,
		state,
		message,
		query,
		provider,
		selected,
		prompt,
		testing,
		answer,
		verify,
		onquery,
		onprovider,
		onselect,
		onprompt,
		onsend
	}: Props = $props();

	// The providers this listing actually names, so the filter offers only what
	// is there.
	const modelProviders = $derived(providersOf(models));
	const visibleModels = $derived(filterModels(models, { query, provider }));
	const selectedModel = $derived(models.find((model) => model.id === selected) ?? null);
	const selectedContext = $derived(contextLabelOf(selectedModel));
	// selectedProvider names the connection the chosen model came from, read from
	// the listing's own metadata rather than from the public name.
	const selectedProvider = $derived(selectedModel ? providerOf(selectedModel) : '');
</script>

<div class="flex min-w-0 flex-col gap-3 rounded-lg border border-border bg-sunken p-4">
	<div class="min-w-0">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.testTitle')}</h3>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.testHint')}</p>
	</div>

	{#if !agent.enabled}
		<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.testOff')}</p>
	{:else if state === 'loading'}
		<!-- Slot loading: the pane keeps its shape and the slot spins. -->
		<p class="flex items-center gap-2 text-xs text-muted-foreground">
			<Icon name="loader" size={14} spin />
			{$t('ui.common.loading')}
		</p>
	{:else if models.length === 0}
		<p class="text-xs text-muted-foreground">
			{message || $t('ui.pages.integrationsPage.testNoModels')}
		</p>
	{:else}
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.integrationsPage.testModels', {
				values: { count: models.length, format: agent.protocol }
			})}
		</p>
		<!-- The filters narrow the picker by text and provider, so a long list
		     stays usable. -->
		<div class="grid gap-2 sm:grid-cols-2">
			<Field id="integration-model-search" label={$t('ui.pages.integrationsPage.testSearch')}>
				<Input
					id="integration-model-search"
					value={query}
					oninput={(event) => onquery(event.currentTarget.value)}
					placeholder={$t('ui.pages.integrationsPage.testSearchPlaceholder')}
				/>
			</Field>
			<Field id="integration-model-provider" label={$t('ui.pages.integrationsPage.testProvider')}>
				<NativeSelect
					id="integration-model-provider"
					value={provider}
					onchange={(event) => onprovider(event.currentTarget.value)}
				>
					<option value="">{$t('ui.pages.integrationsPage.testAll')}</option>
					{#each modelProviders as name (name)}
						<option value={name}>{name}</option>
					{/each}
				</NativeSelect>
			</Field>
		</div>
		{#if visibleModels.length === 0}
			<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.testNoMatch')}</p>
		{/if}
		<Field id="integration-test-model" label={$t('ui.pages.integrationsPage.testModel')}>
			<NativeSelect
				id="integration-test-model"
				value={selected}
				onchange={(event) => onselect(event.currentTarget.value)}
			>
				{#each visibleModels as model (model.id)}
					<option value={model.id}>{model.name || model.id}</option>
				{/each}
			</NativeSelect>
		</Field>
		{#if selectedContext}
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.integrationsPage.testContext', {
					values: { context: selectedContext, provider: selectedProvider }
				})}
			</p>
		{/if}
		<Field id="integration-test-prompt" label={$t('ui.pages.integrationsPage.testPrompt')}>
			<Input
				id="integration-test-prompt"
				value={prompt}
				oninput={(event) => onprompt(event.currentTarget.value)}
				placeholder={$t('ui.pages.integrationsPage.testPlaceholder')}
			/>
		</Field>
		<div class="flex flex-wrap items-center gap-2">
			<Button size="sm" disabled={testing || prompt.trim() === ''} onclick={onsend}>
				<Icon name={testing ? 'loader' : 'message-square'} size={14} spin={testing} />
				{$t('ui.pages.integrationsPage.testSend')}
			</Button>
			{#if answer && answer.status > 0}
				<span class={cn('mono-data text-xs', answer.ok ? 'text-ok' : 'text-danger')}>
					{answer.status} · {answer.duration_ms} ms
				</span>
			{/if}
		</div>
		{#if answer}
			{#if answer.ok}
				<pre
					class="mono-data max-h-56 overflow-auto rounded-lg bg-canvas p-3 text-xs whitespace-pre-wrap">{answer.text ||
						$t('ui.pages.integrationsPage.testEmpty')}</pre>
				{#if (answer.warnings ?? []).length > 0}
					<div class="flex flex-wrap items-center gap-1.5" role="status">
						{#each answer.warnings ?? [] as warning (warning)}
							<StatusBadge kind="warn" label={warning} />
						{/each}
					</div>
				{/if}
			{:else}
				<p class="flex items-start gap-1.5 text-xs text-danger" role="alert">
					<Icon name="circle-x" size={13} class="mt-px shrink-0" />
					{answer.error}
				</p>
			{/if}
		{/if}
	{/if}

	<!-- Verification reports beside the wiring it proves, so the pane's own body
	     stays about the models rather than the last check. The check itself
	     runs from the modal footer, so the pane carries no second Verify. -->
	{#if verify && verify.copy}
		<p
			class={cn('flex items-start gap-1.5 text-xs', verify.ok ? 'text-ok' : 'text-danger')}
			role="status"
		>
			<Icon name={verify.ok ? 'circle-check' : 'circle-x'} size={13} class="mt-px shrink-0" />
			{verify.copy}
		</p>
	{/if}
</div>
