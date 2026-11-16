<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import {
		bindingBody,
		newBinding,
		targetFromValue,
		targetValue,
		unboundOfficial
	} from '$lib/claude-bindings';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Select from '$lib/components/ui/select.svelte';
	import { integrationPath } from '$lib/integration-actions';
	import type {
		ClaudeBinding,
		ClaudeBindingEntry,
		ClaudeBindings,
		ClaudeBindingsUpdate,
		IntegrationView
	} from '$lib/types';

	// Which Relo model or route answers a Claude model while Claude Code runs in proxy
	// mode. A model with no binding still reaches Anthropic with the caller's own login.
	interface Props {
		agent: IntegrationView;
	}

	let { agent }: Props = $props();

	let loaded = $state<ClaudeBindings | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let failure = $state('');
	let pickModel = $state('');
	let pickTarget = $state('');

	const entries = $derived(loaded?.bindings ?? []);
	const targetOptions = $derived(
		(loaded?.targets ?? []).map((option) => ({ value: targetValue(option), label: option.label }))
	);
	const modelOptions = $derived(
		unboundOfficial(loaded?.official ?? [], entries).map((model) => ({
			value: model.id,
			label: model.id
		}))
	);
	const directionOptions = $derived([
		{ value: 'replace', label: $t('ui.pages.integrationsPage.claudeBindingsReplace') },
		{ value: 'fallback', label: $t('ui.pages.integrationsPage.claudeBindingsFallback') }
	]);
	const failureOptions = $derived([
		{ value: 'none', label: $t('ui.pages.integrationsPage.claudeBindingsOnFailureNone') },
		{ value: 'native', label: $t('ui.pages.integrationsPage.claudeBindingsOnFailureNative') }
	]);

	onMount(() => {
		void load();
	});

	async function load() {
		loading = true;
		failure = '';
		try {
			loaded = await api<ClaudeBindings>(integrationPath(agent.id, 'bindings'));
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
		}
	}

	async function write(update: ClaudeBindingsUpdate) {
		saving = true;
		failure = '';
		try {
			loaded = await api<ClaudeBindings>(integrationPath(agent.id, 'bindings'), {
				method: 'POST',
				body: JSON.stringify(update)
			});
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			saving = false;
		}
	}

	function change(entry: ClaudeBindingEntry, patch: Partial<ClaudeBinding>) {
		void write({ set: [bindingBody({ ...entry, ...patch })] });
	}

	function add() {
		const target = targetFromValue(pickTarget);
		if (!pickModel || !target) return;
		void write({ set: [newBinding(pickModel, target)] }).then(() => {
			if (!failure) {
				pickModel = '';
				pickTarget = '';
			}
		});
	}

	function stateKey(state: ClaudeBindingEntry['state']): string {
		switch (state) {
			case 'dangling':
				return 'ui.pages.integrationsPage.claudeBindingsStateDangling';
			case 'disabled':
				return 'ui.pages.integrationsPage.claudeBindingsStateDisabled';
			default:
				return 'ui.pages.integrationsPage.claudeBindingsStateBound';
		}
	}
</script>

<section class="space-y-3 border-t border-border pt-4">
	<div class="min-w-0">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.claudeBindingsTitle')}</h3>
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.integrationsPage.claudeBindingsHint')}
		</p>
	</div>

	{#if loading}
		<p class="flex items-center gap-2 text-xs text-muted-foreground">
			<Icon name="loader" size={14} spin />
			{$t('ui.common.loading')}
		</p>
	{:else if loaded}
		{#if entries.length === 0}
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.integrationsPage.claudeBindingsEmpty')}
			</p>
		{/if}

		{#each entries as entry (entry.native_id)}
			<div class="space-y-2 rounded-lg border border-border p-3">
				<div class="flex flex-wrap items-center justify-between gap-2">
					<code class="min-w-0 break-all text-xs font-medium">{entry.native_id}</code>
					<span
						class="text-xs {entry.state === 'bound' ? 'text-muted-foreground' : 'text-warning'}"
					>
						{$t(stateKey(entry.state))}
					</span>
				</div>
				<div class="grid gap-2 sm:grid-cols-2">
					<Select
						options={targetOptions}
						value={targetValue(entry.target)}
						disabled={saving}
						ariaLabel={$t('ui.pages.integrationsPage.claudeBindingsTarget')}
						onValueChange={(next) => {
							const target = targetFromValue(next);
							if (target) change(entry, { target });
						}}
					/>
					<Select
						options={directionOptions}
						value={entry.direction}
						disabled={saving}
						ariaLabel={$t('ui.pages.integrationsPage.claudeBindingsDirection')}
						onValueChange={(next) =>
							change(entry, { direction: next === 'fallback' ? 'fallback' : 'replace' })}
					/>
					{#if entry.direction === 'replace'}
						<Select
							options={failureOptions}
							value={entry.on_failure}
							disabled={saving}
							ariaLabel={$t('ui.pages.integrationsPage.claudeBindingsOnFailure')}
							onValueChange={(next) =>
								change(entry, { on_failure: next === 'native' ? 'native' : 'none' })}
						/>
					{/if}
				</div>
				<Button
					size="sm"
					variant="ghost"
					disabled={saving}
					onclick={() => void write({ remove: [entry.native_id] })}
				>
					{$t('ui.common.remove')}
				</Button>
			</div>
		{/each}

		<div class="grid gap-2 sm:grid-cols-2">
			<Select
				options={modelOptions}
				bind:value={pickModel}
				disabled={saving || modelOptions.length === 0}
				placeholder={$t('ui.pages.integrationsPage.claudeBindingsChooseModel')}
				ariaLabel={$t('ui.pages.integrationsPage.claudeBindingsModel')}
			/>
			<Select
				options={targetOptions}
				bind:value={pickTarget}
				disabled={saving || targetOptions.length === 0}
				placeholder={$t('ui.pages.integrationsPage.claudeBindingsChooseTarget')}
				ariaLabel={$t('ui.pages.integrationsPage.claudeBindingsTarget')}
			/>
		</div>
		<Button size="sm" disabled={saving || !pickModel || !pickTarget} onclick={add}>
			{$t('ui.pages.integrationsPage.claudeBindingsAdd')}
		</Button>

		<p class="flex items-start gap-2 text-xs text-muted-foreground">
			<Icon name="info-circle" size={14} />
			<span>{$t('ui.pages.integrationsPage.claudeBindingsCaveat')}</span>
		</p>
	{/if}

	{#if saving}
		<p class="flex items-center gap-2 text-xs text-muted-foreground">
			<Icon name="loader" size={14} spin />
			{$t('ui.common.saving')}
		</p>
	{:else if failure}
		<p class="text-sm text-danger" role="alert">{failure}</p>
	{/if}
</section>
