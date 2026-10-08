<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import { keyPillValues, keyStatusPill } from '$lib/access-keys';
	import { formatDate, relativeTime } from '$lib/format';
	import type { AccessKey } from '$lib/types';

	// KeyDetailModal is one key read in full, and the two parts of it an
	// operator may edit. Rotation and revocation do not happen here: they
	// need a confirmation of their own, so they hand off and the page asks.

	interface Props {
		open?: boolean;
		accessKey: AccessKey | null;
		onsaved: () => void;
		onrotate: (key: AccessKey) => void;
		onrevoke: (key: AccessKey) => void;
		onclose: () => void;
	}

	let {
		open = $bindable(false),
		accessKey,
		onsaved,
		onrotate,
		onrevoke,
		onclose
	}: Props = $props();

	let view = $state<'read' | 'edit'>('read');
	let editName = $state('');
	let editExpiry = $state('');
	let saving = $state(false);
	let discardOpen = $state(false);
	let discardCloses = $state(false);

	const pill = $derived(accessKey ? keyStatusPill(accessKey) : null);
	const dirty = $derived(
		accessKey !== null && (editName !== accessKey.name || editExpiry !== expiryOf(accessKey))
	);

	function expiryOf(key: AccessKey): string {
		return key.expires_at_ms ? new Date(key.expires_at_ms).toISOString().slice(0, 10) : '';
	}

	function startEdit() {
		if (!accessKey) return;
		editName = accessKey.name;
		editExpiry = expiryOf(accessKey);
		view = 'edit';
	}

	// A close is a question while the edit holds changes: the surface stays,
	// and the discard dialog decides.
	function requestClose() {
		if (view === 'edit' && dirty) {
			discardCloses = true;
			discardOpen = true;
			open = true;
			return;
		}
		finish();
	}

	function backToRead() {
		if (dirty) {
			discardCloses = false;
			discardOpen = true;
			return;
		}
		view = 'read';
	}

	function discardEdits() {
		discardOpen = false;
		view = 'read';
		if (discardCloses) {
			discardCloses = false;
			finish();
		}
	}

	function finish() {
		open = false;
		view = 'read';
		onclose();
	}

	async function save() {
		if (!accessKey) return;
		saving = true;
		try {
			const expires = editExpiry ? new Date(editExpiry + 'T23:59:59').getTime() : 0;
			await api('/clients/keys/' + encodeURIComponent(accessKey.id), {
				method: 'PATCH',
				body: JSON.stringify({ name: editName.trim(), expires_at_ms: expires })
			});
			view = 'read';
			toast.success($t('ui.pages.keysPage.updated'));
			onsaved();
		} catch (error) {
			toast.error(error instanceof Error ? error.message : $t('ui.common.error'));
		} finally {
			saving = false;
		}
	}
</script>

<CenteredModal
	bind:open
	onOpenChange={(value) => {
		if (!value) requestClose();
	}}
	size="standard"
	title={accessKey?.name ?? ''}
	description={view === 'edit' ? $t('ui.pages.keysPage.editDescription') : undefined}
>
	{#snippet footer()}
		<div class="modal-footer">
			{#if accessKey?.owner}
				<!-- An integration owns this key: rotating or revoking it here would
				     leave the agent Relo configured pointing at a secret that no
				     longer works, so the integration page is where it changes. -->
				<Button variant="ghost" onclick={requestClose}>
					<Icon name="x" size={14} />
					{$t('ui.common.close')}
				</Button>
			{:else if view === 'edit'}
				<Button variant="ghost" onclick={backToRead}>
					<Icon name="arrow-left" size={14} />
					{$t('ui.common.back')}
				</Button>
				<Button onclick={save} disabled={saving || !dirty || !editName.trim()}>
					<Icon name={saving ? 'loader' : 'check'} size={14} spin={saving} />
					{$t('ui.common.save')}
				</Button>
			{:else}
				<!-- The destructive action leads the footer group, apart from the primary. -->
				<Button
					variant="destructive"
					onclick={() => accessKey && onrevoke(accessKey)}
					disabled={accessKey?.status === 'revoked'}
				>
					<Icon name="trash" size={14} />
					{$t('ui.pages.keysPage.revoke')}
				</Button>
				<Button
					variant="outline"
					onclick={() => accessKey && onrotate(accessKey)}
					disabled={accessKey?.status === 'revoked'}
				>
					<Icon name="refresh" size={14} />
					{$t('ui.pages.keysPage.rotate')}
				</Button>
				<Button onclick={startEdit}>
					<Icon name="pencil" size={14} />
					{$t('ui.common.edit')}
				</Button>
			{/if}
		</div>
	{/snippet}
	{#if accessKey && pill}
		{#if view === 'read'}
			<div class="flex flex-col gap-4">
				<div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
					<div>
						<p class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.columnKind')}</p>
						<p class="mt-0.5 text-sm font-medium">{$t('ui.status.kind.' + accessKey.kind)}</p>
					</div>
					<div>
						<p class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.columnStatus')}</p>
						<div class="mt-0.5">
							<StatusBadge
								kind={pill.kind}
								label={$t(pill.labelKey, { values: keyPillValues(pill) })}
							/>
						</div>
					</div>
					<div>
						<p class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.columnHint')}</p>
						<p class="mono-data mt-0.5 text-sm">{accessKey.token_hint}</p>
					</div>
					<div>
						<p class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.columnCreated')}</p>
						<p class="mt-0.5 text-sm">{formatDate(accessKey.created_at_ms)}</p>
					</div>
					<div>
						<p class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.columnLastUsed')}</p>
						<p class="mt-0.5 text-sm">{relativeTime(accessKey.last_used_at_ms)}</p>
					</div>
					<div>
						<p class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.columnExpires')}</p>
						<p class="mt-0.5 text-sm">
							{accessKey.expires_at_ms
								? formatDate(accessKey.expires_at_ms)
								: $t('ui.common.never')}
						</p>
					</div>
					<div>
						<p class="text-xs text-muted-foreground">{$t('ui.pages.keysPage.generation')}</p>
						<p class="mono-data mt-0.5 text-sm">{accessKey.generation}</p>
					</div>
				</div>
				{#if accessKey.owner}
					<p
						class="rounded-panel border border-line bg-sunken px-3 py-2 text-xs text-muted-foreground"
					>
						{$t('ui.pages.keysPage.ownedHint')}
					</p>
				{/if}
			</div>
		{:else}
			<div class="field-grid">
				<Field id="edit-name" label={$t('ui.pages.keysPage.nameLabel')}>
					<Input id="edit-name" bind:value={editName} />
				</Field>
				<Field id="edit-expiry" label={$t('ui.pages.keysPage.expiryLabel')}>
					<Input id="edit-expiry" type="date" bind:value={editExpiry} />
				</Field>
			</div>
		{/if}
	{/if}
</CenteredModal>

<ConfirmDialog
	bind:open={discardOpen}
	title={$t('ui.common.discardTitle')}
	body={$t('ui.common.discardBody')}
	confirmLabel={$t('ui.common.discardConfirm')}
	tone="destructive"
	icon="alert-triangle"
	onconfirm={discardEdits}
/>
