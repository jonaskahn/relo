<script lang="ts">
	import { t } from 'svelte-i18n';
	import { consoleState } from '$lib/console-state.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';

	// CardLayoutToggle is the one control for the connection, account, key,
	// agent, and group grids: vertical packs four cards, horizontal packs three.
</script>

<Segmented
	class="max-sm:hidden"
	value={consoleState.settings.cardLayout}
	ariaLabel={$t('ui.pages.providersPage.list.cardLayout')}
	iconOnly
	options={[
		{
			value: 'vertical',
			label: $t('ui.pages.providersPage.list.cardLayoutVertical'),
			icon: 'layout-dashboard'
		},
		{
			value: 'horizontal',
			label: $t('ui.pages.providersPage.list.cardLayoutHorizontal'),
			icon: 'list-details'
		}
	]}
	onchange={(layout) => consoleState.setCardLayout(layout)}
/>
