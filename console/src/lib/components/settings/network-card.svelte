<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import SettingsCard from './settings-card.svelte';
	import SettingsRow from './settings-row.svelte';
	import SettingsSave from './settings-save.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import { cloneDraft, draftDirty } from '$lib/settings-draft';
	import type { ServerSettings } from '$lib/types';

	// NetworkCard edits where the daemon listens. The choices apply at the
	// next daemon start, which the restart banner reports.
	interface Props {
		value: ServerSettings;
		onsaved: (next: ServerSettings) => void;
	}

	let { value, onsaved }: Props = $props();

	let draft = $state(untrack(() => cloneDraft(value)));
	let saved = $state(untrack(() => cloneDraft(value)));
	let saving = $state(false);
	let error = $state('');

	const dirty = $derived(draftDirty(draft, saved));

	const portProblem = $derived.by(() => {
		const ports = [draft.port, draft.openai_port, draft.anthropic_port, draft.gemini_port];
		if (ports.some((port) => !Number.isInteger(port) || port < 0 || port > 65535)) {
			return $t('ui.settingsPage.portInvalid');
		}
		const seen = new Set<number>();
		for (const port of ports) {
			if (port === 0) continue;
			if (seen.has(port)) return $t('ui.settingsPage.duplicatePort');
			seen.add(port);
		}
		return '';
	});
	const bindProblem = $derived(draft.bind.trim() === '' ? $t('ui.settingsPage.bindInvalid') : '');
	const problem = $derived(bindProblem || portProblem);

	async function save() {
		if (saving || problem) return;
		saving = true;
		error = '';
		try {
			const next = await api<ServerSettings>('/settings/network', {
				method: 'PATCH',
				body: JSON.stringify(draft)
			});
			draft = cloneDraft(next);
			saved = cloneDraft(next);
			onsaved(next);
			toast.success($t('ui.settingsPage.networkSaved'));
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			saving = false;
		}
	}
</script>

<SettingsCard
	id="settings-network"
	title={$t('ui.settingsPage.sectionNetwork')}
	description={$t('ui.settingsPage.networkDescription')}
>
	{#snippet footer()}
		<SettingsSave {dirty} {saving} disabled={problem !== ''} {error} onsave={save} />
	{/snippet}
	<div class="flex flex-col gap-5">
		<SettingsRow label={$t('ui.settingsPage.bindLabel')} hint={$t('ui.settingsPage.bindHint')}>
			<Input
				id="bind-address"
				class="mono-data max-w-56 max-sm:h-11"
				bind:value={draft.bind}
				disabled={saving}
				aria-label={$t('ui.settingsPage.bindLabel')}
			/>
		</SettingsRow>
		<SettingsRow label={$t('ui.settingsPage.portLabel')} hint={$t('ui.settingsPage.portHint')}>
			<Input
				id="console-port"
				type="number"
				min="1"
				max="65535"
				class="mono-data w-28 max-sm:h-11"
				bind:value={draft.port}
				disabled={saving}
				aria-label={$t('ui.settingsPage.portLabel')}
			/>
		</SettingsRow>
		<SettingsRow
			label={$t('ui.settingsPage.dataPlaneLabel')}
			hint={$t('ui.settingsPage.dataPlaneHint')}
		>
			<div class="grid grid-cols-3 gap-2">
				<Field id="port-openai" label="OpenAI">
					<Input
						id="port-openai"
						type="number"
						min="0"
						max="65535"
						class="mono-data max-sm:h-11"
						bind:value={draft.openai_port}
						disabled={saving}
					/>
				</Field>
				<Field id="port-anthropic" label="Anthropic">
					<Input
						id="port-anthropic"
						type="number"
						min="0"
						max="65535"
						class="mono-data max-sm:h-11"
						bind:value={draft.anthropic_port}
						disabled={saving}
					/>
				</Field>
				<Field id="port-gemini" label="Gemini">
					<Input
						id="port-gemini"
						type="number"
						min="0"
						max="65535"
						class="mono-data max-sm:h-11"
						bind:value={draft.gemini_port}
						disabled={saving}
					/>
				</Field>
			</div>
		</SettingsRow>
	</div>
	{#if problem}
		<p
			class="settings-field-action mt-3 flex items-start gap-1.5 text-sm text-destructive"
			role="alert"
		>
			<Icon name="circle-alert" size={14} class="mt-0.5 shrink-0" />
			<span>{problem}</span>
		</p>
	{/if}
</SettingsCard>
