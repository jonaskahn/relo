<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api';
	import AddTile from '$lib/components/ui/add-tile.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import CardLayoutToggle from '$lib/components/providers/card-layout-toggle.svelte';
	import GroupCard from '$lib/components/groups/group-card.svelte';
	import GroupDetail from '$lib/components/groups/group-detail.svelte';
	import RouteStepper from '$lib/components/groups/route-stepper.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import { exposedName, groupMembersOf } from '$lib/catalog';
	import { consoleState } from '$lib/console-state.svelte';
	import { cardLayoutGrid } from '$lib/theme.svelte';
	import type { Group, Model, Provider } from '$lib/types';
	import { toast } from 'svelte-sonner';
	import FilterSearch from '$lib/components/shell/filter-search.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	// CapabilityFilter is the filter's one choice: everything, or one
	// capability a group can promise whichever member answers.
	type CapabilityFilter = 'all' | 'tools' | 'vision' | 'reasoning';

	let groups = $state<Group[]>([]);
	let providers = $state<Provider[]>([]);
	// The detail modal shows what a rate costs, and the group the daemon
	// stored carries no prices: the catalog does, one read per connection
	// the members name. A connection that cannot be read simply leaves its
	// members without a rate, rather than holding the whole grid.
	let meta = $state<Record<string, Model>>({});
	let loading = $state(true);
	let failure = $state('');
	// The toolbar narrows the grid two ways. The search reads what an operator
	// already sees — the label, the group name, the name an agent asks for — so
	// a half-remembered slug finds the group as well as a half-typed label. The
	// filter asks what the group can promise, one choice at a time the way the
	// connections page narrows its list.
	let search = $state('');
	let capability = $state<CapabilityFilter>('all');

	// The stepper owns the editor, the page owns the list: editing names the
	// group it opened on, the modal is where a group is read, and removing is
	// the one action a confirm is about.
	let editorOpen = $state(false);
	let editing = $state<Group | null>(null);
	let detailOpen = $state(false);
	let detailing = $state<Group | null>(null);
	let removing = $state<Group | null>(null);
	let removeOpen = $state(false);

	// A connection a group can point at is one that routes and serves models.
	const configured = $derived(
		providers.filter((provider) => provider.routable && provider.counts.models > 0)
	);

	const query = $derived(search.trim().toLowerCase());
	const searching = $derived(query !== '');
	const filtersActive = $derived(capability !== 'all');
	const searched = $derived(
		groups.filter((group) => matchesFilters(group, capability) && matchesQuery(group))
	);

	onMount(load);

	async function load() {
		loading = true;
		failure = '';
		try {
			const [groupList, providerList] = await Promise.all([
				api<{ items: Group[] }>('/routes'),
				api<{ items: Provider[] }>('/connections')
			]);
			groups = groupList.items ?? [];
			providers = providerList.items ?? [];
			void readMemberModels();
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			loading = false;
		}
	}

	// The page does not flash its loading state over a group that was just
	// stored.
	async function refreshGroups() {
		try {
			const data = await api<{ items: Group[] }>('/routes');
			groups = data.items ?? [];
			void readMemberModels();
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		}
	}

	async function readMemberModels() {
		const ids = new Set<string>();
		for (const group of groups) {
			for (const member of groupMembersOf(group)) {
				if (member.provider_id !== '') ids.add(member.provider_id);
			}
		}
		if (ids.size === 0) return;
		const results = await Promise.allSettled(
			[...ids].map((id) =>
				api<{ items: Model[] }>('/models?limit=500&provider=' + encodeURIComponent(id))
			)
		);
		const learned: Record<string, Model> = {};
		for (const result of results) {
			if (result.status !== 'fulfilled') continue;
			for (const model of result.value.items ?? []) {
				learned[model.provider_id + '/' + model.model_id] = model;
			}
		}
		if (Object.keys(learned).length > 0) meta = { ...meta, ...learned };
	}

	function openCreate() {
		editing = null;
		editorOpen = true;
	}

	function openEdit(group: Group) {
		editing = group;
		editorOpen = true;
	}

	function openDetail(group: Group) {
		detailing = group;
		detailOpen = true;
	}

	function editFromDetail(group: Group) {
		detailOpen = false;
		detailing = null;
		openEdit(group);
	}

	// A confirm never stacks on top of the modal that asked for it, so the
	// detail view is closed before the question appears.
	function removeFromDetail(group: Group) {
		detailOpen = false;
		detailing = null;
		askRemove(group);
	}

	function askRemove(group: Group) {
		removing = group;
		removeOpen = true;
	}

	async function remove() {
		if (!removing) return;
		try {
			await api('/routes/' + encodeURIComponent(removing.id), { method: 'DELETE' });
			removing = null;
			removeOpen = false;
			await load();
		} catch (error) {
			toast.error(error instanceof Error ? error.message : String(error));
		}
	}

	// The filter's choices: everything, or one capability the group promises
	// whichever member answers. The option words reuse the card's capability
	// line, so one concept never gets two names in one language.
	const CAPABILITIES = [
		{ value: 'all', labelKey: 'ui.common.all' },
		{ value: 'tools', labelKey: 'ui.pages.groupsPage.chipTools' },
		{ value: 'vision', labelKey: 'ui.pages.groupsPage.chipVision' },
		{ value: 'reasoning', labelKey: 'ui.pages.groupsPage.chipReasoning' }
	] as const;

	// A capability nothing states is null, and null never matches, so an absence
	// is never shown as a promise the group does not make.
	function matchesFilters(group: Group, kind: CapabilityFilter): boolean {
		if (kind === 'all') return true;
		if (kind === 'tools') return group.supports_tools === true;
		if (kind === 'reasoning') return group.supports_reasoning === true;
		return group.supports_vision === true;
	}

	function matchesQuery(group: Group): boolean {
		if (!searching) return true;
		const name = group.label || group.id;
		return (
			name.toLowerCase().includes(query) ||
			group.id.toLowerCase().includes(query) ||
			exposedName(group).toLowerCase().includes(query) ||
			$t('ui.pages.groupsPage.strategyName.' + group.strategy)
				.toLowerCase()
				.includes(query)
		);
	}

	// The count is what the option would leave given the search already typed, so
	// it reads "Vision (5)" before it is chosen.
	function optionCount(kind: CapabilityFilter): number {
		return groups.filter((group) => matchesQuery(group) && matchesFilters(group, kind)).length;
	}

	function clearFilters() {
		capability = 'all';
	}
