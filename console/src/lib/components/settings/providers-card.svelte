<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import SettingsCard from './settings-card.svelte';
	import SettingsRow from './settings-row.svelte';
	import SettingsSave from './settings-save.svelte';
	import RetryWaitsEditor from './retry-waits-editor.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import { cloneDraft, draftDirty } from '$lib/settings-draft';
	import {
		CALL_WAIT_PRESETS,
		FAILOVER_COOLDOWN_PRESETS,
		callWaitLabel,
		failoverCooldownLabel,
		retryBackoffValid
	} from '$lib/upstream-settings';
	import type { ProvidersSettings } from '$lib/types';

	// ProvidersCard edits how the daemon reaches providers and their model
	// directory: the models.dev document, the outbound proxy, the call wait,
	// the retry waits, and the failover wait. Every field saves together, so a
	// chip never applies beside a form that has an explicit save.
	interface Props {
		value: ProvidersSettings;
		onsaved: (next: ProvidersSettings) => void;
	}

	let { value, onsaved }: Props = $props();

	let draft = $state(untrack(() => cloneDraft(value)));
	let saved = $state(untrack(() => cloneDraft(value)));
	let saving = $state(false);
	let error = $state('');
	let retryOpen = $state(false);

	const dirty = $derived(draftDirty(draft, saved));
	const retryInvalid = $derived(!retryBackoffValid(draft.retry_backoff));
	const catalogProblem = $derived(
		draft.catalog_url.trim() === '' ? $t('ui.settingsPage.catalogInvalid') : ''
	);

	async function save() {
		if (saving || retryInvalid || catalogProblem) return;
		saving = true;
		error = '';
		try {
			const next = await api<ProvidersSettings>('/settings/providers', {
				method: 'PATCH',
				body: JSON.stringify(draft)
			});
			draft = cloneDraft(next);
			saved = cloneDraft(next);
			onsaved(next);
			toast.success($t('ui.settingsPage.providersSaved'));
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			saving = false;
		}
	}
</script>

<SettingsCard
	id="settings-providers"
	title={$t('ui.settingsPage.sectionProviders')}
	description={$t('ui.settingsPage.providersDescription')}
>
	{#snippet footer()}
		<SettingsSave
			{dirty}
			{saving}
			disabled={retryInvalid || catalogProblem !== ''}
			{error}
			onsave={save}
		/>
	{/snippet}
	<div class="flex flex-col gap-5">
		<SettingsRow
			label={$t('ui.settingsPage.catalogLabel')}
			hint={$t('ui.settingsPage.catalogHint')}
		>
			<Input
				id="catalog-url"
				class="mono-data max-sm:h-11"
				bind:value={draft.catalog_url}
				disabled={saving}
				aria-label={$t('ui.settingsPage.catalogLabel')}
				aria-invalid={catalogProblem ? true : undefined}
			/>
		</SettingsRow>
		<SettingsRow label={$t('ui.settingsPage.proxyURL')} hint={$t('ui.settingsPage.proxyHint')}>
			<Input
				id="proxy-url"
				class="mono-data max-sm:h-11"
				bind:value={draft.proxy_url}
				disabled={saving}
				aria-label={$t('ui.settingsPage.proxyURL')}
				placeholder="http://127.0.0.1:7890"
			/>
		</SettingsRow>
		<SettingsRow
			label={$t('ui.settingsPage.timeoutLabel')}
			hint={$t('ui.settingsPage.timeoutHint')}
		>
			<div class="flex flex-wrap gap-2">
				{#each CALL_WAIT_PRESETS as seconds (seconds)}
					<Button
						type="button"
						variant="chip"
						size="sm"
						aria-pressed={draft.timeout_seconds === seconds}
						disabled={saving}
						onclick={() => (draft = { ...draft, timeout_seconds: seconds })}
					>
						{callWaitLabel(seconds)}
					</Button>
				{/each}
			</div>
		</SettingsRow>
		<SettingsRow label={$t('ui.settingsPage.retryLabel')} hint={$t('ui.settingsPage.retryHint')}>
			<div>
				<button
					type="button"
					class="settings-disclosure-toggle"
					aria-expanded={retryOpen}
					aria-controls="settings-retry-panel"
					aria-label="{$t('ui.common.edit')}: {$t('ui.settingsPage.retryLabel')}"
					onclick={() => (retryOpen = !retryOpen)}
				>
					<Icon name={retryOpen ? 'chevron-down' : 'chevron-right'} size={16} />
					{$t('ui.common.edit')}
				</button>
				<div id="settings-retry-panel" class="settings-disclosure" class:open={retryOpen}>
					<div class="settings-disclosure-body">
						<div class="disclosure-pad min-h-0 overflow-hidden">
							<RetryWaitsEditor
								windows={draft.retry_backoff}
								onchange={(next) => (draft = { ...draft, retry_backoff: next })}
								disabled={saving}
								invalid={retryInvalid}
							/>
						</div>
					</div>
				</div>
			</div>
		</SettingsRow>
		<SettingsRow
			label={$t('ui.settingsPage.failoverLabel')}
			hint={$t('ui.settingsPage.failoverHint')}
		>
			<div class="flex flex-wrap gap-2">
				{#each FAILOVER_COOLDOWN_PRESETS as seconds (seconds)}
					<Button
						type="button"
						variant="chip"
						size="sm"
						aria-pressed={draft.failover_cooldown_seconds === seconds}
						disabled={saving}
						onclick={() => (draft = { ...draft, failover_cooldown_seconds: seconds })}
					>
						{seconds === 0 ? $t('ui.common.none') : failoverCooldownLabel(seconds)}
					</Button>
				{/each}
			</div>
		</SettingsRow>
	</div>
</SettingsCard>
