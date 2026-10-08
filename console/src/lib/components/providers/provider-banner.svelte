<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Provider } from '$lib/types';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { bannerAction, providerStatus, type ProviderStatusName } from '$lib/provider-status';

	interface Props {
		provider: Provider;
		onaction: (name: ProviderStatusName) => void;
	}

	let { provider, onaction }: Props = $props();

	const status = $derived(providerStatus(provider));
	const action = $derived(status.name === 'ready' ? '' : bannerAction(status.name));

	const actionIcon = $derived.by((): IconName => {
		switch (action) {
			case 'ui.pages.providersPage.banner.actionResume':
				return 'player-play';
			case 'ui.pages.providersPage.banner.actionOpenSettings':
				return 'settings';
			case 'ui.pages.providersPage.banner.actionAddAccount':
				return 'plus';
			case 'ui.pages.providersPage.banner.actionOpenAccounts':
				return 'user';
			case 'ui.pages.providersPage.banner.actionRefresh':
				return 'refresh';
			case 'ui.pages.providersPage.banner.actionShowModels':
				return 'list';
			default:
				return 'refresh';
		}
	});

	// The body names what is wrong, with the values only that status knows.
	const values = $derived.by(() => {
		switch (status.name) {
			case 'needsSetup':
				return { names: (provider.needs_setup ?? []).join(', ') };
			case 'signInAgain':
				return { count: provider.counts.reauth_accounts };
			case 'modelListFailed':
				return { error: provider.last_refresh_error ?? '' };
			default:
				return {};
		}
	});
</script>

{#if status.name !== 'ready'}
	<div
		class="flex flex-wrap items-center rounded-control gap-3 border-amber-500/40 bg-amber-500/5 py-2 px-2 mb-1 text-sm"
		role="status"
	>
		<Icon name="alert-triangle" size={15} class="shrink-0 text-amber-700 dark:text-amber-400" />
		<span class="min-w-0 flex-1 text-muted-foreground">
			{$t(status.bannerKey, { values })}
		</span>
		{#if action}
			<button
				type="button"
				aria-expanded="false"
				class="inline-flex min-h-control shrink-0 cursor-pointer items-center gap-1.5 rounded-control border border-warn/40 px-2.5 text-xs font-medium text-warn transition-colors hover:bg-warn/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
				onclick={() => onaction(status.name)}
			>
				<Icon name={actionIcon} size={14} />
				{$t(action)}
			</button>
		{/if}
	</div>
{/if}
