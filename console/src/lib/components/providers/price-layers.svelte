<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ModelPrices, Prices } from '$lib/types';
	import { formatUnitPrice } from '$lib/format';
	import { Switch } from 'bits-ui';
	import { PRICE_TAG_KEY, priceTag } from '$lib/model-filters';

	interface Props {
		prices: ModelPrices;
	}

	let { prices }: Props = $props();

	let longContext = $state(false);

	// The four rows a price table shows, plus the long-context rates that only
	// apply above a threshold and stay folded away until asked for.
	const rows: { key: keyof Prices; labelKey: string }[] = [
		{ key: 'input', labelKey: 'ui.pages.providersPage.models.input' },
		{ key: 'output', labelKey: 'ui.pages.providersPage.models.output' },
		{ key: 'cache_read', labelKey: 'ui.pages.providersPage.models.cacheRead' },
		{ key: 'cache_write', labelKey: 'ui.pages.providersPage.models.cacheWrite' }
	];

	const extended: { key: keyof Prices; labelKey: string }[] = [
		{ key: 'ext_input', labelKey: 'ui.pages.providersPage.models.input' },
		{ key: 'ext_output', labelKey: 'ui.pages.providersPage.models.output' },
		{ key: 'ext_cache_read', labelKey: 'ui.pages.providersPage.models.cacheRead' },
		{ key: 'ext_cache_write', labelKey: 'ui.pages.providersPage.models.cacheWrite' }
	];

	const tag = $derived(priceTag(prices));
</script>

<div class="flex flex-col gap-2">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<span class="text-xs font-medium">{$t('ui.pages.providersPage.models.priceTable')}</span>
		<span class="inline-flex items-center gap-1.5 text-[0.7rem] text-muted-foreground">
			{$t('ui.pages.providersPage.models.columnEffective')}:
			<span class="rounded border border-border px-1.5 py-0.5 font-mono"
				>{$t(PRICE_TAG_KEY[tag])}</span
			>
		</span>
	</div>

	<div class="overflow-x-auto">
		<table class="w-full min-w-[34rem] border-collapse text-xs">
			<thead>
				<tr class="text-left text-muted-foreground">
					<th class="w-28 py-1 font-normal"></th>
					<th class="py-1 pe-3 font-normal">{$t('ui.pages.providersPage.models.columnProvider')}</th
					>
					<th class="py-1 pe-3 font-normal"
						>{$t('ui.pages.providersPage.models.columnModelsDev')}</th
					>
					<th class="py-1 pe-3 font-normal">{$t('ui.pages.providersPage.models.columnOverride')}</th
					>
					<th class="py-1 font-normal">{$t('ui.pages.providersPage.models.columnEffective')}</th>
				</tr>
			</thead>
			<tbody class="font-mono">
				{#each rows as row (row.key)}
					<tr class="border-t border-border/60">
						<td class="py-1.5 font-sans text-muted-foreground">{$t(row.labelKey)}</td>
						<td class="py-1.5 pe-3">{formatUnitPrice(prices.provider[row.key])}</td>
						<td class="py-1.5 pe-3">{formatUnitPrice(prices.modelsdev[row.key])}</td>
						<td class="py-1.5 pe-3">{formatUnitPrice(prices.override[row.key])}</td>
						<td class="py-1.5">{formatUnitPrice(prices.effective[row.key])}</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<div class="flex flex-col gap-2">
		<label class="flex items-center gap-2 text-xs text-muted-foreground">
			<Switch.Root
				checked={longContext}
				onCheckedChange={(value: boolean) => (longContext = value)}
				aria-label={$t('ui.pages.providersPage.models.longContext')}
			/>
			{$t('ui.pages.providersPage.models.longContext')}
			<span class="font-mono">
				{$t('ui.pages.providersPage.models.threshold')}
				{formatUnitPrice(prices.effective.ext_threshold)}
			</span>
		</label>
		{#if longContext}
			<div class="overflow-x-auto">
				<table class="w-full min-w-[34rem] border-collapse text-xs">
					<tbody class="font-mono">
						{#each extended as row (row.key)}
							<tr class="border-t border-border/60">
								<td class="w-28 py-1.5 font-sans text-muted-foreground">{$t(row.labelKey)}</td>
								<td class="py-1.5 pe-3">{formatUnitPrice(prices.provider[row.key])}</td>
								<td class="py-1.5 pe-3">{formatUnitPrice(prices.modelsdev[row.key])}</td>
								<td class="py-1.5 pe-3">{formatUnitPrice(prices.override[row.key])}</td>
								<td class="py-1.5">{formatUnitPrice(prices.effective[row.key])}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
		<p class="text-[0.7rem] text-muted-foreground">
			{$t('ui.pages.providersPage.models.perMillion')}
		</p>
	</div>
</div>
