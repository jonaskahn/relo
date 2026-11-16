<script lang="ts">
	import { t } from 'svelte-i18n';

	import { browserStorage, markWhatsNewSeen } from '$lib/whats-new';
	import { notesText } from '$lib/update-notes';
	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';

	interface Props {
		open?: boolean;
		version?: string;
		notes: string;
	}

	let { open = $bindable(false), version, notes }: Props = $props();

	// Marked on open, not on close: the notes have been shown, and a page
	// that reloads before they are dismissed must not ask again.
	$effect(() => {
		if (open && version) markWhatsNewSeen(browserStorage(), version);
	});
</script>

<CenteredModal
	bind:open
	size="compact"
	title={$t('ui.whatsNewDialog.title', { values: { version: version ?? '' } })}
>
	<p class="whitespace-pre-line text-sm leading-6 text-muted-foreground">{notesText(notes)}</p>
	{#snippet footer()}
		<div class="modal-footer">
			<Button onclick={() => (open = false)}>{$t('ui.common.done')}</Button>
		</div>
	{/snippet}
</CenteredModal>
