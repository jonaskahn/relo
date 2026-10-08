<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import SettingsCard from './settings-card.svelte';
	import SettingsRow from './settings-row.svelte';
	import SettingsSwitch from './settings-switch.svelte';
	import type { AccessSettings, SecretsSettings } from '$lib/types';

	// AccessCard holds the external-access switch, which applies at once, and
	// the two choices an operator changes in config.toml only: the sign-in
	// and the credential vault. A switch that applies at once never shares a
	// panel with a form that has an explicit save.
	interface Props {
		access: AccessSettings;
		secrets: SecretsSettings;
		onsaved: (next: AccessSettings) => void;
	}

	let { access, secrets, onsaved }: Props = $props();

	let checked = $state(untrack(() => access.allow_external));
	let saving = $state(false);
	let error = $state('');

	async function setExternal(next: boolean) {
		if (saving) return;
		const previous = checked;
		checked = next;
		saving = true;
		error = '';
		try {
			await api('/settings/access', {
				method: 'PATCH',
				body: JSON.stringify({ allow_external: next })
			});
			onsaved({ ...access, allow_external: next });
		} catch (failure) {
			checked = previous;
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			saving = false;
		}
	}
</script>

<SettingsCard
	id="settings-access"
	title={$t('ui.settingsPage.sectionAccess')}
	description={$t('ui.settingsPage.accessDescription')}
>
	<div class="flex flex-col gap-5">
		<SettingsSwitch
			id="allow-external"
			bind:checked
			label={$t('ui.settingsPage.externalLabel')}
			hint={$t('ui.settingsPage.externalHint')}
			disabled={saving}
			onCheckedChange={(next) => void setExternal(next)}
		/>
		<SettingsRow label={$t('ui.settingsPage.loginLabel')} hint={$t('ui.settingsPage.loginHint')}>
			<p class="text-sm">
				{access.login
					? $t('ui.settingsPage.loginRequired')
					: $t('ui.settingsPage.loginNotRequired')}
			</p>
		</SettingsRow>
		<SettingsRow label={$t('ui.settingsPage.vaultLabel')} hint={$t('ui.settingsPage.vaultHint')}>
			<p class="mono-data">
				{secrets.keychain
					? $t('ui.settingsPage.vaultKeychain')
					: $t('ui.settingsPage.vaultKeyFile', { values: { file: secrets.key_file } })}
			</p>
		</SettingsRow>
		{#if error}
			<p
				class="settings-field-action flex items-start gap-1.5 text-sm text-destructive"
				role="alert"
			>
				<Icon name="circle-alert" size={14} class="mt-0.5 shrink-0" />
				<span>{error}</span>
			</p>
		{/if}
	</div>
</SettingsCard>
