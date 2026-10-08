<script lang="ts">
	import { t } from 'svelte-i18n';
	import { goto } from '$app/navigation';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import { daemonActionPath, type DaemonActionKind } from '$lib/daemon-lifecycle';
	import { daemonDialog } from '$lib/daemon-dialog.svelte';
	import { session } from '$lib/session.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { cn } from '$lib/utils';

	const manageActions: {
		kind: DaemonActionKind;
		icon: IconName;
		labelKey: string;
		off?: boolean;
	}[] = [
		{ kind: 'restart', icon: 'refresh', labelKey: 'ui.sidebar.manageRestart' },
		{ kind: 'shutdown', icon: 'power', labelKey: 'ui.sidebar.manageShutdown', off: true },
		{ kind: 'force-restart', icon: 'player-stop', labelKey: 'ui.sidebar.manageForce' }
	];

	let signingOut = $state(false);
	let actionKind = $state<DaemonActionKind>('restart');
	let actionOpen = $state(false);
	let actionBusy = $state(false);
	let actionError = $state('');

	function askAction(kind: DaemonActionKind) {
		actionKind = kind;
		actionError = '';
		daemonDialog.open = false;
		actionOpen = true;
	}

	async function confirmAction() {
		actionBusy = true;
		actionError = '';
		try {
			await api(daemonActionPath(actionKind), { method: 'POST' });
			actionOpen = false;
			toast.success(
				actionKind === 'shutdown'
					? $t('ui.sidebar.shutdownStarted')
					: actionKind === 'force-restart'
						? $t('ui.sidebar.forceRestarted')
						: $t('ui.sidebar.restarted')
			);
		} catch (error) {
			actionError = error instanceof Error ? error.message : $t('ui.common.error');
		} finally {
			actionBusy = false;
		}
	}

	async function signOut() {
		signingOut = true;
		try {
			await api('/auth/logout', { method: 'POST' });
		} finally {
			signingOut = false;
			await goto('/login');
		}
	}
</script>

<CenteredModal
	bind:open={daemonDialog.open}
	size="compact"
	class="max-w-md"
	title={$t('ui.sidebar.manage')}
	description={$t('ui.sidebar.manageBody')}
>
	<div class="flex flex-col gap-4">
		<div class="flex items-start justify-center gap-6 px-2 py-1">
			{#each manageActions as action (action.kind)}
				<div class="flex w-[5.75rem] flex-col items-center gap-2">
					<Button
						variant="outline"
						size="icon"
						disabled={actionBusy}
						aria-label={$t(action.labelKey)}
						title={$t(action.labelKey)}
						class={cn(
							'rounded-full',
							action.off
								? 'size-16 text-destructive hover:border-destructive/40 hover:bg-destructive/10 hover:text-destructive'
								: 'size-14 text-muted-foreground hover:text-foreground'
						)}
						onclick={() => askAction(action.kind)}
					>
						<Icon name={action.icon} size={action.off ? 26 : 22} />
					</Button>
					<span class="text-center text-xs leading-snug text-muted-foreground"
						>{$t(action.labelKey)}</span
					>
				</div>
			{/each}
		</div>
		{#if session.loginRequired}
			<Button variant="ghost" class="w-full" onclick={signOut} disabled={signingOut}>
				<Icon name={signingOut ? 'loader' : 'logout'} size={14} spin={signingOut} />
				{signingOut ? $t('ui.sidebar.signingOut') : $t('ui.sidebar.signOut')}
			</Button>
		{/if}
	</div>
	{#snippet footer()}
		<div class="modal-footer">
			<Button variant="outline" disabled={actionBusy} onclick={() => (daemonDialog.open = false)}>
				<Icon name="x" size={14} />
				{$t('ui.common.cancel')}
			</Button>
		</div>
	{/snippet}
</CenteredModal>

<ConfirmDialog
	bind:open={actionOpen}
	title={actionKind === 'shutdown'
		? $t('ui.sidebar.shutdownTitle')
		: actionKind === 'force-restart'
			? $t('ui.sidebar.forceRestartTitle')
			: $t('ui.sidebar.restartTitle')}
	body={actionKind === 'shutdown'
		? $t('ui.sidebar.shutdownBody')
		: actionKind === 'force-restart'
			? $t('ui.sidebar.forceRestartBody')
			: $t('ui.sidebar.restartBody')}
	confirmLabel={actionKind === 'shutdown'
		? $t('ui.sidebar.shutdown')
		: actionKind === 'force-restart'
			? $t('ui.sidebar.forceRestart')
			: $t('ui.sidebar.restart')}
	tone={actionKind === 'restart' ? 'default' : 'destructive'}
	busy={actionBusy}
	error={actionError}
	icon={actionKind === 'shutdown'
		? 'power'
		: actionKind === 'force-restart'
			? 'player-stop'
			: 'refresh'}
	onconfirm={confirmAction}
/>
