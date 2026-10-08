<script lang="ts">
	import { t } from 'svelte-i18n';
	import { AlertDialog } from 'bits-ui';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	// The confirmation for an irreversible action on one agent. It is an
	// AlertDialog, not a second modal: the detail modal closes first, so a
	// destructive step is never stacked on the surface that asked for it.
	interface Props {
		open?: boolean;
		title: string;
		body: string;
		confirmLabel: string;
		tone?: 'default' | 'destructive';
		busy?: boolean;
		error?: string;
		icon?: 'alert-triangle' | 'trash';
		onconfirm: () => void | Promise<void>;
		onClose?: () => void;
	}

	let {
		open = $bindable(false),
		title,
		body,
		confirmLabel,
		tone = 'default',
		busy = false,
		// error is a refused action: it stays in the dialog with its reason rather
		// than becoming a toast behind a scrim.
		error = '',
		icon = 'alert-triangle',
		onconfirm,
		// onClose runs when the operator dismisses the confirmation, so the page
		// stops reading a question that is no longer asked.
		onClose = () => {}
	}: Props = $props();
</script>

<AlertDialog.Root
	bind:open
	onOpenChange={(value) => {
		if (!value && !busy) onClose();
	}}
>
	<AlertDialog.Portal>
		<AlertDialog.Overlay class="fixed inset-0 z-50 bg-[var(--scrim)]" />
		<AlertDialog.Content
			class="fixed left-1/2 top-1/2 z-50 w-[calc(100%-1.5rem)] max-w-lg -translate-x-1/2 -translate-y-1/2 rounded-card bg-surface p-5 text-ink shadow-modal outline-none max-sm:bottom-0 max-sm:left-0 max-sm:top-auto max-sm:w-full max-sm:max-w-none max-sm:translate-x-0 max-sm:translate-y-0 max-sm:rounded-b-none max-sm:rounded-t-card"
		>
			<div class="flex items-start gap-2.5">
				<Icon
					name={icon}
					size={18}
					class={tone === 'destructive'
						? 'mt-0.5 shrink-0 text-danger'
						: 'mt-0.5 shrink-0 text-accent-strong'}
				/>
				<div class="min-w-0">
					<AlertDialog.Title class="text-[1.0625rem] font-semibold leading-snug"
						>{title}</AlertDialog.Title
					>
					<AlertDialog.Description class="mt-1 text-sm text-muted-foreground"
						>{body}</AlertDialog.Description
					>
				</div>
			</div>
			{#if error}
				<p class="mt-3 flex items-start gap-1.5 text-sm text-danger" role="alert">
					<Icon name="circle-x" size={14} class="mt-0.5 shrink-0" />
					{error}
				</p>
			{/if}
			<div class="modal-footer mt-5">
				<Button variant="outline" disabled={busy} onclick={() => (open = false)}>
					<Icon name="x" size={14} />
					{$t('ui.common.cancel')}
				</Button>
				<Button
					variant={tone === 'destructive' ? 'destructive' : 'default'}
					disabled={busy}
					onclick={onconfirm}
				>
					<Icon name={busy ? 'loader' : icon} size={14} spin={busy} />
					{confirmLabel}
				</Button>
			</div>
		</AlertDialog.Content>
	</AlertDialog.Portal>
</AlertDialog.Root>
