<script lang="ts">
	import { afterNavigate, replaceState } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import {
		connectionsViewQuery,
		linkedProviderId,
		readConnectionsView,
		type ConnectionsViewName
	} from '$lib/connections-view';
	import type {
		Account,
		CatalogRefreshResult,
		Model,
		ModelsDevState,
		Provider,
		ProviderTemplate,
		ProviderTemplatesResponse,
		QuotaWindow
	} from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import ProviderPane from '$lib/components/providers/provider-pane.svelte';
	import ConnectionsOverview from '$lib/components/providers/connections-overview.svelte';
	import ModelList from '$lib/components/providers/model-list.svelte';
	import AddProviderStepper from '$lib/components/providers/add/add-provider-stepper.svelte';
	import { API_FORMATS } from '$lib/catalog';
	import {
		EMPTY_MODEL_FILTERS,
		type ModelFilters,
		type ModelLabelFilter
	} from '$lib/model-filters';
	import { offeredTemplates } from '$lib/provider-sections';
	import {
		connectionCards,
		groupAccountsByConnection,
		groupQuotaByConnection
	} from '$lib/provider-quota';
	import { coalesceReload } from '$lib/refresh';
	import { subscribe } from '$lib/sse';

	let providers = $state<Provider[]>([]);
	let templates = $state<ProviderTemplate[]>([]);
	let modelsdev = $state<ModelsDevState | null>(null);
	let loading = $state(true);
	let failed = $state('');
	let updating = $state(false);

	// The selection lives in the URL, so a reload lands where the operator
	// left and a link to one provider is a link to its models. paneOpen is the
	// one part of that state the wide layout does not need: below it the page
	// shows the list or the pane, never both.
	let selectedId = $state('');
	let view = $state<ConnectionsViewName>('provider');
	let tab = $state('models');
	let paneOpen = $state(false);
	let addOpen = $state(false);
	// modelLabel carries the one filter an attention chip or the unpriced tag
	// asked the pane to open on; every other entry resets it.
	let modelLabel = $state<ModelLabelFilter>('all');
	// The all-models list filters in place: it is the page's own list rather
	// than one connection's, so its filter state lives here beside it.
	let allFilters = $state<ModelFilters>({ ...EMPTY_MODEL_FILTERS });

	// All models reads the server in pages, because a machine with every
	// models.dev provider added holds more rows than a page should.
	let allModels = $state<Model[]>([]);
	let allTotal = $state(0);
	let allLoading = $state(false);
	let allFailed = $state('');
	const allPageSize = 200;

	// The overview cards close with the quota their accounts reported, so the
	// page reads the accounts and the stored windows once and hands each card
	// its own slice. A read that fails leaves the cards without a footer rather
	// than failing the page, which is about connections.
	let accounts = $state<Account[]>([]);
	let quotaWindows = $state<QuotaWindow[]>([]);
	let unsubscribeQuota: (() => void) | undefined;
	// Every live response can announce a fresh reading, so the cards reload
	// once per burst rather than once per announcement.
	const reloadQuota = coalesceReload(() => loadQuota());

	const accountsByConnection = $derived(groupAccountsByConnection(accounts));
	const quotaByConnection = $derived(groupQuotaByConnection(quotaWindows));
	const cards = $derived(connectionCards(providers, accountsByConnection, quotaByConnection));

	async function loadQuota() {
		try {
			const [accountList, windows] = await Promise.all([
				api<{ items: Account[] }>('/accounts'),
				api<{ items: QuotaWindow[] }>('/activity/quota')
			]);
			accounts = accountList.items ?? [];
			quotaWindows = windows.items ?? [];
		} catch {
			// The quota is a report about an account, so a read that fails leaves
			// the accounts themselves untouched.
		}
	}

	const selected = $derived(providers.find((provider) => provider.id === selectedId) ?? null);
	const providerLabels = $derived(
		Object.fromEntries(providers.map((provider) => [provider.id, provider.label]))
	);

	onMount(() => {
		readURL();
		void load();
		void loadQuota();
		// A quota reading arrives from a probe or a live response, so the cards
		// refresh the moment the daemon has one worth showing.
		unsubscribeQuota = subscribe('quota', () => reloadQuota.schedule());
		// The header reports how old the saved models.dev copy is, so the page
		// reads the template list and the fetch state as soon as it opens.
		void readCatalog();
	});

	onDestroy(() => {
		unsubscribeQuota?.();
		reloadQuota.cancel();
	});

	// A quick add reaches this page by navigating to it, so a URL that arrives
	// while the page is already open has to be read again: the state it carries
	// is the state the operator asked for.
	afterNavigate(() => {
		readURL();
		// The add flow lists every provider Relo knows, so a quick add that
		// arrives before the page has the catalog waits for the read it started
		// rather than asking the daemon twice.
		if (addOpen && templates.length === 0) void readCatalog();
		// A link may name a connection this page does not hold. The list has to
		// be loaded before that is known: an empty list on arrival is the
		// read still in flight, and dropping the id then opens the first
		// connection instead of the one the link named.
		if (view === 'provider') {
			selectedId = linkedProviderId(
				selectedId,
				providers.map((provider) => provider.id),
				!loading
			);
		}
	});

	function readURL() {
		if (typeof window === 'undefined') return;
		const state = readConnectionsView(window.location.search);
		view = state.view;
		tab = state.tab;
		// A URL that names no connection leaves the selection to the page: the
		// list an operator returns to is no reason to forget which connection
		// the pane next to it holds.
		if (state.selectedId !== '') selectedId = state.selectedId;
		paneOpen = state.paneOpen;
		addOpen = state.addOpen;
	}

	// replaceState keeps the URL honest without adding history entries for
	// every card an operator clicks past.
	function writeURL() {
		if (typeof window === 'undefined') return;
		const query = connectionsViewQuery({ view, tab, selectedId, paneOpen, addOpen });
		replaceState(query === '' ? '/connections' : '/connections?' + query, {});
	}

	// Closing the flow takes the flag that asked for it back out of the URL, so
	// returning to the same page later does not open it again on its own.
	function closeAddFlow() {
		void refreshProviders();
		if (new URLSearchParams(window.location.search).get('add') === '1') writeURL();
	}

	async function load() {
		loading = true;
		failed = '';
		try {
			const data = await api<{ items: Provider[] }>('/connections');
			applyProviders(data.items ?? []);
			if (view === 'models') await loadAllModels(true);
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
			writeURL();
		}
	}

	// The selection stays on a connection that still exists.
	function applyProviders(items: Provider[]) {
		providers = items;
		if (view === 'provider' && (selectedId === '' || !providers.some((p) => p.id === selectedId))) {
			selectedId = providers[0]?.id ?? '';
		}
	}

	// A connection just saved appears and one a discarded sign-in left behind
	// disappears, without reloading the templates, the all-models paging, or the
	// loading state the whole page would flash.
	async function refreshProviders() {
		try {
			const data = await api<{ items: Provider[] }>('/connections');
			applyProviders(data.items ?? []);
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		}
	}

	async function loadCatalog() {
		try {
			const data = await api<ProviderTemplatesResponse>('/templates');
			templates = offeredTemplates(data.items ?? []);
			modelsdev = data.modelsdev ?? null;
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		}
	}

	// readingCatalog is the read in flight, so a caller that needs the list
	// waits on the read already running rather than asking the daemon twice.
	let readingCatalog: Promise<void> | null = null;

	function readCatalog(): Promise<void> {
		readingCatalog ??= loadCatalog().finally(() => {
			readingCatalog = null;
		});
		return readingCatalog;
	}

	async function loadAllModels(restart = false) {
		allLoading = true;
		allFailed = '';
		try {
			if (restart) {
				allModels = [];
				allTotal = 0;
			}
			const offset = allModels.length;
			const page = await api<{ items: Model[]; total: number }>(
				'/models?limit=' + allPageSize + '&offset=' + offset
			);
			allModels = [...allModels, ...(page.items ?? [])];
			allTotal = page.total ?? allModels.length;
		} catch (error) {
			allFailed = error instanceof Error ? error.message : String(error);
		} finally {
			allLoading = false;
		}
	}

	async function openAdd() {
		await readCatalog();
		addOpen = true;
		writeURL();
	}

	async function updateCatalog() {
		updating = true;
		try {
			const result = await api<CatalogRefreshResult>('/catalog/refresh', { method: 'POST' });
			modelsdev = result.metadata?.modelsdev ?? modelsdev;
			const notice = $t('ui.pages.providersPage.header.catalogUpdated', {
				values: {
					updated: result.updated,
					skipped: result.skipped,
					failed: result.failed,
					matched: result.metadata?.models_matched ?? 0,
					changed: result.metadata?.prices_changed ?? 0
				}
			});
			const failures = (result.providers ?? [])
				.filter((outcome) => outcome.status === 'failed')
				.map((outcome) =>
					$t('ui.pages.providersPage.header.connectionFailed', {
						values: { label: outcome.label || outcome.provider_id, detail: outcome.detail ?? '' }
					})
				);
			// The list, the templates and the quota are separate reads of what
			// the refresh just rewrote, so they go out together.
			await Promise.all([load(), loadCatalog(), loadQuota()]);
			if (failures.length > 0 || result.metadata_error) {
				toast.error(
					$t('ui.pages.providersPage.header.catalogPartial', {
						values: { failed: result.failed + (result.metadata_error ? 1 : 0) }
					})
				);
			} else {
				toast.success(notice);
			}
		} catch (error) {
			toast.error(
				$t('ui.pages.providersPage.header.catalogFailed', {
					values: { detail: error instanceof Error ? error.message : String(error) }
				})
			);
		} finally {
			updating = false;
		}
	}

	// An attention chip or the unpriced tag passes the one model filter it wants
	// the pane to start on; every other entry starts on every model.
	function selectProvider(id: string, label: ModelLabelFilter = 'all') {
		modelLabel = label;
		selectedId = id;
		view = 'provider';
		paneOpen = true;
		writeURL();
	}

	function selectAllModels() {
		view = 'models';
		tab = 'models';
		paneOpen = true;
		writeURL();
		void loadAllModels(true);
	}

	function backToList() {
		paneOpen = false;
		writeURL();
	}

	function selectTab(next: string) {
		tab = next;
		writeURL();
	}

	async function refreshDev() {
		await loadCatalog();
	}

	function onProviderChanged(updated: Provider) {
		providers = providers.map((provider) => (provider.id === updated.id ? updated : provider));
	}

	async function onAdded(providerId: string) {
		// The connection list and the quota behind its footer are separate
		// reads, so they go out together rather than one after the other.
		await Promise.all([refreshProviders(), loadQuota()]);
		const added = providers.find((provider) => provider.id === providerId);
		if (added) {
			selectProvider(providerId);
		} else {
			// The flow is closed with no card to select, so the URL is the only
			// thing left still asking for it.
			writeURL();
		}
		// The count is the one the connection now has, which is what a sign-in
		// has just filled in and what a saved key may have widened.
		toast.success(
			$t('ui.pages.providersPage.review.added', {
				values: { label: added?.label ?? providerId, count: added?.counts.models ?? 0 }
			})
		);
	}

	function onDeleted() {
		selectedId = '';
		tab = 'models';
		void loadQuota();
		// The pane held a connection that no longer exists, so the list is what
		// the operator is left with.
		paneOpen = false;
		void load().then(() => {
			requestAnimationFrame(() => {
				const target =
					document.querySelector<HTMLElement>('[aria-current="true"]') ??
					document.getElementById('add-connection-button');
				target?.focus();
			});
		});
	}

	// A format list for a provider the daemon knows nothing about still offers
	// every shape Relo can speak.
	const formatsFor = (provider: Provider) =>
		(provider.available_formats ?? []).length > 0
			? (provider.available_formats ?? [])
			: API_FORMATS.map((format) => ({
					format,
					default_base_url: provider.base_url,
					key_header: provider.key_header,
					models_format: provider.models_format,
					label: format
				}));
