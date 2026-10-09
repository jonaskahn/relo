<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import type { Account, Model, Provider, QuotaWindow, TemplateFormatOption } from '$lib/types';
	import { Tabs } from 'bits-ui';
	import TabStrip from '$lib/components/ui/tab-strip.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import { Tooltip } from 'bits-ui';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import ProviderBanner from './provider-banner.svelte';
	import ModelFilterMenu from './model-filter-menu.svelte';
	import CardLayoutToggle from './card-layout-toggle.svelte';
	import ModelList from './model-list.svelte';
	import AccountsTab from './accounts-tab.svelte';
	import SettingsTab from './settings-tab.svelte';
	import { supportsPaidFree } from '$lib/model-filters';
	import { EMPTY_MODEL_FILTERS, type ModelFilters } from '$lib/model-filters';
	import type { ModelLabelFilter } from '$lib/model-filters';
	import { providerStatus } from '$lib/provider-status';
	import { ageOf } from '$lib/format';

	interface Props {
		provider: Provider;
		formats: TemplateFormatOption[];
		windows?: QuotaWindow[];
		tab?: string;
		// version is the page's update counter: the pane reads its models again
		// whenever an update ran, because a price or a name can change without
		// the connection row itself changing.
		version?: number;
		// labelFilter is the one filter the page asked the model list to open
		// on, such as unpriced after the attention strip named the count.
		labelFilter?: ModelLabelFilter;
		// onback returns to the connection grid, which owns the selection.
		onback: () => void;
		onchanged: (updated: Provider) => void;
		ondeleted: () => void;
		onquotas?: () => void;
		// onaddaccount opens the add flow already inside this connection.
		onaddaccount?: () => void;
		ontab: (tab: string) => void;
	}

	let {
		provider,
		formats,
		windows = [],
		tab = 'models',
		version = 0,
		labelFilter = 'all',
		onback,
		onchanged,
		ondeleted,
		onquotas,
		onaddaccount,
		ontab
	}: Props = $props();

	// The models tab holds every model of one provider, which is what makes
	// the search and the chips instant.
	const pageSize = 1000;

	let models = $state.raw<Model[]>([]);
	let modelsLoading = $state(false);
	let modelsFailed = $state('');
	let accounts = $state.raw<Account[]>([]);
	let accountsLoading = $state(false);
	let accountsFailed = $state('');
	let deleteOpen = $state(false);
	let deleteFailed = $state('');
	let refreshing = $state(false);
	let quotaRevision = $state(0);

	const status = $derived(providerStatus(provider));
	const quotaStale = $derived(windows.some((window) => window.stale));
	// modelFilters lives here so search and the one filter menu can sit beside
	// Back on the models tab while the list below keeps filtering.
	let modelFilters = $state<ModelFilters>({ ...EMPTY_MODEL_FILTERS });
	// The page may already have asked for one filter, such as unpriced after
	// the attention strip named the count. That intent resets the list onto
	// exactly that filter, and every change the operator makes afterwards is
	// their own state; intent names the one already taken up.
	let intent = $state<ModelLabelFilter | ''>('');
	const shownFilters = $derived(
		intent === labelFilter ? modelFilters : { ...EMPTY_MODEL_FILTERS, label: labelFilter }
	);

	function setFilters(next: ModelFilters) {
		modelFilters = next;
		intent = labelFilter;
	}

	// The connection and the page's update counter together are the key of the
	// effect below, so either one sends it to the daemon for a fresh read.
	const loaded = $derived({ id: provider.id, revision: version });

	$effect(() => {
		const { id } = loaded;
		if (!id) return;
		void loadModels(id);
		void loadAccounts(id);
	});

	async function loadModels(id: string) {
		modelsLoading = true;
		modelsFailed = '';
		try {
			const collected: Model[] = [];
			for (let offset = 0; offset < 20; offset++) {
				const page = await api<{ items: Model[]; total: number }>(
					'/models?provider=' +
						encodeURIComponent(id) +
						'&limit=' +
						pageSize +
						'&offset=' +
						offset * pageSize
				);
				collected.push(...(page.items ?? []));
				if ((page.items ?? []).length < pageSize) break;
			}
			models = collected;
		} catch (error) {
			modelsFailed = error instanceof Error ? error.message : String(error);
		} finally {
			modelsLoading = false;
		}
	}

	async function loadAccounts(id: string) {
		accountsLoading = true;
		accountsFailed = '';
		try {
			const data = await api<{ items: Account[] }>('/accounts?provider=' + encodeURIComponent(id));
			accounts = data.items ?? [];
		} catch (error) {
			accountsFailed = error instanceof Error ? error.message : String(error);
		} finally {
			accountsLoading = false;
		}
	}

	// The counts and the status the card, the tab labels, the header and the
	// banner read come from the connection row, so an account or a model that
	// just changed has to move them without the operator reloading the page.
	async function refreshProvider(id: string) {
		try {
			onchanged(await api<Provider>('/connections/' + encodeURIComponent(id)));
		} catch {
			// The row the page holds is the best answer a failed read leaves,
			// and the pane's own error line reports what the operator asked for.
		}
	}

	function reloadModels() {
		void loadModels(provider.id);
		void refreshProvider(provider.id);
	}

	function reloadAccounts() {
		void loadAccounts(provider.id);
		reloadModels();
	}

	async function toggleModels(enabled: boolean, ids: string[]) {
		try {
			await api('/connections/' + encodeURIComponent(provider.id) + '/models/enabled', {
				method: 'POST',
				body: JSON.stringify({ model_ids: ids, enabled })
			});
		} catch (error) {
			modelsFailed = error instanceof Error ? error.message : String(error);
			throw error;
		}
	}

	async function removeProvider() {
		deleteOpen = false;
		deleteFailed = '';
		try {
			await api('/connections/' + encodeURIComponent(provider.id), { method: 'DELETE' });
			ondeleted();
		} catch (error) {
			deleteFailed = error instanceof Error ? error.message : String(error);
			deleteOpen = true;
		}
	}

	// A banner carries one action, and that action is the shortest way out of
	// the state the banner describes.
	function onBanner(name: string) {
		switch (name) {
			case 'paused':
				void api<Provider>('/connections/' + encodeURIComponent(provider.id), {
					method: 'PATCH',
					body: JSON.stringify({ enabled: true })
				}).then(onchanged);
				return;
			case 'needsSetup':
				ontab('settings');
				return;
			case 'noAccount':
			case 'signInAgain':
				onaddaccount?.();
				return;
			case 'accountsPaused':
				ontab('accounts');
				return;
			case 'modelListFailed':
				void refreshThisConnection();
				return;
			case 'noModelsOn':
				ontab('models');
				return;
		}
	}

	async function toggleEnabled(enabled: boolean) {
		const updated = await api<Provider>('/connections/' + encodeURIComponent(provider.id), {
			method: 'PATCH',
			body: JSON.stringify({ enabled })
		});
		onchanged(updated);
	}

	async function refreshThisConnection() {
		refreshing = true;
		try {
			const result = await api<{ listed: number }>(
				'/connections/' + encodeURIComponent(provider.id) + '/models/refresh',
				{ method: 'POST' }
			);
			toast.success(
				$t('ui.pages.providersPage.pane.refreshDone', { values: { count: result.listed } })
			);
			await reloadModels();
			quotaRevision += 1;
			onquotas?.();
		} catch (error) {
			toast.error(
				$t('ui.pages.providersPage.pane.refreshFailed', {
					values: { detail: error instanceof Error ? error.message : String(error) }
				})
			);
		} finally {
			refreshing = false;
		}
	}
