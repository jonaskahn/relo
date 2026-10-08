<script lang="ts">
	import { t } from 'svelte-i18n';

	import { api, acceptLanguage } from '$lib/api';
	import AccentPicker from '$lib/components/shell/accent-picker.svelte';
	import LanguagePicker from '$lib/components/ui/language-picker.svelte';
	import SettingsCard from './settings-card.svelte';
	import SettingsRow from './settings-row.svelte';
	import ThemePicker from '$lib/components/shell/theme-picker.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import { consoleState } from '$lib/console-state.svelte';
	import {
		auto,
		languageChoices,
		savedLanguage,
		setLanguage,
		type LanguageChoice
	} from '$lib/i18n';
	import type { QuotaDisplay } from '$lib/types';

	// ConsoleCard is the appearance and language every surface shares: the
	// same controls as the appearance popover, applied on selection.
	let savingLanguage = $state(false);
	let languageError = $state('');

	const quotaOptions: { value: QuotaDisplay; label: string }[] = $derived([
		{ value: 'used', label: $t('ui.settingsPage.quotaDisplayUsed') },
		{ value: 'remaining', label: $t('ui.settingsPage.quotaDisplayRemaining') }
	]);

	const selectedLanguage = $derived(
		languageChoices.find((choice) => choice === $savedLanguage) ?? auto
	);

	async function saveLanguage(value: LanguageChoice) {
		const previous = $savedLanguage;
		const previousRendered = acceptLanguage();
		savedLanguage.set(value);
		savingLanguage = true;
		languageError = '';
		try {
			await setLanguage(value === auto ? navigator.language : value);
			await api('/settings/language', {
				method: 'PATCH',
				body: JSON.stringify({ language: value })
			});
			if (value === auto) {
				try {
					const status = await api<{ language?: string }>('/status');
					await setLanguage(status.language);
				} catch {
					// The choice was saved; the browser locale is the best immediate
					// approximation until the next status read or page load.
				}
			}
		} catch (error) {
			savedLanguage.set(previous);
			await setLanguage(previousRendered);
			languageError = error instanceof Error ? error.message : $t('ui.settings.language.failure');
		} finally {
			savingLanguage = false;
		}
	}
</script>

<SettingsCard
	id="settings-console"
	title={$t('ui.settingsPage.sectionConsole')}
	description={$t('ui.settingsPage.appearanceDescription')}
>
	<div class="flex flex-col gap-5">
		<SettingsRow label={$t('ui.settingsPage.themeLabel')}>
			<ThemePicker layout="page" />
		</SettingsRow>
		<SettingsRow label={$t('ui.settingsPage.accentLabel')}>
			<div class="max-w-2xl">
				<AccentPicker layout="page" />
			</div>
		</SettingsRow>
		<SettingsRow
			label={$t('ui.settingsPage.quotaDisplayLabel')}
			hint={$t('ui.settingsPage.quotaDisplayHint')}
		>
			<Segmented
				value={consoleState.quotaDisplay}
				options={quotaOptions}
				ariaLabel={$t('ui.settingsPage.quotaDisplayLabel')}
				onchange={(display) => void consoleState.setQuotaDisplay(display)}
			/>
		</SettingsRow>
		<SettingsRow
			label={$t('ui.settings.language.label')}
			hint={$t('ui.settings.language.description')}
		>
			<div class="flex max-w-2xl flex-col gap-2">
				<LanguagePicker
					layout="page"
					value={selectedLanguage}
					disabled={savingLanguage}
					onselect={(code) => void saveLanguage(code)}
				/>
				{#if languageError}
					<p class="text-sm text-destructive" role="alert">{languageError}</p>
				{/if}
			</div>
		</SettingsRow>
	</div>
</SettingsCard>