</script>

<svelte:head><title>{$t('ui.pages.groupsPage.title')} · Relo</title></svelte:head>

<PageHeader title="ui.pages.groupsPage.title" description="ui.pages.groupsPage.subtitle">
	{#snippet actions()}
		<Button onclick={openCreate}>
			<Icon name="plus" size={14} />
			{$t('ui.pages.groupsPage.create')}
		</Button>
	{/snippet}
</PageHeader>

<div class="page-frame page-stack pt-6">
	<div class="flex flex-wrap items-center gap-3">
		<FilterSearch
			filterId="group-filter"
			filterLabel={$t('ui.pages.groupsPage.filterLabel')}
			searchLabel={$t('ui.pages.groupsPage.searchGroups')}
			searchPlaceholder={$t('ui.pages.groupsPage.searchGroups')}
			bind:filter={capability}
			bind:search
		>
			{#each CAPABILITIES as option (option.value)}
				<option value={option.value}>
					{$t('ui.pages.groupsPage.filterOption', {
						values: { label: $t(option.labelKey), count: optionCount(option.value) }
					})}
				</option>
			{/each}
		</FilterSearch>
		<div class="ml-auto hidden items-center sm:flex">
			<CardLayoutToggle />
		</div>
	</div>

	{#if failure}
		<p class="text-sm text-destructive" role="alert">{failure}</p>
	{/if}

	{#if loading}
		<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
			{#each [0, 1, 2, 3] as placeholder (placeholder)}
				<div class="section-surface flat-surface h-56 animate-pulse bg-sunken"></div>
			{/each}
		</div>
	{:else if searched.length === 0}
		{@const narrowed = searching || filtersActive}
		<div
			class="empty-panel rounded-card border-[1.5px] border-dashed border-accent-border bg-accent-soft"
		>
			<p class="section-heading text-ink">
				{narrowed
					? $t('ui.pages.groupsPage.emptySearchTitle')
					: $t('ui.pages.groupsPage.emptyTitle')}
			</p>
			<p class="max-w-[46ch] text-sm text-muted-foreground">
				{searching
					? $t('ui.pages.groupsPage.emptySearch')
					: filtersActive
						? $t('ui.pages.groupsPage.emptyFilters')
						: $t('ui.pages.groupsPage.empty')}
			</p>
			{#if narrowed}
				<div class="flex flex-wrap items-center justify-center gap-2">
					{#if searching}
						<Button variant="outline" onclick={() => (search = '')}>
							<Icon name="x" size={14} />
							{$t('ui.pages.groupsPage.clearSearch')}
						</Button>
					{/if}
					{#if filtersActive}
						<Button variant="outline" onclick={clearFilters}>
							<Icon name="x" size={14} />
							{$t('ui.pages.groupsPage.clearFilters')}
						</Button>
					{/if}
				</div>
			{:else}
				<Button onclick={openCreate}>
					<Icon name="plus" size={14} />
					{$t('ui.pages.groupsPage.create')}
				</Button>
			{/if}
		</div>
	{:else}
		<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
			{#each searched as group (group.id)}
				<GroupCard {group} {providers} onopen={openDetail} onedit={openEdit} onremove={askRemove} />
			{/each}
			<AddTile
				label={$t('ui.pages.groupsPage.create')}
				hint={$t('ui.pages.groupsPage.addTileHint')}
				onclick={openCreate}
			/>
		</div>
	{/if}
</div>

<GroupDetail
	bind:open={detailOpen}
	group={detailing}
	{providers}
	{meta}
	onedit={editFromDetail}
	ondelete={removeFromDetail}
/>

<RouteStepper
	bind:open={editorOpen}
	{providers}
	{configured}
	{editing}
	onsaved={() => void refreshGroups()}
/>

<ConfirmDialog
	bind:open={removeOpen}
	title={$t('ui.pages.groupsPage.removeTitle', {
		values: { label: removing?.label || removing?.id || '' }
	})}
	body={$t('ui.pages.groupsPage.removeBody')}
	confirmLabel={$t('ui.common.remove')}
	tone="destructive"
	icon="trash"
	onconfirm={remove}
/>
