<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api';
	import type { ModelsDevSearchHit, ModelsDevSearchResult, Model } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { formatUnitPrice } from '$lib/format';

	interface Props {
		model: Model;
		onchange: (updated: Model) => void;
	}

	let { model, onchange }: Props = $props();

	let query = $state('');
	let hits = $state<ModelsDevSearchHit[]>([]);
	let working = $state(false);
	let failed = $state('');

	// statusKey names how the model is priced today, which is the one line
	// that tells an operator whether they chose the match.
	const statusKey = $derived.by(() => {
		if (model.match === 'manual' && model.modelsdev_ref !== '') {
			return 'ui.pages.providersPage.models.pricedAsManual';
		}
		if (model.modelsdev_ref !== '') return 'ui.pages.providersPage.models.pricedAsAuto';
		return 'ui.pages.providersPage.models.pricedAsNone';
	});

	async function search() {
		working = true;
		failed = '';
		try {
			const data = await api<ModelsDevSearchResult>(
				'/modelsdev/models?limit=20&q=' + encodeURIComponent(query.trim())
			);
			hits = data.items ?? [];
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		} finally {
			working = false;
		}
	}

	// Choosing a reference re-prices the model straight away, because the
	// daemon rewrites the models.dev layer as it saves.
	async function choose(ref: string) {
		working = true;
		failed = '';
		try {
			const updated = await api<Model>(
				'/models/' +
					encodeURIComponent(model.provider_id) +
					'/' +
					encodeURIComponent(model.model_id),
				{ method: 'PATCH', body: JSON.stringify({ priced_as: ref }) }
			);
			onchange(updated);
			query = '';
			hits = [];
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		} finally {
			working = false;
		}
	}
</script>

<div class="flex flex-col gap-2 rounded-lg border border-border p-3">
	<div class="flex flex-wrap items-center gap-2 text-xs">
		<Icon name="tag" size={14} class="text-muted-foreground" />
		<span class="text-muted-foreground">{$t('ui.pages.providersPage.models.pricedAs')}</span>
		<span class="font-mono">
			{$t(statusKey, { values: { ref: model.modelsdev_ref } })}
		</span>
	</div>

	<div class="flex flex-wrap items-center gap-2">
		<Input
			class="max-w-xs font-mono text-xs"
			placeholder={$t('ui.pages.providersPage.models.pricedAsSearch')}
			aria-label={$t('ui.pages.providersPage.models.pricedAsSearch')}
			bind:value={query}
			onkeydown={(event) => {
				if (event.key === 'Enter') void search();
			}}
		/>
		<Button variant="outline" size="sm" disabled={working} onclick={search}>
			<Icon name="search" size={14} />
			{$t('ui.pages.providersPage.models.pricedAsSearch')}
		</Button>
		{#if model.match === 'manual'}
			<Button variant="ghost" size="sm" disabled={working} onclick={() => choose('')}>
				<Icon name="sparkles" size={14} />
				{$t('ui.pages.providersPage.models.useAutomatic')}
			</Button>
		{/if}
	</div>

	{#if failed}
		<p class="text-xs text-destructive">
			{$t('ui.pages.providersPage.models.pricedAsFailed', { values: { detail: failed } })}
		</p>
	{/if}

	{#if hits.length > 0}
		<div class="max-h-56 overflow-y-auto rounded-md border border-border">
			{#each hits as hit (hit.ref)}
				<button
					type="button"
					data-plain
					class="flex w-full items-center gap-3 px-2.5 py-1.5 text-left text-xs hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
					onclick={() => choose(hit.ref)}
				>
					<span class="min-w-0 flex-1">
						<span class="block truncate font-mono">{hit.ref}</span>
						<span class="block truncate text-muted-foreground">{hit.name || hit.model_id}</span>
					</span>
					<span class="shrink-0 font-mono text-muted-foreground">
						{formatUnitPrice(hit.prices.input)} / {formatUnitPrice(hit.prices.output)}
					</span>
				</button>
			{/each}
		</div>
	{:else if query.trim() !== '' && !working}
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.providersPage.models.pricedAsNoResults', { values: { query: query.trim() } })}
		</p>
	{/if}
</div>
