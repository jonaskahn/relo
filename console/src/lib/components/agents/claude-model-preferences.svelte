<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import {
		claudeEffortChoices,
		claudePreferenceModels,
		claudePreferencePayload,
		type ClaudePreferenceDraft
	} from '$lib/claude-preferences';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Select from '$lib/components/ui/select.svelte';
	import { integrationPath } from '$lib/integration-actions';
	import type { ClaudeModelConfig, IntegrationModel, IntegrationView } from '$lib/types';

	// Personal Claude Code choices, shown only inside that agent's manage modal.
	// The settings file is read when the section opens and written from here, so
	// a change Claude Code made itself is what the controls show.
	interface Props {
		agent: IntegrationView;
		models: IntegrationModel[];
		modelsReady: boolean;
	}

	let { agent, models, modelsReady }: Props = $props();

	let config = $state<ClaudeModelConfig | null>(null);
	let model = $state('auto');
	let effort = $state('auto');
	let compact = $state<'auto' | 'custom'>('auto');
	let tokens = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let saved = $state(false);
	let loadError = $state('');
	let saveError = $state('');
	let fieldError = $state<'model' | 'compact' | ''>('');

	const listed = $derived(
		claudePreferenceModels(config?.model ?? '', models, modelsReady, config?.models ?? [])
	);
	const selectedEfforts = $derived(listed.find((entry) => entry.id === model)?.efforts);
	const effortLevels = $derived(
		model === 'auto'
			? []
			: claudeEffortChoices(model, effort === 'auto' ? '' : effort, selectedEfforts)
	);
	const modelOptions = $derived([
		{ value: 'auto', label: $t('ui.pages.integrationsPage.modelPreferencesModelAuto') },
		...listed.map((entry) => ({
			value: entry.id,
			label: unavailable(entry.id)
				? $t('ui.pages.integrationsPage.modelPreferencesUnavailable', {
						values: { model: entry.id }
					})
				: entry.name && entry.name !== entry.id
					? entry.name
					: entry.id
		}))
	]);
	const effortOptions = $derived([
		{ value: 'auto', label: $t('ui.pages.integrationsPage.modelPreferencesEffortAuto') },
		...effortLevels.map((level) => ({
			value: level,
			label: knownEffort(level) ? $t(effortKey(level)) : level
		}))
	]);
	const compactOptions = $derived([
		{ value: 'auto', label: $t('ui.pages.integrationsPage.modelPreferencesCompactAuto') },
		{ value: 'custom', label: $t('ui.pages.integrationsPage.modelPreferencesCompactCustom') }
	]);

	onMount(() => {
		void load();
	});

	function unavailable(id: string): boolean {
		if (id !== config?.model) return false;
		if (modelsReady && models.length > 0) return !models.some((item) => item.id === id);
		return !(config?.models ?? []).some((item) => item.id === id);
	}

	function knownEffort(level: string): boolean {
		return level === 'low' || level === 'medium' || level === 'high' || level === 'xhigh';
	}

	function effortKey(level: string): string {
		switch (level) {
			case 'low':
				return 'ui.pages.integrationsPage.modelPreferencesEffortLow';
			case 'medium':
				return 'ui.pages.integrationsPage.modelPreferencesEffortMedium';
			case 'high':
				return 'ui.pages.integrationsPage.modelPreferencesEffortHigh';
			case 'xhigh':
				return 'ui.pages.integrationsPage.modelPreferencesEffortXhigh';
			default:
				return 'ui.pages.integrationsPage.modelPreferencesEffort';
		}
	}

	function apply(next: ClaudeModelConfig) {
		config = next;
		model = next.model.trim() === '' ? 'auto' : next.model;
		const entry = next.entries.find((item) => item.model === next.model);
		const savedEffort = (entry?.effort || next.effort).trim();
		effort = savedEffort === '' ? 'auto' : savedEffort;
		const savedCompact = (entry?.auto_compact || next.auto_compact).trim();
		if (savedCompact === '' || savedCompact === 'auto') {
			compact = 'auto';
			tokens = '';
		} else {
			compact = 'custom';
			tokens = savedCompact;
		}
	}

	function chooseModel(value: string) {
		model = value;
		saved = false;
		fieldError = '';
		saveError = '';
		if (value === 'auto') {
			effort = 'auto';
			compact = 'auto';
			tokens = '';
			return;
		}
		const entry = config?.entries.find((item) => item.model === value);
		effort = entry?.effort?.trim() ? entry.effort : 'auto';
		const savedCompact = entry?.auto_compact?.trim() ?? '';
		if (savedCompact === '' || savedCompact === 'auto') {
			compact = 'auto';
			tokens = '';
		} else {
			compact = 'custom';
			tokens = savedCompact;
		}
	}

	async function load() {
		loading = true;
		loadError = '';
		try {
			apply(await api<ClaudeModelConfig>(integrationPath(agent.id, 'modelConfig')));
		} catch (error) {
			loadError = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
		}
	}

	async function save() {
		const draft: ClaudePreferenceDraft = { model, effort, compact, tokens };
		const payload = claudePreferencePayload(draft);
		saved = false;
		saveError = '';
		if (!payload.ok) {
			fieldError = payload.error;
			return;
		}
		fieldError = '';
		saving = true;
		try {
			apply(
				await api<ClaudeModelConfig>(integrationPath(agent.id, 'modelConfig'), {
					method: 'POST',
					body: JSON.stringify(payload.body)
				})
			);
			saved = true;
		} catch (error) {
			saveError = error instanceof Error ? error.message : String(error);
		} finally {
			saving = false;
		}
	}