</script>

<svelte:head><title>{$t('ui.pages.providersPage.header.title')} · Relo</title></svelte:head>

<div class="flex min-h-full flex-col bg-background">
	<PageHeader
		title="ui.pages.providersPage.header.title"
		description="ui.pages.providersPage.header.subtitle"
	>
		{#snippet titleSuffix()}
			{#if view === 'models' && paneOpen}
				<span
					class="shrink-0 rounded-full border border-border px-2 py-0.5 font-mono text-xs text-muted-foreground max-sm:hidden"
				>
					{$t('ui.pages.providersPage.list.modelCount', { values: { count: allTotal } })}
				</span>
			{/if}
		{/snippet}
		{#snippet actions()}
			<Button id="add-connection-button" onclick={openAdd}>
				<Icon name="plus" size={14} />
				{$t('ui.pages.providersPage.header.addProvider')}
			</Button>
		{/snippet}
	</PageHeader>

	<div class="page-frame page-stack pt-2">
		{#if failed}
			<p class="text-sm text-destructive" role="alert">
				{$t('ui.pages.providersPage.list.loadFailed', { values: { detail: failed } })}
			</p>
		{/if}

		{#if loading && providers.length === 0}
			<div class="flex flex-col gap-3">
				{#each [0, 1, 2, 3] as row (row)}
					<div class="h-16 animate-pulse rounded-xl border border-border bg-muted/40"></div>
				{/each}
			</div>
		{:else if providers.length === 0 && view === 'provider'}
			<div
				class="empty-panel rounded-card border-[1.5px] border-dashed border-accent-border bg-accent-soft"
			>
				<h2 class="section-heading text-ink">{$t('ui.pages.providersPage.list.emptyTitle')}</h2>
				<p class="max-w-[46ch] text-sm text-muted-foreground">
					{$t('ui.pages.providersPage.list.emptyBody')}
				</p>
				<Button onclick={openAdd}>
					<Icon name="plus" size={14} />
					{$t('ui.pages.providersPage.header.addProvider')}
				</Button>
			</div>
		{:else}
			<div class="flex">
				<main class="min-w-0 flex-1">
					{#if !paneOpen}
						<ConnectionsOverview
							{cards}
							{updating}
							onopen={selectProvider}
							onrefresh={() => {
								void refreshProviders();
								void loadQuota();
							}}
							onallmodels={selectAllModels}
							onupdate={() => void updateCatalog()}
							onadd={openAdd}
						/>
					{:else if view === 'models'}
						<div class="flex flex-col">
							<ModelList
								models={allModels}
								loading={allLoading}
								failed={allFailed}
								showProvider
								{providerLabels}
								filters={allFilters}
								onfilters={(next) => (allFilters = next)}
								hasMore={allModels.length < allTotal}
								onloadmore={() => void loadAllModels(false)}
								onreload={() => void loadAllModels(true)}
								onopenprovider={(id) => selectProvider(id)}
							>
								{#snippet toolbarStart()}
									<Button variant="outline" onclick={backToList}>
										<Icon name="arrow-left" size={14} />
										{$t('ui.pages.providersPage.pane.back')}
									</Button>
								{/snippet}
								{#snippet toolbarTitle()}
									<div class="flex min-w-0 items-center justify-center gap-2 sm:hidden">
										<span
											class="shrink-0 rounded-full border border-border px-2 py-0.5 font-mono text-xs text-muted-foreground"
										>
											{$t('ui.pages.providersPage.list.modelCount', {
												values: { count: allTotal }
											})}
										</span>
									</div>
								{/snippet}
							</ModelList>
						</div>
					{:else if selected}
						<!-- The pane keeps per-connection toolbar state, so a
						     different connection starts from its own. -->
						{#key selected.id}
							<ProviderPane
								provider={selected}
								formats={formatsFor(selected)}
								windows={quotaByConnection.get(selected.id) ?? []}
								labelFilter={modelLabel}
								{tab}
								ontab={selectTab}
								onback={backToList}
								onchanged={onProviderChanged}
								ondeleted={onDeleted}
								onquotas={() => {
									void loadQuota();
									void loadCatalog();
								}}
								onaddkey={() => {
									addOpen = true;
								}}
							/>
						{/key}
					{:else}
						<div class="flex items-center justify-center py-6 text-sm text-muted-foreground">
							{$t('ui.pages.providersPage.pane.notFound')}
						</div>
					{/if}
				</main>
			</div>
		{/if}
	</div>
</div>

<AddProviderStepper
	bind:open={addOpen}
	{templates}
	{modelsdev}
	{providers}
	onadded={onAdded}
	ondiscarded={closeAddFlow}
	onrefreshdev={refreshDev}
/>
