<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api';
	import { modelPrice, providerLabel } from '$lib/catalog';
	import { allowedByLanes } from '$lib/group-suggestions';
	import type { Lane } from '$lib/group-suggestions';
	import BillingChip from '$lib/components/providers/billing-chip.svelte';
	import type { GroupMember, Model, Provider } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Select from '$lib/components/ui/select.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';

	interface Props {
		configured: Provider[];
		providers: Provider[];
		members: GroupMember[];
		lanes?: Lane[];
		onadd: (member: GroupMember) => void;
		onlearn: (models: Model[]) => void;
	}

	let { configured, providers, members, lanes = [], onadd, onlearn }: Props = $props();

	// A page of rows: the list paints this many and asks for the next page as
	// its foot scrolls into view, so a large catalogue never paints all at once.
	const PAGE = 40;

	// The picker reads models on its own, so a route being built never waits
	// on a connection it has not added a member from yet.
	let provider = $state('');
	let search = $state('');
	let onOnly = $state(true);
	let models = $state<Model[]>([]);
	let shown = $state(PAGE);
	let pane = $state<HTMLElement | null>(null);
	let loading = $state(false);
	let failed = $state('');
	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	let requestSeq = 0;
	let windowKey = '';

	const providerMap = $derived(new Map(providers.map((provider) => [provider.id, provider])));
	const visible = $derived(
		(onOnly ? models.filter((model) => model.enabled) : models).filter((model) => {
			const provider = providerMap.get(model.provider_id);
			return provider ? allowedByLanes(provider, lanes) : true;
		})
	);
	const windowed = $derived(visible.slice(0, shown));
	const hasMore = $derived(shown < visible.length);

	// A new connection, search, lane set, or read starts the list at its first
	// page and back at the top: a window grown on the old filter would open the
	// new one part-way down.
	const filterKey = $derived([provider, search, onOnly, lanes.join(','), models.length].join('|'));
	$effect(() => {
		if (filterKey === windowKey) return;
		windowKey = filterKey;
		shown = PAGE;
		pane?.scrollTo({ top: 0 });
	});

	// The sentinel asks for the next page while the foot stays in view, so a
	// pane taller than one page fills itself. Its root is the pane, not the
	// window, because the list is what scrolls.
	function watchMore(node: HTMLElement) {
		$effect(() => {
			void shown;
			if (!hasMore || pane === null) return;
			const observer = new IntersectionObserver(
				(entries) => {
					if (entries.some((entry) => entry.isIntersecting)) more();
				},
				{ root: pane, rootMargin: '0px 0px 120px 0px' }
			);
			observer.observe(node);
			return () => observer.disconnect();
		});
	}

	function more() {
		shown = Math.min(shown + PAGE, visible.length);
	}

	onMount(() => void load());
	onDestroy(() => {
		clearTimeout(searchTimer);
		requestSeq++;
	});

	function isMember(model: Model): boolean {
		return members.some(
			(member) => member.provider_id === model.provider_id && member.model_id === model.model_id
		);
	}

	function add(model: Model) {
		if (isMember(model)) return;
		onadd({
			provider_id: model.provider_id,
			model_id: model.model_id,
			kind: 'model',
			weight: 1,
			enabled: true,
			eligible: model.routable,
			serving_accounts: model.serving_accounts ?? 0,
			active_accounts: model.active_accounts ?? 0
		});
	}

	async function load() {
		const seq = ++requestSeq;
		loading = true;
		failed = '';
		try {
			// limit=0 is the daemon's "no page cap". The lane filter runs here,
			// after the read, so a capped page would drop whole connections.
			const params = new URLSearchParams({ limit: '0' });
			if (provider !== '') params.set('provider', provider);
			if (search.trim() !== '') params.set('q', search.trim());
			const data = await api<{ items: Model[] }>('/models?' + params);
			if (seq !== requestSeq) return;
			models = data.items ?? [];
			onlearn(models);
		} catch (error) {
			if (seq !== requestSeq) return;
			failed = error instanceof Error ? error.message : String(error);
		} finally {
			if (seq === requestSeq) loading = false;
		}
	}

	function onSearchInput() {
		clearTimeout(searchTimer);
		searchTimer = setTimeout(() => void load(), 250);
	}
</script>

<span class="field-label shrink-0">{$t('ui.pages.groupsPage.addModels')}</span>
<div class="flex shrink-0 flex-wrap items-center gap-2">
	<Select
		class="min-w-40 flex-1"
		ariaLabel={$t('ui.pages.groupsPage.pickProvider')}
		bind:value={provider}
		onValueChange={() => void load()}
		options={[
			{ value: '', label: $t('ui.pages.groupsPage.allConnections') },
			...configured.map((connection) => ({ value: connection.id, label: connection.label }))
		]}
	/>
	<Button variant="chip" size="sm" aria-pressed={onOnly} onclick={() => (onOnly = !onOnly)}>
		{$t('ui.pages.groupsPage.onOnly')}
	</Button>
</div>
<Input
	id="route-model-search"
	class="shrink-0"
	placeholder={$t('ui.pages.groupsPage.searchModels')}
	aria-label={$t('ui.pages.groupsPage.searchModels')}
	bind:value={search}
	oninput={onSearchInput}
/>
<div
	class="flex shrink-0 items-center justify-between gap-2 px-2.5 text-[0.7rem] text-muted-foreground"
>
	<span>{$t('ui.common.model')}</span>
	<span>{$t('ui.pages.groupsPage.priceLegend')}</span>
</div>
<div bind:this={pane} class="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
	{#if loading}
		<p class="text-sm text-muted-foreground">{$t('ui.common.loading')}</p>
	{:else if failed}
		<p class="text-sm text-destructive" role="alert">{failed}</p>
	{:else if visible.length === 0}
		<p
			class="rounded-panel border-[1.5px] border-dashed border-accent-line bg-card p-4 text-center text-sm text-muted-foreground"
		>
			{$t('ui.pages.groupsPage.noMatchingModels')}
		</p>
	{:else}
		{#each windowed as model (model.provider_id + '/' + model.model_id)}
			<div class="flex items-center justify-between gap-2 rounded-lg px-2.5 py-1.5 hover:bg-card">
				<div class="flex min-w-0 items-center gap-2.5">
					<ProviderLogo
						id={model.provider_id}
						label={providerLabel(providers, model.provider_id)}
						size="xs"
					/>
					<div class="min-w-0">
						<span class="block truncate text-sm">{model.name || model.model_id}</span>
						<span class="mono-data block truncate text-muted-foreground"
							>{model.provider_id}/{model.model_id}</span
						>
					</div>
				</div>
				<div class="flex shrink-0 items-center gap-2">
					{#if providerMap.get(model.provider_id)}
						<BillingChip provider={providerMap.get(model.provider_id)!} />
					{/if}
					<span class="mono-data text-muted-foreground">{modelPrice(model)}</span>
					<Button
						variant={isMember(model) ? 'secondary' : 'outline'}
						size="sm"
						disabled={isMember(model)}
						onclick={() => add(model)}
					>
						<Icon name={isMember(model) ? 'check' : 'plus'} size={14} />
						{isMember(model) ? $t('ui.pages.groupsPage.added') : $t('ui.pages.groupsPage.add')}
					</Button>
				</div>
			</div>
		{/each}
		{#if hasMore}
			<div use:watchMore class="flex shrink-0 justify-center py-1">
				<Button variant="ghost" size="sm" onclick={more}>
					{$t('ui.pages.groupsPage.scrollForMore', {
						values: { shown: windowed.length, total: visible.length }
					})}
				</Button>
			</div>
		{/if}
	{/if}
</div>
