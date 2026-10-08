<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { page } from '$app/state';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import AddTile from '$lib/components/ui/add-tile.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import FilterSearch from '$lib/components/shell/filter-search.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import CardLayoutToggle from '$lib/components/providers/card-layout-toggle.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import KeyCard from '$lib/components/keys/key-card.svelte';
	import KeyCreateModal from '$lib/components/keys/key-create-modal.svelte';
	import KeyDetailModal from '$lib/components/keys/key-detail-modal.svelte';
	import KeyRevealModal from '$lib/components/keys/key-reveal-modal.svelte';
	import { canDeleteExpired, groupKeys, type KeyStatusFilter } from '$lib/access-keys';
	import type { DataPlaneURLs } from '$lib/client-setup';
	import { consoleState } from '$lib/console-state.svelte';
	import { cardLayoutGrid } from '$lib/theme.svelte';
	import type { AccessKey, DataPlaneAddress, IssuedAccessKey, StatusResponse } from '$lib/types';

	let keys = $state<AccessKey[]>([]);
	let loading = $state(true);
	let failure = $state('');
	let searchQ = $state('');
	let statusFilter = $state<KeyStatusFilter>('all');
	let selected = $state<string[]>([]);
	let revokedOpen = $state(false);

	// The search narrows first, and the status filter then decides which of
	// the groups the list draws.
	const searched = $derived.by(() => {
		const q = searchQ.trim().toLowerCase();
		if (!q) return keys;
		return keys.filter(
			(key) =>
				key.name.toLowerCase().includes(q) ||
				(key.client ?? '').toLowerCase().includes(q) ||
				key.token_hint.includes(q)
		);
	});
	// Inactive keys pile up as they expire, so an operator's live keys stay on
	// top and the lapsed ones sit in their own section below.
	const grouped = $derived(groupKeys(searched, statusFilter));
	const empty = $derived(
		grouped.active.length === 0 && grouped.expired.length === 0 && grouped.revoked.length === 0
	);
	const expiredDeletable = $derived(grouped.expired.filter((key) => canDeleteExpired(key)));
	const selectedExpired = $derived(
		selected.filter((id) => expiredDeletable.some((key) => key.id === id))
	);
	// The filter options carry their counts, so the list is legible before it
	// is opened.
	const counts = $derived.by(() => {
		const active = keys.filter((key) => key.status === 'active').length;
		return { all: keys.length, active, inactive: keys.length - active };
	});

	// The detail modal reads and edits one key; rotation and revocation leave
	// it first, so one surface is ever open over the page.
	let detailOpen = $state(false);
	let detailKey = $state<AccessKey | null>(null);

	let createOpen = $state(false);

	// The reveal surface shows a key once, the moment it exists.
	let revealOpen = $state(false);
	let revealed = $state<IssuedAccessKey | null>(null);
	let revealedFor = $state('codex');

	// The address a client points at, read from the daemon rather than
	// written here.
	let dataPlane = $state<DataPlaneAddress[]>([]);
	const urls = $derived<DataPlaneURLs>({
		openai:
			(dataPlane.find((entry) => entry.protocol === 'openai') ?? dataPlane[0])?.base_url ??
			'http://127.0.0.1:10201',
		anthropic:
			(dataPlane.find((entry) => entry.protocol === 'anthropic') ?? dataPlane[1])?.base_url ??
			'http://127.0.0.1:10202'
	});

	// Destructive confirmations
	let rotating = $state<AccessKey | null>(null);
	let revoking = $state<AccessKey | null>(null);
	let rotateOpen = $state(false);
	let revokeOpen = $state(false);
	let deleteSelectedOpen = $state(false);
	let clearExpiredOpen = $state(false);
	let busy = $state(false);

	async function load() {
		failure = '';
		// The key list and the data plane are separate reads, so they go out
		// together rather than one after the other.
		const [list, status] = await Promise.allSettled([
			api<{ items: AccessKey[] }>('/clients/keys'),
			api<StatusResponse>('/status')
		]);
		loading = false;
		if (list.status === 'fulfilled') keys = list.value.items ?? [];
		else failure = list.reason instanceof Error ? list.reason.message : String(list.reason);
		dataPlane = status.status === 'fulfilled' ? (status.value.data_plane ?? []) : [];
	}

	onMount(() => {
		void load();
		if (page.url.searchParams.get('create') === '1') createOpen = true;
	});

	// A search that lands on the revoked group opens it: the operator asked to
	// see those keys, so leaving the group closed would hide the answer. The
	// group only renders when the search still leaves revoked keys in it.
	function onSearch(next: string) {
		searchQ = next;
		if (next.trim() !== '' && grouped.revoked.length > 0) revokedOpen = true;
	}

	function clearNarrowing() {
		searchQ = '';
		statusFilter = 'all';
	}

	function openDetail(key: AccessKey) {
		detailKey = key;
		detailOpen = true;
	}

	// A saved edit lands in a freshly read list, so the modal shows the key as
	// the daemon now holds it rather than the copy it edited.
	async function onDetailSaved() {
		await load();
		detailKey = keys.find((key) => key.id === detailKey?.id) ?? null;
	}

	// A rotation or a revocation leaves the detail modal first, so one
	// surface is ever open over the page.
	function askRotate(key: AccessKey) {
		rotating = key;
		detailOpen = false;
		detailKey = null;
		rotateOpen = true;
	}

	function askRevoke(key: AccessKey) {
		revoking = key;
		detailOpen = false;
		detailKey = null;
		revokeOpen = true;
	}

	function openReveal(issued: IssuedAccessKey, clientId: string) {
		revealed = issued;
		revealedFor = clientId;
		revealOpen = true;
	}

	async function rotateKey() {
		if (!rotating) return;
		busy = true;
		try {
			const rotatedClient = rotating.client ?? 'codex';
			const issued = await api<IssuedAccessKey>(
				'/clients/keys/' + encodeURIComponent(rotating.id) + '/rotate',
				{ method: 'POST' }
			);
			rotating = null;
			rotateOpen = false;
			openReveal(issued, rotatedClient);
			await load();
			toast.success($t('ui.pages.keysPage.rotated'));
		} catch (error) {
			toast.error(error instanceof Error ? error.message : $t('ui.common.error'));
		} finally {
			busy = false;
		}
	}

	async function revokeKey() {
		if (!revoking) return;
		busy = true;
		try {
			await api('/clients/keys/' + encodeURIComponent(revoking.id), { method: 'DELETE' });
			revoking = null;
			revokeOpen = false;
			await load();
			toast.success($t('ui.pages.keysPage.revoked'));
		} catch (error) {
			toast.error(error instanceof Error ? error.message : $t('ui.common.error'));
		} finally {
			busy = false;
		}
	}

	function toggleSelected(id: string, on: boolean) {
		selected = on ? [...new Set([...selected, id])] : selected.filter((item) => item !== id);
	}

	async function deleteExpired(ids?: string[]) {
		busy = true;
		try {
			await api('/clients/keys/deletions', {
				method: 'POST',
				body: JSON.stringify(ids ? { ids } : {})
			});
			selected = [];
			deleteSelectedOpen = false;
			clearExpiredOpen = false;
			await load();
			toast.success($t('ui.pages.keysPage.deleted'));
		} catch (error) {
			toast.error(error instanceof Error ? error.message : $t('ui.common.error'));
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head><title>{$t('ui.pages.keys.title')} · Relo</title></svelte:head>

<PageHeader title="ui.pages.keys.title" description="ui.pages.keys.description">
	{#snippet actions()}
		<Button onclick={() => (createOpen = true)}>
			<Icon name="plus" size={14} />
			{$t('ui.pages.keysPage.create')}
		</Button>
	{/snippet}
</PageHeader>

<div class="page-frame page-stack pt-6">
	<div class="flex flex-wrap items-center gap-3">
		<FilterSearch
			filterId="key-filter"
			filterLabel={$t('ui.pages.keysPage.columnStatus')}
			searchLabel={$t('ui.pages.keysPage.searchPlaceholder')}
			searchPlaceholder={$t('ui.pages.keysPage.searchPlaceholder')}
			filterFullRowOnMobile
			bind:filter={statusFilter}
			search={searchQ}
			onsearch={(next) => onSearch(next)}
		>
			<option value="all">
				{$t('ui.pages.keysPage.filterOption', {
					values: { label: $t('ui.pages.keysPage.filterAll'), count: counts.all }
				})}
			</option>
			<option value="active">
				{$t('ui.pages.keysPage.filterOption', {
					values: { label: $t('ui.pages.keysPage.filterActive'), count: counts.active }
				})}
			</option>
			<option value="inactive">
				{$t('ui.pages.keysPage.filterOption', {
					values: { label: $t('ui.pages.keysPage.filterInactive'), count: counts.inactive }
				})}
			</option>
		</FilterSearch>
		<div class="ml-auto flex flex-wrap items-center gap-2">
			{#if selectedExpired.length > 0}
				<Button variant="destructive" onclick={() => (deleteSelectedOpen = true)}>
					<Icon name="trash" size={14} />
					{$t('ui.pages.keysPage.deleteSelected')}
				</Button>
			{/if}
			{#if expiredDeletable.length > 0}
				<Button variant="outline" onclick={() => (clearExpiredOpen = true)}>
					<Icon name="trash" size={14} />
					{$t('ui.pages.keysPage.clearExpired')}
				</Button>
			{/if}
			<div class="hidden sm:block">
				<CardLayoutToggle />
			</div>
		</div>
	</div>

	{#if loading}
		<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
			{#each [0, 1, 2, 3] as placeholder (placeholder)}
				<div class="section-surface flat-surface h-56 animate-pulse bg-sunken"></div>
			{/each}
		</div>
	{:else if failure}
		<p class="text-sm text-danger" role="alert">{failure}</p>
	{:else if empty}
		<div
			class="empty-panel rounded-card border-[1.5px] border-dashed border-accent-border bg-accent-soft"
		>
			<p class="section-heading text-ink">
				{keys.length === 0
					? $t('ui.pages.keysPage.emptyTitle')
					: $t('ui.pages.keysPage.noMatchTitle')}
			</p>
			<p class="max-w-[46ch] text-sm text-muted-foreground">
				{keys.length === 0 ? $t('ui.pages.keysPage.empty') : $t('ui.pages.keysPage.noMatch')}
			</p>
			{#if keys.length === 0}
				<Button onclick={() => (createOpen = true)}>
					<Icon name="plus" size={14} />
					{$t('ui.pages.keysPage.create')}
				</Button>
			{:else}
				<Button variant="outline" onclick={clearNarrowing}>
					<Icon name="x" size={14} />
					{$t('ui.pages.keysPage.clearFilters')}
				</Button>
			{/if}
		</div>
	{:else}
		<div class="flex flex-col gap-6">
			{#if grouped.active.length > 0}
				<section class="space-y-3">
					<h2 class="section-heading">
						{$t('ui.pages.keysPage.filterOption', {
							values: { label: $t('ui.pages.keysPage.filterActive'), count: grouped.active.length }
						})}
					</h2>
					<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
						{#each grouped.active as key (key.id)}
							<KeyCard
								accessKey={key}
								selected={selected.includes(key.id)}
								onopen={openDetail}
								onselect={toggleSelected}
							/>
						{/each}
						<AddTile
							label={$t('ui.pages.keysPage.create')}
							hint={$t('ui.pages.keysPage.addTileHint')}
							onclick={() => (createOpen = true)}
						/>
					</div>
				</section>
			{:else}
				<!-- With no live keys the tile still needs a grid of its own, so
				     the add action stays above the history sections. -->
				<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
					<AddTile
						label={$t('ui.pages.keysPage.create')}
						hint={$t('ui.pages.keysPage.addTileHint')}
						onclick={() => (createOpen = true)}
					/>
				</div>
			{/if}

			{#if grouped.expired.length > 0}
				<section class="space-y-3">
					<h2 class="section-heading">
						{$t('ui.pages.keysPage.filterOption', {
							values: {
								label: $t('ui.status.badge.expired'),
								count: grouped.expired.length
							}
						})}
					</h2>
					<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
						{#each grouped.expired as key (key.id)}
							<KeyCard
								accessKey={key}
								selectable={canDeleteExpired(key)}
								selected={selected.includes(key.id)}
								onopen={openDetail}
								onselect={toggleSelected}
							/>
						{/each}
					</div>
				</section>
			{/if}

			{#if grouped.revoked.length > 0}
				<details bind:open={revokedOpen}>
					<summary
						class="flex cursor-pointer list-none items-center gap-2 py-1 [&::-webkit-details-marker]:hidden"
					>
						<Icon
							name={revokedOpen ? 'chevron-down' : 'chevron-right'}
							size={14}
							class="shrink-0 text-muted-foreground"
						/>
						<span class="section-heading">
							{$t('ui.pages.keysPage.revokedSection', {
								values: { count: grouped.revoked.length }
							})}
						</span>
					</summary>
					<div class="mt-3 {cardLayoutGrid(consoleState.settings.cardLayout)}">
						{#each grouped.revoked as key (key.id)}
							<KeyCard accessKey={key} onopen={openDetail} onselect={toggleSelected} />
						{/each}
					</div>
				</details>
			{/if}
		</div>
	{/if}
</div>

<KeyCreateModal
	bind:open={createOpen}
	oncreated={(issued, clientId) => {
		openReveal(issued, clientId);
		void load();
	}}
/>

<KeyRevealModal
	bind:open={revealOpen}
	issued={revealed}
	issuedFor={revealedFor}
	{urls}
	onclose={() => (revealed = null)}
/>

<KeyDetailModal
	bind:open={detailOpen}
	accessKey={detailKey}
	onsaved={() => void onDetailSaved()}
	onrotate={askRotate}
	onrevoke={askRevoke}
	onclose={() => (detailKey = null)}
/>

<ConfirmDialog
	bind:open={rotateOpen}
	title={$t('ui.pages.keysPage.rotateConfirmTitle')}
	body={$t('ui.pages.keysPage.rotateConfirmBody')}
	confirmLabel={$t('ui.pages.keysPage.rotate')}
	{busy}
	icon="refresh"
	onconfirm={rotateKey}
/>

<ConfirmDialog
	bind:open={revokeOpen}
	title={$t('ui.pages.keysPage.revokeConfirmTitle')}
	body={$t('ui.pages.keysPage.revokeConfirmBody')}
	confirmLabel={$t('ui.pages.keysPage.revoke')}
	tone="destructive"
	{busy}
	icon="trash"
	onconfirm={revokeKey}
/>

<ConfirmDialog
	bind:open={deleteSelectedOpen}
	title={$t('ui.pages.keysPage.deleteSelectedConfirmTitle')}
	body={$t('ui.pages.keysPage.deleteSelectedConfirmBody')}
	confirmLabel={$t('ui.pages.keysPage.deleteSelected')}
	tone="destructive"
	{busy}
	icon="trash"
	onconfirm={() => deleteExpired(selectedExpired)}
/>

<ConfirmDialog
	bind:open={clearExpiredOpen}
	title={$t('ui.pages.keysPage.clearExpiredConfirmTitle')}
	body={$t('ui.pages.keysPage.clearExpiredConfirmBody')}
	confirmLabel={$t('ui.pages.keysPage.clearExpired')}
	tone="destructive"
	{busy}
	icon="trash"
	onconfirm={() => deleteExpired()}
/>
