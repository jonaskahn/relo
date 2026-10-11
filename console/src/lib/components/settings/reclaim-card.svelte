<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import SettingsCard from './settings-card.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { formatBytes } from '$lib/format';
	import { stepSwap } from '$lib/modal-motion';
	import type { SweepStatus } from '$lib/types';

	// ReclaimCard owns the maintenance sweep that deletes stored bodies and
	// compacts the database. It reads the sweep on mount and polls while one
	// runs, so the report appears without leaving the page.
	let sweep = $state<SweepStatus | null>(null);
	let sweepAsking = $state(false);
	let sweepError = $state('');
	let sweepTimer: ReturnType<typeof setInterval> | undefined;
	let alive = true;

	const sweepRunning = $derived(sweep !== null && sweep.state === 'working');

	onMount(() => {
		void readSweep();
	});

	onDestroy(() => {
		alive = false;
		if (sweepTimer !== undefined) clearInterval(sweepTimer);
	});

	async function readSweep() {
		const wasRunning = sweepRunning;
		try {
			sweep = await api<SweepStatus>('/settings/retention/sweep');
		} catch {
			// A daemon that cannot answer keeps the state it had, and the next
			// read picks the sweep up again.
			followSweep();
			return;
		}
		if (wasRunning && sweep?.state === 'done') {
			toast.success(
				$t('ui.settingsPage.sweepDoneToast', {
					values: {
						before: formatBytes(sweep.report?.live_bytes_before ?? 0),
						after: formatBytes(sweep.report?.live_bytes_after ?? 0)
					}
				})
			);
		}
		followSweep();
	}

	function followSweep() {
		if (sweepTimer !== undefined) clearInterval(sweepTimer);
		sweepTimer = undefined;
		if (alive && sweepRunning) sweepTimer = setInterval(() => void readSweep(), 1000);
	}

	async function startSweep() {
		sweepAsking = false;
		sweepError = '';
		try {
			sweep = await api<SweepStatus>('/settings/retention/sweep', { method: 'POST' });
			followSweep();
		} catch (failure) {
			sweepError = failure instanceof Error ? failure.message : String(failure);
		}
	}
</script>

<SettingsCard
	id="settings-reclaim"
	title={$t('ui.settingsPage.sweepTitle')}
	description={$t('ui.settingsPage.sweepDescription')}
>
	<div class="flex flex-col gap-3">
		<div class="settings-field-action">
			<Button
				type="button"
				size="sm"
				variant="outline"
				disabled={sweepRunning}
				onclick={() => (sweepAsking = true)}
			>
				<Icon name="database" size={14} />
				{$t('ui.settingsPage.sweepAction')}
			</Button>
		</div>
		{#if sweepRunning}
			<p class="mt-1 flex items-center gap-1.5 text-sm text-muted-foreground" role="status">
				<Icon name="loader" size={13} spin />
				{$t('ui.settingsPage.sweepWorking')}
			</p>
		{:else if sweep?.state === 'failed'}
			<p class="mt-1 text-sm text-destructive" role="alert">
				{$t('ui.settingsPage.sweepFailed', { values: { error: sweep.error ?? '' } })}
			</p>
		{/if}
		{#if sweepError}
			<p class="mt-1 text-sm text-destructive" role="alert">
				{$t('ui.settingsPage.sweepFailed', { values: { error: sweepError } })}
			</p>
		{/if}
		{#if sweep?.report}
			<div class="mt-2 flex flex-col gap-1.5" in:stepSwap>
				<div class="flex items-center justify-between gap-3 text-xs">
					<span class="text-muted-foreground">{$t('ui.settingsPage.sweepReportDays')}</span>
					<span class="mono-data">{sweep.report.finalized_days.toLocaleString()}</span>
				</div>
				<div class="flex items-center justify-between gap-3 text-xs">
					<span class="text-muted-foreground">{$t('ui.settingsPage.sweepReportRows')}</span>
					<span class="mono-data">{sweep.report.rows_deleted.toLocaleString()}</span>
				</div>
				<div class="flex items-center justify-between gap-3 text-xs">
					<span class="text-muted-foreground">{$t('ui.settingsPage.sweepReportCaptures')}</span>
					<span class="mono-data">{sweep.report.captures_deleted.toLocaleString()}</span>
				</div>
				<div class="flex items-center justify-between gap-3 text-xs">
					<span class="text-muted-foreground">{$t('ui.settingsPage.sweepReportSize')}</span>
					<span class="mono-data"
						>{formatBytes(sweep.report.live_bytes_before)} → {formatBytes(
							sweep.report.live_bytes_after
						)}</span
					>
				</div>
				{#if !sweep.report.vacuumed}
					<p class="text-xs leading-relaxed text-muted-foreground">
						{$t('ui.settingsPage.sweepNotCompacted')}
					</p>
				{/if}
			</div>
		{/if}
	</div>
</SettingsCard>

<ConfirmDialog
	bind:open={sweepAsking}
	title={$t('ui.settingsPage.sweepConfirmTitle')}
	body={$t('ui.settingsPage.sweepConfirmBody')}
	confirmLabel={$t('ui.settingsPage.sweepAction')}
	tone="destructive"
	icon="database"
	onconfirm={startSweep}
/>
