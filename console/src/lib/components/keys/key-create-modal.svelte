<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import { CLIENT_OPTIONS } from '$lib/client-setup';
	import type { IssuedAccessKey } from '$lib/types';

	// KeyCreateModal mints a client key. The client card decides the kind, so
	// the form stays one choice away from the key the operator wants. The key
	// itself is never shown here: the flow hands it to the reveal surface the
	// moment it exists.

	interface Props {
		open?: boolean;
		oncreated: (issued: IssuedAccessKey, clientId: string) => void;
	}

	let { open = $bindable(false), oncreated }: Props = $props();

	let name = $state('');
	let clientId = $state('codex');
	let otherClient = $state('');
	let expiry = $state<'never' | '30' | '90' | 'custom'>('never');
	let customDate = $state('');
	let creating = $state(false);

	const selectedClient = $derived(
		CLIENT_OPTIONS.find((option) => option.id === clientId) ?? CLIENT_OPTIONS[0]
	);
	const canSubmit = $derived(
		name.trim() !== '' && !(clientId === 'other' && otherClient.trim() === '')
	);

	// The form starts empty every time it opens, so a discarded draft never
	// greets the next operator.
	function reset() {
		name = '';
		clientId = 'codex';
		otherClient = '';
		expiry = 'never';
		customDate = '';
	}

	async function submit() {
		creating = true;
		try {
			const expires =
				expiry === 'never'
					? 0
					: expiry === 'custom' && customDate
						? new Date(customDate + 'T23:59:59').getTime()
						: Date.now() + (expiry === '30' ? 30 : 90) * 86_400_000;
			const client =
				selectedClient.kind === 'shared'
					? ''
					: clientId === 'other'
						? otherClient.trim()
						: clientId;
			const issued = await api<IssuedAccessKey>('/clients/keys', {
				method: 'POST',
				body: JSON.stringify({
					name: name.trim(),
					kind: selectedClient.kind,
					client,
					expires_at_ms: expires
				})
			});
			open = false;
			toast.success($t('ui.pages.keysPage.created'));
			oncreated(issued, clientId);
		} catch (error) {
			toast.error(error instanceof Error ? error.message : $t('ui.common.error'));
		} finally {
			creating = false;
		}
	}
</script>

<CenteredModal
	bind:open
	onOpenChange={(value) => {
		if (!value) reset();
	}}
	size="standard"
	title={$t('ui.pages.keysPage.createTitle')}
	description={$t('ui.pages.keysPage.createDescription')}
>
	{#snippet footer()}
		<div class="modal-footer">
			<Button variant="ghost" onclick={() => (open = false)}>
				<Icon name="x" size={14} />
				{$t('ui.common.cancel')}
			</Button>
			<Button onclick={submit} disabled={creating || !canSubmit}>
				<Icon name={creating ? 'loader' : 'plus'} size={14} spin={creating} />
				{$t('ui.pages.keysPage.createSubmit')}
			</Button>
		</div>
	{/snippet}
	<div class="flex flex-col gap-4">
		<Field
			id="key-name"
			label={$t('ui.pages.keysPage.nameLabel')}
			hint={$t('ui.pages.keysPage.namePlaceholder')}
		>
			<Input
				id="key-name"
				bind:value={name}
				placeholder={$t('ui.pages.keysPage.namePlaceholder')}
			/>
		</Field>
		<div class="flex flex-col gap-2">
			<span class="field-label">{$t('ui.pages.keysPage.usedBy')}</span>
			<div
				class="grid grid-cols-2 gap-2 lg:grid-cols-5"
				role="radiogroup"
				aria-label={$t('ui.pages.keysPage.usedBy')}
			>
				{#each CLIENT_OPTIONS as option (option.id)}
					{@const chosen = clientId === option.id}
					<button
						type="button"
						data-plain
						role="radio"
						aria-checked={chosen}
						class="flex min-w-0 items-center gap-2.5 rounded-panel border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {chosen
							? 'border-accent bg-accent-soft shadow-[0_0_0_3px_var(--accent-100)]'
							: 'border-line hover:border-line-strong'}"
						onclick={() => (clientId = option.id)}
					>
						<span
							class="flex size-[34px] shrink-0 items-center justify-center overflow-hidden rounded-control {chosen
								? 'bg-accent text-on-accent'
								: 'bg-sunken text-muted-foreground'}"
						>
							{#if option.logoId}
								<ProviderLogo id={option.logoId} label={option.label} size="sm" />
							{:else}
								<span class="text-xs font-semibold" aria-hidden="true">
									{option.label.slice(0, 2).toUpperCase()}
								</span>
							{/if}
						</span>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm font-semibold">{option.label}</span>
							<span class="mono-data block truncate text-xs text-muted-foreground"
								>{option.protocol}</span
							>
						</span>
					</button>
				{/each}
			</div>
			{#if selectedClient.kind === 'shared'}
				<p class="field-hint">{$t('ui.pages.keysPage.kindShared')}</p>
			{:else if clientId === 'other'}
				<Field
					id="key-client"
					label={$t('ui.pages.keysPage.clientLabel')}
					hint={$t('ui.pages.keysPage.clientHint')}
				>
					<Input
						id="key-client"
						bind:value={otherClient}
						placeholder={$t('ui.pages.keysPage.clientPlaceholder')}
					/>
				</Field>
			{/if}
		</div>
		<div class="flex flex-col gap-2">
			<span class="field-label">{$t('ui.pages.keysPage.expiryLabel')}</span>
			<Segmented
				bind:value={expiry}
				options={[
					{ value: 'never', label: $t('ui.pages.keysPage.expiryNever') },
					{ value: '30', label: $t('ui.pages.keysPage.expiry30') },
					{ value: '90', label: $t('ui.pages.keysPage.expiry90') },
					{ value: 'custom', label: $t('ui.pages.keysPage.expiryCustom') }
				]}
				ariaLabel={$t('ui.pages.keysPage.expiryLabel')}
			/>
			{#if expiry === 'custom'}
				<Input
					type="date"
					bind:value={customDate}
					aria-label={$t('ui.pages.keysPage.expiryLabel')}
				/>
			{/if}
		</div>
	</div>
</CenteredModal>