</script>

<section class="space-y-3 border-t border-border pt-4">
	<div class="min-w-0">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.modelPreferencesTitle')}</h3>
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.integrationsPage.modelPreferencesHint')}
		</p>
	</div>

	{#if loading}
		<p class="flex items-center gap-2 text-xs text-muted-foreground">
			<Icon name="loader" size={14} spin />
			{$t('ui.common.loading')}
		</p>
	{:else if loadError}
		<p class="text-sm text-danger" role="alert">{loadError}</p>
	{:else}
		<Field
			id="claude-model"
			label={$t('ui.pages.integrationsPage.modelPreferencesModel')}
			error={fieldError === 'model'
				? $t('ui.pages.integrationsPage.modelPreferencesModelRequired')
				: ''}
		>
			<Select
				id="claude-model"
				options={modelOptions}
				value={model}
				onValueChange={chooseModel}
				disabled={saving}
			/>
		</Field>
		<Field id="claude-effort" label={$t('ui.pages.integrationsPage.modelPreferencesEffort')}>
			<Select
				id="claude-effort"
				options={effortOptions}
				value={effort}
				onValueChange={(value) => {
					effort = value;
					saved = false;
				}}
				disabled={saving || model === 'auto'}
			/>
		</Field>
		<Field
			id="claude-compact"
			label={$t('ui.pages.integrationsPage.modelPreferencesCompact')}
			error={fieldError === 'compact'
				? $t('ui.pages.integrationsPage.modelPreferencesWindowInvalid')
				: ''}
		>
			<Select
				id="claude-compact"
				options={compactOptions}
				value={compact}
				onValueChange={(value) => {
					compact = value === 'custom' ? 'custom' : 'auto';
					saved = false;
					fieldError = '';
				}}
				disabled={saving || model === 'auto'}
			/>
		</Field>
		{#if compact === 'custom' && model !== 'auto'}
			<Field id="claude-window" label={$t('ui.pages.integrationsPage.modelPreferencesTokens')}>
				<Input
					id="claude-window"
					inputmode="numeric"
					value={tokens}
					disabled={saving}
					oninput={(event) => {
						tokens = event.currentTarget.value;
						saved = false;
						fieldError = '';
					}}
				/>
			</Field>
		{/if}
		<div class="flex flex-wrap items-center gap-3">
			<Button variant="outline" disabled={saving} onclick={() => void save()}>
				{#if saving}
					<Icon name="loader" size={14} spin />
				{/if}
				{$t(
					saving
						? 'ui.pages.integrationsPage.modelPreferencesSaving'
						: 'ui.pages.integrationsPage.modelPreferencesSave'
				)}
			</Button>
			{#if saved}
				<span class="text-xs text-muted-foreground"
					>{$t('ui.pages.integrationsPage.modelPreferencesSaved')}</span
				>
			{/if}
		</div>
		{#if saveError}
			<p class="text-sm text-danger" role="alert">{saveError}</p>
		{/if}
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.integrationsPage.modelPreferencesRestart')}
		</p>
	{/if}
</section>
