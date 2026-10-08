<script lang="ts">
	import { t } from 'svelte-i18n';
	import CenteredModal from './centered-modal.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Icon, { type IconName } from './icon.svelte';

	// ConfirmDialog is the one confirmation surface: a compact sheet with the
	// risky action on the right and, on a phone, the primary action on top of
	// the stacked footer.
	interface Props {
		open?: boolean;
		title: string;
		body: string;
		confirmLabel: string;
		cancelLabel?: string;
		tone?: 'default' | 'destructive';
		busy?: boolean;
		error?: string;
		icon?: IconName;
		onconfirm: () => void | Promise<void>;
	}

	let {
		open = $bindable(false),
		title,
		body,
		confirmLabel,
		// cancelLabel names what leaving the dialog does when Cancel is not the
		// whole word for it, the way discarding an unsaved choice is.
		cancelLabel = '',
		tone = 'default',
		busy = false,
		error = '',
		icon = 'alert-triangle',
		onconfirm
	}: Props = $props();
</script>

<CenteredModal bind:open size="compact" {title} dismissible={!busy}>
	{#snippet footer()}
		<div class="modal-footer">
			<Button variant="outline" disabled={busy} onclick={() => (open = false)}>
				<Icon name="x" size={14} />
				{cancelLabel || $t('ui.common.cancel')}
			</Button>
			<Button
				variant={tone === 'destructive' ? 'destructive' : 'default'}
				class={tone === 'destructive'
					? 'border-danger bg-danger text-[var(--destructive-foreground)] hover:bg-danger/90'
					: ''}
				disabled={busy}
				onclick={onconfirm}
			>
				<Icon name={busy ? 'loader' : icon} size={14} spin={busy} />
				{confirmLabel}
			</Button>
		</div>
	{/snippet}
	<div class="flex items-start gap-2.5">
		<Icon
			name={icon}
			size={18}
			class={tone === 'destructive'
				? 'mt-0.5 shrink-0 text-danger'
				: 'mt-0.5 shrink-0 text-accent-strong'}
		/>
		<p class="text-sm text-muted-foreground">{body}</p>
	</div>
	{#if error}
		<p class="mt-3 text-sm text-danger" role="alert">{error}</p>
	{/if}
</CenteredModal>
