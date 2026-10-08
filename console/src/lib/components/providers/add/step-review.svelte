<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ProbeResult } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Label from '$lib/components/ui/label.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { formatUnitPrice } from '$lib/format';

	// Review is the last look before the write: what will be stored on the
	// left, and the models the connection will serve on the right, each one
	// switchable before it is added.
	interface Props {
		probe: ProbeResult | null;
		providerId?: string;
		label?: string;
		disabled?: string[];
		idProblem?: string;
		onchange: (patch: { providerId?: string; label?: string; disabled?: string[] }) => void;
		isAddKey?: boolean;
		targetLabel?: string;
	}

	let {
		probe,
		providerId = '',
		label = '',
		disabled = [],
		idProblem = '',
		onchange,
		isAddKey = false,
		targetLabel = ''
	}: Props = $props();

	let search = $state('');

	const sourceKeys: Record<string, string> = {
		listing: 'ui.pages.providersPage.review.sourceListing',
		manual: 'ui.pages.providersPage.review.sourceManual'
	};

	const models = $derived(
		(probe?.models ?? []).filter((model) =>
			search.trim() === ''
				? true
				: (model.id + ' ' + (model.name ?? '')).toLowerCase().includes(search.trim().toLowerCase())
		)
	);

	const counts = $derived(
		probe?.counts ?? { listed: 0, matched: 0, priced: 0, from_listing: 0, from_manual: 0 }
	);

	function toggle(id: string, on: boolean) {
		onchange({ disabled: on ? disabled.filter((entry) => entry !== id) : [...disabled, id] });
	}
</script>

<div class="grid gap-4 lg:grid-cols-[minmax(0,5fr)_minmax(0,6fr)]">
	<div class="flex h-fit flex-col gap-3 rounded-panel bg-sunken p-4">
		{#if isAddKey}
			<p class="text-sm text-muted-foreground">
				{$t('ui.pages.providersPage.review.keyAdded', { values: { label: targetLabel } })}
			</p>
		{:else}
			<div class="flex flex-col gap-1">
				<Label for="review-id">{$t('ui.pages.providersPage.review.providerId')}</Label>
				<Input
					id="review-id"
					class="font-mono text-xs"
					value={providerId}
					oninput={(event) => onchange({ providerId: event.currentTarget.value })}
				/>
				{#if idProblem}
					<p class="text-[0.7rem] text-destructive" role="alert">{idProblem}</p>
				{/if}
			</div>
			<div class="flex flex-col gap-1">
				<Label for="review-label">{$t('ui.pages.providersPage.review.label')}</Label>
				<Input
					id="review-label"
					value={label}
					oninput={(event) => onchange({ label: event.currentTarget.value })}
				/>
			</div>
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.providersPage.review.summary', {
					values: {
						listing: counts.from_listing,
						manual: counts.from_manual,
						priced: counts.priced
					}
				})}
			</p>
		{/if}
	</div>

	<div class="flex min-h-0 flex-col gap-3">
		<div class="flex flex-wrap items-center gap-2">
			<Input
				class="max-w-xs"
				placeholder={$t('ui.pages.providersPage.review.search')}
				aria-label={$t('ui.pages.providersPage.review.search')}
				bind:value={search}
			/>
			<Button variant="outline" size="sm" onclick={() => onchange({ disabled: [] })}>
				<Icon name="check" size={14} />
				{$t('ui.pages.providersPage.review.turnAllOn')}
			</Button>
			<Button
				variant="outline"
				size="sm"
				onclick={() => onchange({ disabled: (probe?.models ?? []).map((model) => model.id) })}
			>
				<Icon name="circle-off" size={14} />
				{$t('ui.pages.providersPage.review.turnAllOff')}
			</Button>
		</div>

		<p class="text-xs text-muted-foreground">{$t('ui.pages.providersPage.review.legend')}</p>

		<div class="flex max-h-80 flex-col gap-2 overflow-y-auto pe-1">
			{#if models.length === 0}
				<p
					class="rounded-panel border border-dashed border-border px-3 py-6 text-center text-sm text-muted-foreground"
				>
					{$t('ui.pages.providersPage.review.modelsEmpty')}
				</p>
			{:else}
				{#each models.slice(0, 200) as model (model.id)}
					<label class="section-surface flat-surface flex items-center gap-2.5 px-3 py-2 text-sm">
						<input
							type="checkbox"
							class="size-4 shrink-0"
							checked={!disabled.includes(model.id)}
							onchange={(event) => toggle(model.id, event.currentTarget.checked)}
						/>
						<span class="min-w-0 flex-1">
							<span class="block truncate text-sm">{model.name || model.id}</span>
							<span class="block truncate font-mono text-[0.7rem] text-muted-foreground"
								>{model.id}</span
							>
						</span>
						<span class="shrink-0 font-mono text-[0.7rem] text-muted-foreground">
							{formatUnitPrice(model.prices.effective.input)} / {formatUnitPrice(
								model.prices.effective.output
							)}
						</span>
						{#if !model.enabled}
							<span class="badge shrink-0 border border-warn/40 text-warn">
								{$t('ui.pages.providersPage.models.tagDeprecated')}
							</span>
						{/if}
						{#if model.prices.effective.input === null && model.prices.effective.output === null}
							<span class="badge shrink-0 border border-warn/40 text-warn">
								{$t('ui.pages.providersPage.models.tagUnpriced')}
							</span>
						{/if}
						<span class="badge shrink-0 border border-border text-muted-foreground">
							{$t(sourceKeys[model.source] ?? 'ui.pages.providersPage.review.sourceManual')}
						</span>
					</label>
				{/each}
			{/if}
		</div>
	</div>
</div>
