<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	interface Props {
		open?: boolean;
		onenabled: () => void;
	}
	let { open = $bindable(false), onenabled }: Props = $props();
	let busy = $state(false);
	let error = $state('');
	async function enable() {
		if (busy) return;
		busy = true;
		error = '';
		try {
			await api('/settings/connections', {
				method: 'PATCH',
				body: JSON.stringify({ free_enabled: true })
			});
			open = false;
			onenabled();
			toast.success($t('ui.pages.providersPage.freeConsent.enabledToast'));
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			busy = false;
		}
	}
</script>

<ConfirmDialog
	bind:open
	title={$t('ui.pages.providersPage.freeConsent.title')}
	body={$t('ui.pages.providersPage.freeConsent.body')}
	confirmLabel={$t('ui.pages.providersPage.freeConsent.confirm')}
	{busy}
	{error}
	onconfirm={enable}
/>
