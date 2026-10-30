<script lang="ts">
	import { t } from 'svelte-i18n';

	import IconAction from '$lib/components/ui/icon-action.svelte';

	// AllowOverridesButton is the one control for a connection's manual
	// capability values. It names the connection because the All models list can
	// show several at once, and a row action there changes the shared setting.
	interface Props {
		label: string;
		allowed: boolean;
		busy?: boolean;
		ontoggle: () => void;
	}

	let { label, allowed, busy = false, ontoggle }: Props = $props();

	const text = $derived(
		allowed
			? $t('ui.pages.providersPage.override.allowFor', { values: { connection: label } })
			: $t('ui.pages.providersPage.override.blockedFor', { values: { connection: label } })
	);
</script>

<IconAction
	icon="toggle-left"
	label={text}
	variant="outline"
	pressed={allowed}
	tone={allowed ? 'accent' : 'default'}
	disabled={busy}
	onclick={ontoggle}
/>
