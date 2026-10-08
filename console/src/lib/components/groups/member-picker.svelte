<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api';
	import { modelPrice, providerLabel } from '$lib/catalog';
	import type { GroupMember, Model, Provider } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';

	interface Props {
		configured: Provider[];
		providers: Provider[];
		members: GroupMember[];
		onadd: (member: GroupMember) => void;
		onlearn: (models: Model[]) => void;
	}

	let { configured, providers, members, onadd, onlearn }: Props = $props();

	// The picker reads models on its own, so a route being built never waits
	// on a connection it has not added a member from yet.
	let provider = $state('');
	let search = $state('');
	let onOnly = $state(true);
	let models = $state<Model[]>([]);
	let loading = $state(false);
	let failed = $state('');
	let searchTimer: ReturnType<typeof setTimeout> | undefined;
	let requestSeq = 0;

	const visible = $derived(onOnly ? models.filter((model) => model.enabled) : models);

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
			const params = new URLSearchParams({ limit: '100' });
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
	<NativeSelect
		class="min-w-40 flex-1"
		aria-label={$t('ui.pages.groupsPage.pickProvider')}
		value={provider}
		onchange={(event) => {
			provider = event.currentTarget.value;
			void load();
		}}
	>
		<option value="">{$t('ui.pages.groupsPage.allConnections')}</option>
		{#each configured as connection (connection.id)}
			<option value={connection.id}>{connection.label}</option>
		{/each}
	</NativeSelect>
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
<div class="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
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
		{#each visible.slice(0, 100) as model (model.provider_id + '/' + model.model_id)}
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
	{/if}
</div>
