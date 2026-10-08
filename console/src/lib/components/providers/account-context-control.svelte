<script lang="ts">
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import { Popover } from 'bits-ui';
	import PopoverPanel from '$lib/components/ui/popover-panel.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { CONTEXT_PRESETS, contextLabel, contextTargets } from '$lib/context-window';
	import type { Account, AccountContext } from '$lib/types';

	// AccountContextControl sizes every model of one account at once, which is
	// how an operator gives two accounts of the same connection different
	// context windows. The list is a command rather than a readout: choosing a
	// size asks for a confirmation, and only Save writes it.
	interface Props {
		account: Account;
		disabled?: boolean;
		onnotify: (message: string) => void;
	}

	let { account, disabled = false, onnotify }: Props = $props();

	type ContextWrite = { applied: string[]; skipped: { model_id: string; reason: string }[] };

	// applied is the size the last save wrote, which the list marks and a
	// discarded choice falls back to.
	let applied = $state('default');
	let choice = $state('default');
	let choosing = $state(false);
	let confirming = $state(false);
	let busy = $state(false);

	// The size a confirmation is asking about is the one just picked; one
	// closed without saving falls back to the applied size, which covers
	// Discard, Escape and a click outside the sheet alike.
	const pending = $derived(confirming ? choice : applied);
	const value = $derived(pending === 'default' ? null : Number(pending));

	function pick(next: string) {
		choice = next;
		choosing = false;
		confirming = true;
	}

	async function save() {
		if (busy) return;
		busy = true;
		try {
			const contexts = await api<AccountContext>(
				'/accounts/' + encodeURIComponent(account.id) + '/models/context'
			);
			const targets = contextTargets(contexts.models ?? []);
			const result = await api<ContextWrite>(
				'/accounts/' + encodeURIComponent(account.id) + '/models/context',
				{ method: 'POST', body: JSON.stringify({ model_ids: targets, context_window: value }) }
			);
			applied = choice;
			confirming = false;
			onnotify(
				result.skipped.length > 0
					? $t('ui.pages.providersPage.accounts.contextAppliedSkipped', {
							values: { applied: result.applied.length, skipped: result.skipped.length }
						})
					: $t('ui.pages.providersPage.accounts.contextApplied', {
							values: { count: result.applied.length }
						})
			);
		} catch (error) {
			onnotify(
				$t('ui.pages.providersPage.accounts.contextFailed', {
					values: { detail: error instanceof Error ? error.message : String(error) }
				})
			);
		} finally {
			busy = false;
		}
	}
</script>

<Popover.Root bind:open={choosing}>
	<Popover.Trigger>
		{#snippet child({ props })}
			<Button
				variant="ghost"
				size="icon"
				{...props}
				{disabled}
				title={$t('ui.pages.providersPage.accounts.contextOverrideTitle')}
				aria-label={$t('ui.pages.providersPage.accounts.contextOverrideTitle')}
			>
				<Icon name="pencil" size={15} />
			</Button>
		{/snippet}
	</Popover.Trigger>
	<!-- A card row scrolls nothing, so the list opens beside the button and
	     portals out of the grid rather than being clipped by it. -->
	<Popover.Portal>
		<PopoverPanel class="z-50 w-60 surface-pop p-3" align="end" sideOffset={6}>
			<p class="text-xs font-semibold">
				{$t('ui.pages.providersPage.accounts.contextOverrideTitle')}
			</p>
			<p class="mt-0.5 text-xs text-muted-foreground">
				{$t('ui.pages.providersPage.accounts.contextOverrideHint')}
			</p>
			<div class="mt-2 flex flex-col">
				<button
					type="button"
					data-plain
					aria-pressed={applied === 'default'}
					class="flex min-h-control items-center rounded-md px-2 py-1.5 text-left text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {applied ===
					'default'
						? 'bg-accent-soft text-foreground'
						: ''}"
					onclick={() => pick('default')}
				>
					{$t('ui.pages.providersPage.accounts.contextDefault')}
				</button>
				{#each CONTEXT_PRESETS as preset (preset)}
					<button
						type="button"
						data-plain
						aria-pressed={applied === String(preset)}
						class="flex min-h-control items-center rounded-md px-2 py-1.5 font-mono text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {applied ===
						String(preset)
							? 'bg-accent-soft text-foreground'
							: ''}"
						onclick={() => pick(String(preset))}
					>
						{contextLabel(preset)}
					</button>
				{/each}
			</div>
		</PopoverPanel>
	</Popover.Portal>
</Popover.Root>

<ConfirmDialog
	bind:open={confirming}
	title={$t('ui.pages.providersPage.accounts.contextConfirmTitle', {
		values: { label: account.label }
	})}
	body={value === null
		? $t('ui.pages.providersPage.accounts.contextConfirmClear')
		: $t('ui.pages.providersPage.accounts.contextConfirmSet', {
				values: { value: contextLabel(value) }
			})}
	confirmLabel={$t('ui.pages.providersPage.accounts.contextSave')}
	cancelLabel={$t('ui.pages.providersPage.accounts.contextDiscard')}
	icon="pencil"
	{busy}
	onconfirm={save}
/>
