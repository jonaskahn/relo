<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import SettingsCard from './settings-card.svelte';
	import SettingsRow from './settings-row.svelte';
	import SettingsSave from './settings-save.svelte';
	import SettingsSwitch from './settings-switch.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import { cloneDraft, draftDirty } from '$lib/settings-draft';
	import type { SystemSettings } from '$lib/types';

	// DaemonCard edits when Relo starts, how much it logs, and where it
	// checks for updates. The switch and the fields save together; the log
	// level and the update feed apply at the next daemon start, which the
	// restart banner reports.
	interface Props {
		value: SystemSettings;
		onsaved: (next: SystemSettings) => void;
	}

	let { value, onsaved }: Props = $props();

	let draft = $state(untrack(() => cloneDraft(value)));
	let saved = $state(untrack(() => cloneDraft(value)));
	let saving = $state(false);
	let error = $state('');

	const dirty = $derived(draftDirty(draft, saved));

	async function save() {
		if (saving) return;
		saving = true;
		error = '';
		try {
			const next = await api<SystemSettings>('/settings/system', {
				method: 'PATCH',
				body: JSON.stringify(draft)
			});
			draft = cloneDraft(next);
			saved = cloneDraft(next);
			onsaved(next);
			toast.success($t('ui.settingsPage.daemonSaved'));
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			saving = false;
		}
	}
</script>

<SettingsCard
	id="settings-daemon"
	title={$t('ui.settingsPage.sectionDaemon')}
	description={$t('ui.settingsPage.daemonDescription')}
>
	{#snippet footer()}
		<SettingsSave {dirty} {saving} {error} onsave={save} />
	{/snippet}
	<div class="flex flex-col gap-5">
		<SettingsSwitch
			id="autostart"
			bind:checked={draft.autostart}
			label={$t('ui.settingsPage.autostartLabel')}
			hint={$t('ui.settingsPage.autostartHint')}
			disabled={saving}
		/>
		<SettingsRow
			label={$t('ui.settingsPage.logLevelLabel')}
			hint={$t('ui.settingsPage.logLevelHint')}
		>
			<NativeSelect
				id="log-level"
				class="max-w-40 max-sm:h-11"
				bind:value={draft.log_level}
				disabled={saving}
				aria-label={$t('ui.settingsPage.logLevelLabel')}
			>
				<option value="debug">debug</option>
				<option value="info">info</option>
				<option value="warn">warn</option>
				<option value="error">error</option>
			</NativeSelect>
		</SettingsRow>
		<SettingsSwitch
			id="log-traffic"
			bind:checked={draft.log_traffic}
			label={$t('ui.settingsPage.trafficLogLabel')}
			hint={$t('ui.settingsPage.trafficLogHint')}
			disabled={saving}
		/>
		<SettingsRow
			label={$t('ui.settingsPage.updatesURL')}
			hint={$t('ui.settingsPage.updatesURLHint')}
		>
			<Input
				id="updates-url"
				class="mono-data max-sm:h-11"
				bind:value={draft.updates_url}
				disabled={saving}
				aria-label={$t('ui.settingsPage.updatesURL')}
			/>
		</SettingsRow>
		<SettingsRow
			label={$t('ui.settingsPage.updatesDownload')}
			hint={$t('ui.settingsPage.updatesDownloadHint')}
		>
			<Input
				id="updates-download"
				class="mono-data max-sm:h-11"
				bind:value={draft.updates_download}
				disabled={saving}
				aria-label={$t('ui.settingsPage.updatesDownload')}
			/>
		</SettingsRow>
	</div>
</SettingsCard>
