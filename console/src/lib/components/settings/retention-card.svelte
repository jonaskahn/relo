<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import Icon from '$lib/components/ui/icon.svelte';
	import SettingsCard from './settings-card.svelte';
	import SettingsRow from './settings-row.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import { formatBytes } from '$lib/format';
	import { stepSwap } from '$lib/modal-motion';
	import { MIN_USAGE_DAYS, validUsageDays } from '$lib/settings-retention';
	import { cloneDraft, draftDirty } from '$lib/settings-draft';
	import type { RetentionPreview, RetentionSettings } from '$lib/types';

	// RetentionCard owns the usage-log budget and the cleanup that applies
	// the stored budget. The reclaim sweep lives in its own card beside it.
	interface Props {
		value: RetentionSettings;
		onsaved: (next: RetentionSettings) => void;
	}

	let { value, onsaved }: Props = $props();

	let draft = $state(untrack(() => cloneDraft(value)));
	let saved = $state(untrack(() => cloneDraft(value)));
	let saving = $state(false);
	let error = $state('');

	let previewing = $state(false);
	let preview = $state<RetentionPreview | null>(null);
	let previewError = $state('');

	let cleaningUp = $state(false);
	let cleanupError = $state('');

	const dirty = $derived(draftDirty(draft, saved));
	const invalid = $derived(!validUsageDays(Number(draft.usage_days)));

	async function save() {
		if (saving || invalid) return;
		saving = true;
		error = '';
		try {
			const next = await api<RetentionSettings>('/settings/retention', {
				method: 'PUT',
				body: JSON.stringify(draft)
			});
			draft = cloneDraft(next);
			saved = cloneDraft(next);
			preview = null;
			onsaved(next);
			toast.success($t('ui.settingsPage.retentionSaved'));
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			saving = false;
		}
	}

	async function previewRetention() {
		if (previewing || invalid) return;
		previewing = true;
		previewError = '';
		try {
			preview = await api<RetentionPreview>('/settings/retention/preview', {
				method: 'PUT',
				body: JSON.stringify(draft)
			});
		} catch (failure) {
			preview = null;
			previewError = failure instanceof Error ? failure.message : String(failure);
		} finally {
			previewing = false;
		}
	}

	async function cleanUpNow() {
		if (cleaningUp) return;
		cleaningUp = true;
		cleanupError = '';
		try {
			const report = await api<{ RowsDeleted: number }>('/settings/retention/run', {
				method: 'POST'
			});
			toast.success($t('ui.settingsPage.cleanupDone', { values: { rows: report.RowsDeleted } }));
		} catch (failure) {
			cleanupError = failure instanceof Error ? failure.message : String(failure);
		} finally {
			cleaningUp = false;
		}
	}
</script>

<SettingsCard
	id="settings-retention"
	title={$t('ui.settingsPage.sectionRetention')}
	description={$t('ui.settingsPage.retentionDescription')}
>
	<div class="flex flex-col gap-5">
		<SettingsRow
			label={$t('ui.settingsPage.usageDays')}
			hint={$t('ui.settingsPage.retentionFloorHint', { values: { min: MIN_USAGE_DAYS } })}
		>
			<Input
				id="ret-days"
				type="number"
				min="0"
				step="1"
				class="mono-data max-w-40 max-sm:h-11"
				bind:value={draft.usage_days}
				disabled={saving}
				aria-invalid={invalid ? true : undefined}
			/>
			{#if invalid}
				<p class="mt-1.5 text-xs text-destructive" role="alert">
					{$t('ui.settingsPage.retentionMinimum', { values: { min: MIN_USAGE_DAYS } })}
				</p>
			{/if}
		</SettingsRow>
		<SettingsRow label={$t('ui.settingsPage.maxEvents')}>
			<Input
				id="ret-events"
				type="number"
				min="0"
				class="mono-data max-w-40 max-sm:h-11"
				bind:value={draft.max_events}
				disabled={saving}
			/>
		</SettingsRow>
		<SettingsRow label={$t('ui.settingsPage.maxBytes')}>
			<Input
				id="ret-bytes"
				type="number"
				min="0"
				class="mono-data max-w-40 max-sm:h-11"
				bind:value={draft.max_bytes}
				disabled={saving}
			/>
		</SettingsRow>
		<div class="settings-field-action flex flex-wrap items-center gap-2">
			<Button type="button" size="sm" onclick={save} disabled={saving || invalid}>
				<Icon name={saving ? 'loader' : 'check'} size={14} spin={saving} />
				{saving ? $t('ui.common.saving') : $t('ui.common.save')}
			</Button>
			{#if dirty && !saving}
				<span class="text-xs text-muted-foreground" in:stepSwap>
					{$t('ui.settingsPage.unsavedChanges')}
				</span>
			{/if}
			<Button
				type="button"
				size="sm"
				variant="outline"
				onclick={previewRetention}
				disabled={previewing || invalid}
			>
				<Icon name={previewing ? 'loader' : 'eye'} size={14} spin={previewing} />
				{previewing ? $t('ui.settingsPage.previewing') : $t('ui.settingsPage.preview')}
			</Button>
			<Button type="button" size="sm" variant="outline" onclick={cleanUpNow} disabled={cleaningUp}>
				<Icon name={cleaningUp ? 'loader' : 'trash'} size={14} spin={cleaningUp} />
				{cleaningUp ? $t('ui.settingsPage.cleanupRunning') : $t('ui.settingsPage.cleanupNow')}
			</Button>
		</div>
		{#if error}
			<p
				class="settings-field-action flex items-start gap-1.5 text-sm text-destructive"
				role="alert"
			>
				<Icon name="circle-alert" size={14} class="mt-0.5 shrink-0" />
				<span>{error}</span>
			</p>
		{/if}
		{#if previewError}
			<p
				class="settings-field-action flex items-start gap-1.5 text-sm text-destructive"
				role="alert"
			>
				<Icon name="circle-alert" size={14} class="mt-0.5 shrink-0" />
				<span>{previewError}</span>
			</p>
		{/if}
		{#if cleanupError}
			<p
				class="settings-field-action flex items-start gap-1.5 text-sm text-destructive"
				role="alert"
			>
				<Icon name="circle-alert" size={14} class="mt-0.5 shrink-0" />
				<span>{cleanupError}</span>
			</p>
		{/if}
		{#if preview}
			<p class="settings-field-action text-sm text-muted-foreground" role="status" in:stepSwap>
				{$t('ui.settingsPage.previewResult')
					.replace('{rows}', String(preview.RowsDeleted))
					.replace('{bytes}', formatBytes(preview.EstimatedBytesFreed))}
			</p>
		{/if}
	</div>
</SettingsCard>
