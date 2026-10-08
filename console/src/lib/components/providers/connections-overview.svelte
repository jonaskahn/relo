<script lang="ts">
	import { t } from 'svelte-i18n';

	import AddTile from '$lib/components/ui/add-tile.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import FilterSearch from '$lib/components/shell/filter-search.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import AttentionStrip from '$lib/components/providers/attention-strip.svelte';
	import CardLayoutToggle from '$lib/components/providers/card-layout-toggle.svelte';
	import ProviderOverviewCard from '$lib/components/providers/provider-overview-card.svelte';
	import { connectionAttention, unpricedTotal } from '$lib/provider-attention';
	import type { ModelLabelFilter } from '$lib/model-filters';
	import type { ConnectionCard } from '$lib/provider-quota';
	import { consoleState } from '$lib/console-state.svelte';
	import { cardLayoutGrid } from '$lib/theme.svelte';

	// ConnectionsOverview is the list pane: the search and filter that narrow
	// it, the attention reading across every connection, and the card grid.
	// The search narrows in place, so finding one connection among many never
	// opens a detail pane. The attention reading is derived from the same
	// quota the cards close with, so the strip and the cards never disagree.

	interface Props {
		cards: ConnectionCard[];
		updating?: boolean;
		onopen: (id: string, label: ModelLabelFilter) => void;
		onrefresh: () => void;
		onallmodels: () => void;
		onupdate: () => void;
		onadd: () => void;
	}

	let {
		cards,
		updating = false,
		onopen,
		onrefresh,
		onallmodels,
		onupdate,
		onadd
	}: Props = $props();

	let query = $state('');
	let filter = $state<'all' | 'attention' | 'unpriced'>('all');

	const providers = $derived(cards.map((card) => card.provider));
	const attentionItems = $derived(
		connectionAttention(providers, new Map(cards.map((card) => [card.provider.id, card.windows])))
	);
	const attentionIds = $derived(new Set(attentionItems.map((item) => item.providerId)));
	const unpricedModels = $derived(unpricedTotal(providers));
	const unpricedConnections = $derived(
		providers.filter((provider) => provider.counts.unpriced_models > 0).length
	);

	const visible = $derived(
		cards.filter((card) => {
			const provider = card.provider;
			const needle = query.trim().toLowerCase();
			if (needle !== '') {
				const labels = card.accounts.map((account) => account.label).join(' ');
				const haystack = (provider.label + ' ' + provider.id + ' ' + labels).toLowerCase();
				if (!haystack.includes(needle)) return false;
			}
			if (filter === 'attention') return attentionIds.has(provider.id);
			if (filter === 'unpriced') return provider.counts.unpriced_models > 0;
			return true;
		})
	);
</script>

<div class="flex flex-col">
	<header class="flex flex-wrap items-center gap-3 pt-4">
		<FilterSearch
			filterId="connection-filter"
			filterLabel={$t('ui.pages.providersPage.list.filters')}
			searchLabel={$t('ui.pages.providersPage.list.search')}
			searchPlaceholder={$t('ui.pages.providersPage.list.search')}
			bind:filter
			bind:search={query}
		>
			<option value="all">
				{$t('ui.pages.providersPage.list.filterOption', {
					values: {
						label: $t('ui.pages.providersPage.modelsTable.segmentAll'),
						count: providers.length
					}
				})}
			</option>
			<option value="attention">
				{$t('ui.pages.providersPage.list.filterOption', {
					values: {
						label: $t('ui.pages.providersPage.list.needsAttention'),
						count: attentionItems.length
					}
				})}
			</option>
			<option value="unpriced">
				{$t('ui.pages.providersPage.list.filterOption', {
					values: {
						label: $t('ui.pages.providersPage.models.chipUnpriced'),
						count: unpricedConnections
					}
				})}
			</option>
		</FilterSearch>
		<AttentionStrip
			variant="inline"
			items={attentionItems}
			unpriced={unpricedModels}
			onopen={(id) => onopen(id, 'all')}
			onunpriced={() => (filter = 'unpriced')}
		/>
		<div class="ml-auto flex flex-wrap items-center gap-2">
			<Button variant="outline" size="sm" onclick={onallmodels}>
				<Icon name="list" size={14} />
				{$t('ui.pages.providersPage.list.allModels')}
			</Button>
			<IconAction
				icon={updating ? 'loader' : 'cloud-download'}
				label={$t('ui.pages.providersPage.header.updateCatalog')}
				variant="outline"
				disabled={updating}
				spin={updating}
				onclick={onupdate}
			/>
			<CardLayoutToggle />
		</div>
	</header>
	<div class="py-4">
		{#if visible.length > 0}
			<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
				{#each visible as card (card.provider.id)}
					<ProviderOverviewCard
						provider={card.provider}
						accounts={card.accounts}
						windows={card.windows}
						layout={consoleState.settings.cardLayout}
						onopen={(id) => onopen(id, 'all')}
						onunpriced={(id) => onopen(id, 'unpriced')}
						onreload={onrefresh}
					/>
				{/each}
				<AddTile
					label={$t('ui.pages.providersPage.header.addProvider')}
					hint={$t('ui.pages.providersPage.list.addTileHint')}
					onclick={onadd}
				/>
			</div>
		{:else}
			<div class="empty-panel section-surface border-dashed">
				<p class="text-sm">{$t('ui.pages.providersPage.list.noMatch')}</p>
				<Button
					variant="outline"
					size="sm"
					onclick={() => {
						query = '';
						filter = 'all';
					}}
				>
					{$t('ui.pages.providersPage.models.clearFilters')}
				</Button>
			</div>
		{/if}
	</div>
</div>
