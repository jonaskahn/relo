<script lang="ts">
	import { t } from 'svelte-i18n';
	import Icon from '$lib/components/ui/icon.svelte';
	import { hasFreeAvailability } from '$lib/provider-free';
import type { Provider } from '$lib/types';

	interface Props {
		provider: Pick<Provider, 'kind' | 'template_id' | 'auth'> & {
			counts?: Pick<Provider['counts'], 'free_models'>;
			free_models?: number;
		};
		class?: string;
	}

	let { provider, class: extra = '' }: Props = $props();
	const shown = $derived(hasFreeAvailability(provider));
</script>

{#if shown}
	<span
		data-free-available-chip
		class="badge shrink-0 bg-accent-soft text-accent-ink {extra}"
		aria-label={$t('ui.pages.providersPage.list.freeAvailable')}
		role="img"
	>
		<Icon name="sparkles" size={11} />
		<span class="ms-1">{$t('ui.pages.providersPage.list.freeAvailable')}</span>
	</span>
{/if}
