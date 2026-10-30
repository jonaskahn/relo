<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import SettingsCard from './settings-card.svelte';
	import { Switch } from 'bits-ui';
	import FreeConsentDialog from '$lib/components/providers/free-consent-dialog.svelte';
	interface Props {
		value: boolean;
	}
	let { value }: Props = $props();
	let enabled = $state(untrack(() => value));
	let consentOpen = $state(false);
	let busy = $state(false);
	let error = $state('');
	async function toggle() {
		if (busy) return;
		if (!enabled) {
			consentOpen = true;
			return;
		}
		busy = true;
		error = '';
		try {
			await api('/settings/connections', {
				method: 'PATCH',
				body: JSON.stringify({ free_enabled: false })
			});
			enabled = false;
			toast.success($t('ui.settingsPage.freeConnectionsDisabledToast'));
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}
</script>

<SettingsCard
	id="free-connections"
	title={$t('ui.settingsPage.freeConnectionsLabel')}
	description={$t('ui.settingsPage.freeConnectionsHint')}
>
	<Switch.Root
		checked={enabled}
		disabled={busy}
		onCheckedChange={() => void toggle()}
		aria-label={$t('ui.settingsPage.freeConnectionsLabel')}
	/>
	{#if error}<p class="text-sm text-danger" role="alert">{error}</p>{/if}
</SettingsCard>
<FreeConsentDialog bind:open={consentOpen} onenabled={() => (enabled = true)} />
