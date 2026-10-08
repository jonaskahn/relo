<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { stepSwap } from '$lib/modal-motion';

	// SettingsSave is the one explicit save a card ends with: the Save button
	// that waits for an edit, the unsaved marker beside it, and the failure
	// line under it. Both sit on the value column, level with the controls
	// they act on, so a failure stays next to the control that caused it.
	interface Props {
		dirty: boolean;
		saving: boolean;
		disabled?: boolean;
		error?: string;
		onsave: () => void;
	}

	let { dirty, saving, disabled = false, error = '', onsave }: Props = $props();
</script>

<div class="settings-field-action mt-5 flex flex-wrap items-center gap-3">
	<Button type="button" onclick={onsave} disabled={saving || disabled || !dirty}>
		<Icon name={saving ? 'loader' : 'check'} size={14} spin={saving} />
		{saving ? $t('ui.common.saving') : $t('ui.common.save')}
	</Button>
	{#if dirty && !saving}
		<span class="text-xs text-muted-foreground" in:stepSwap
			>{$t('ui.settingsPage.unsavedChanges')}</span
		>
	{/if}
</div>
{#if error}
	<p
		class="settings-field-action mt-2 flex items-start gap-1.5 text-sm text-destructive"
		role="alert"
	>
		<Icon name="circle-alert" size={14} class="mt-0.5 shrink-0" />
		<span>{error}</span>
	</p>
{/if}