</script>

<div class="flex flex-col">
	<header class="flex flex-col py-3">
		<div class="flex min-w-0 items-start gap-3">
			<ProviderLogo id={provider.template_id || provider.id} label={provider.label} />
			<div class="min-w-0">
				<h2 class="truncate text-base font-semibold">{provider.label}</h2>
				<div
					class="flex min-w-0 flex-wrap items-center gap-1.5 pt-0.5 text-xs text-muted-foreground"
				>
					<span>
						{$t(status.labelKey)}{#if provider.last_refresh_error}
							{' '}({$t('ui.pages.providersPage.pane.statsRefreshFailed', {
								values: { detail: provider.last_refresh_error }
							})}){:else if provider.last_refreshed_at_ms}
							{' '}({$t('ui.pages.providersPage.pane.statsRefreshed', {
								values: { age: ageOf(provider.last_refreshed_at_ms) }
							})}){/if}
					</span>
					{#if quotaStale}
						<Tooltip.Provider delayDuration={250}>
							<Tooltip.Root>
								<Tooltip.Trigger
									class="inline-flex cursor-pointer items-center justify-center rounded-control text-amber-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring dark:text-amber-400"
									aria-label={$t('ui.pages.providersPage.banner.actionOpenAccounts')}
									onclick={() => ontab('accounts')}
								>
									<Icon name="clock" size={14} />
								</Tooltip.Trigger>
								<Tooltip.Portal>
									<Tooltip.Content class="z-50 surface-pop px-2.5 py-1.5 text-xs">
										{$t('ui.pages.providersPage.accounts.quotaStale')}
									</Tooltip.Content>
								</Tooltip.Portal>
							</Tooltip.Root>
						</Tooltip.Provider>
					{/if}
				</div>
			</div>
		</div>
	</header>

	<ProviderBanner {provider} onaction={(name) => onBanner(name)} />

	<Tabs.Root value={tab} onValueChange={ontab} class="flex flex-col">
		<div class="provider-toolbar" data-tab={tab}>
			<Button variant="outline" onclick={onback} class="pt-back shrink-0">
				<Icon name="arrow-left" size={14} />
				{$t('ui.pages.providersPage.pane.back')}
			</Button>
			<div class="pt-tabs">
				<TabStrip
					tabs={[
						{
							value: 'models',
							label: $t('ui.pages.providersPage.pane.tabModels'),
							count: provider.counts.models,
							icon: 'cpu'
						},
						{
							value: 'accounts',
							label: $t('ui.pages.providersPage.pane.tabAccounts'),
							count: provider.counts.accounts,
							icon: 'user'
						},
						{
							value: 'settings',
							label: $t('ui.pages.providersPage.pane.tabSettings'),
							icon: 'settings'
						}
					]}
					value={tab}
					ariaLabel={$t('ui.pages.providersPage.pane.tabModels')}
				/>
			</div>
			{#key tab}
				{#if tab === 'models'}
					<div class="pt-context pt-context-search toolbar-swap">
						<div class="models-search relative min-w-0">
							<span
								class="pointer-events-none absolute inset-y-0 start-2 flex items-center text-muted-foreground"
							>
								<Icon name="search" size={14} />
							</span>
							<Input
								class="ps-7"
								placeholder={$t('ui.pages.providersPage.models.search')}
								aria-label={$t('ui.pages.providersPage.models.search')}
								value={shownFilters.search}
								oninput={(event) =>
									setFilters({ ...shownFilters, search: event.currentTarget.value })}
							/>
						</div>
					</div>
				{/if}
			{/key}
			<div class="pt-actions">
				{#if tab === 'models'}
					<ModelFilterMenu
						filters={shownFilters}
						onfilters={setFilters}
						{models}
						paidFree={supportsPaidFree(provider.template_id)}
					/>
				{/if}
				{#if tab === 'accounts' && accounts.length > 0}
					<CardLayoutToggle />
				{/if}
				<IconAction
					icon={provider.enabled ? 'player-pause' : 'player-play'}
					label={$t('ui.pages.providersPage.pane.enableToggle', {
						values: { label: provider.label }
					})}
					variant="outline"
					pressed={provider.enabled}
					onclick={() => void toggleEnabled(!provider.enabled)}
				/>
				<IconAction
					icon="refresh"
					label={$t('ui.pages.providersPage.pane.refreshModels')}
					variant="outline"
					disabled={refreshing}
					spin={refreshing}
					onclick={() => void refreshThisConnection()}
				/>
				<IconAction
					icon="trash"
					label={$t('ui.pages.providersPage.pane.deleteProvider')}
					variant="outline"
					tone="destructive"
					onclick={() => (deleteOpen = true)}
				/>
			</div>
		</div>

		<Tabs.Content value="models" class="tab-panel flex flex-col outline-none">
			<ModelList
				providerId={provider.id}
				{models}
				loading={modelsLoading}
				failed={modelsFailed}
				canAdd={provider.models_format === 'none'}
				paidFree={supportsPaidFree(provider.template_id)}
				filters={shownFilters}
				onfilters={setFilters}
				hideSearchFilters
				onreload={reloadModels}
				onenable={(ids) => toggleModels(true, ids)}
				ondisable={(ids) => toggleModels(false, ids)}
			/>
		</Tabs.Content>

		<Tabs.Content value="accounts" class="tab-panel flex flex-col outline-none">
			<AccountsTab
				{provider}
				{accounts}
				{quotaRevision}
				loading={accountsLoading}
				failed={accountsFailed}
				onreload={reloadAccounts}
				onremoved={ondeleted}
				onquotas={() => onquotas?.()}
			/>
		</Tabs.Content>

		<Tabs.Content value="settings" class="tab-panel flex flex-col outline-none">
			{#key provider}
				<SettingsTab {provider} allFormats={formats} {onchanged} />
			{/key}
		</Tabs.Content>
	</Tabs.Root>
</div>

<ConfirmDialog
	bind:open={deleteOpen}
	title={$t('ui.pages.providersPage.pane.deleteConfirmTitle', {
		values: { label: provider.label }
	})}
	body={$t('ui.pages.providersPage.pane.deleteConfirmBody')}
	confirmLabel={$t('ui.pages.providersPage.pane.deleteProvider')}
	tone="destructive"
	icon="trash"
	error={deleteFailed}
	onconfirm={removeProvider}
/>
