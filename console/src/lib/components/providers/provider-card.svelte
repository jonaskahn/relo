<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Provider } from '$lib/types';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { Switch } from 'bits-ui';
	import { providerStatus, type ProviderStatusName } from '$lib/provider-status';

	interface Props {
		provider: Provider;
		selected?: boolean;
		onselect: (id: string) => void;
		ontoggle?: (provider: Provider, enabled: boolean) => void;
		onrowkey?: (event: KeyboardEvent) => void;
	}

	let { provider, selected = false, onselect, ontoggle, onrowkey }: Props = $props();

	const status = $derived(providerStatus(provider));

	// Status is never carried by color alone: every word has an icon beside it.
	const icons: Record<ProviderStatusName, IconName> = {
		ready: 'circle-check',
		paused: 'player-pause',
		needsSetup: 'alert-triangle',
		noAccount: 'user',
		signInAgain: 'lock',
		accountsPaused: 'player-pause',
		modelListFailed: 'circle-alert',
		noModelsOn: 'ban'
	};

	// The line an operator scans for: how much of this connection is actually
	// switched on, and how many of its accounts can carry a request.
	const modelsOn = $derived(
		$t('ui.pages.providersPage.list.modelsOn', {
			values: { on: provider.counts.enabled_models, total: provider.counts.models }
		})
	);
	const accountsOn = $derived(
		$t('ui.pages.providersPage.list.accountsOn', {
			values: { active: provider.counts.active_accounts, total: provider.counts.accounts }
		})
	);
</script>

<div
	class="flex items-center gap-2 rounded-lg border px-2 py-2 transition-[border-color,background-color] duration-150 {selected
		? 'border-accent bg-accent-soft'
		: 'border-transparent hover:border-border hover:bg-surface'}"
>
	<button
		type="button"
		data-provider-row={provider.id}
		aria-current={selected ? 'true' : undefined}
		class="flex min-w-0 flex-1 flex-col gap-1 rounded-md px-1.5 py-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
		onclick={() => onselect(provider.id)}
		onkeydown={onrowkey}
	>
		<span class="flex items-center gap-2.5">
			<span class={provider.enabled ? '' : 'opacity-50'}>
				<ProviderLogo id={provider.template_id || provider.id} label={provider.label} size="sm" />
			</span>
			<span class="min-w-0 flex-1">
				<span class="block truncate text-sm font-medium leading-5">{provider.label}</span>
				<span class="block truncate font-mono text-[0.7rem] text-muted-foreground"
					>{provider.id}</span
				>
			</span>
		</span>
		<span
			class="flex items-center gap-1.5 text-[0.7rem] {status.tone === 'warn'
				? 'text-amber-700 dark:text-amber-400'
				: 'text-muted-foreground'}"
		>
			<Icon name={icons[status.name]} size={13} />
			<span>{$t(status.labelKey)}</span>
		</span>
		<span
			class="flex flex-wrap items-center gap-x-2 gap-y-1 font-mono text-[0.7rem] text-muted-foreground"
		>
			<span>{modelsOn}</span>
			<span aria-hidden="true">·</span>
			<span>{accountsOn}</span>
			{#if provider.counts.unpriced_models > 0}
				<span class="badge border border-border font-sans">
					<Icon name="tag" size={11} />
					{$t('ui.pages.providersPage.list.unpriced', {
						values: { count: provider.counts.unpriced_models }
					})}
				</span>
			{/if}
		</span>
	</button>
	{#if ontoggle}
		<Switch.Root
			checked={provider.enabled}
			onCheckedChange={(value: boolean) => ontoggle(provider, value)}
			aria-label={$t('ui.pages.providersPage.list.toggleLabel', {
				values: { name: provider.label }
			})}
		/>
	{/if}
</div>
