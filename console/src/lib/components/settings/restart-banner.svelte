<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { daemonRestartPath } from '$lib/daemon-lifecycle';

	// RestartBanner names the values awaiting a daemon restart and offers the
	// one action that applies them. It sits above the cards, so it is the
	// first thing an operator sees after saving a listener or the log level.
	interface Props {
		onrestarted?: () => void;
	}

	let { onrestarted }: Props = $props();

	let asking = $state(false);
	let restarting = $state(false);
	let error = $state('');

	async function restart() {
		asking = false;
		restarting = true;
		error = '';
		try {
			await api(daemonRestartPath('restart'), { method: 'POST' });
			toast.success($t('ui.sidebar.restarted'));
			onrestarted?.();
		} catch (failure) {
			error = failure instanceof Error ? failure.message : String(failure);
		} finally {
			restarting = false;
		}
	}
</script>

<div
	class="flex flex-wrap items-start gap-x-4 gap-y-3 rounded-panel border border-warn/35 bg-warn/10 p-[var(--card-pad)]"
>
	<Icon name="alert-triangle" size={18} class="mt-0.5 shrink-0 text-warn" />
	<div class="min-w-[16rem] flex-1">
		<p class="text-sm font-medium">{$t('ui.settingsPage.restartPendingTitle')}</p>
		<p class="mt-0.5 max-w-[72ch] text-sm leading-relaxed text-muted-foreground">
			{$t('ui.settingsPage.restartPendingBody')}
		</p>
		{#if error}
			<p class="mt-1.5 flex items-start gap-1.5 text-sm text-destructive" role="alert">
				<Icon name="circle-alert" size={14} class="mt-0.5 shrink-0" />
				<span>{error}</span>
			</p>
		{/if}
	</div>
	<Button
		type="button"
		size="sm"
		variant="outline"
		disabled={restarting}
		onclick={() => (asking = true)}
	>
		<Icon name={restarting ? 'loader' : 'refresh'} size={14} spin={restarting} />
		{$t('ui.sidebar.restart')}
	</Button>
</div>

<ConfirmDialog
	bind:open={asking}
	title={$t('ui.sidebar.restartTitle')}
	body={$t('ui.sidebar.restartBody')}
	confirmLabel={$t('ui.sidebar.restart')}
	icon="refresh"
	onconfirm={restart}
/>
