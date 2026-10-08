<script lang="ts">
	import { t } from 'svelte-i18n';
	import { DropdownMenu } from 'bits-ui';
	import MenuPanel from '$lib/components/ui/menu-panel.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import {
		EMPTY_MODEL_FILTERS,
		MODEL_CATEGORY_OPTIONS,
		modelCounts,
		type ModelCategoryOption,
		type ModelFilters,
		type ModelLabelFilter,
		type ModelPricingFilter,
		type ModelStatusFilter
	} from '$lib/model-filters';
	import type { Model } from '$lib/types';

	// ModelFilterMenu is the one filter button beside model search: category,
	// status, label and (on routers that name both) pricing live as sections
	// of a single panel instead of four icon menus. The badge names how many
	// dimensions are narrowed.
	interface Props {
		filters?: ModelFilters;
		onfilters?: (next: ModelFilters) => void;
		models: Model[];
		paidFree?: boolean;
	}

	let {
		filters = { ...EMPTY_MODEL_FILTERS },
		onfilters,
		models,
		paidFree = false
	}: Props = $props();

	const counts = $derived(modelCounts(models));
	const categoryLabels = $derived<Record<ModelCategoryOption, string>>({
		reasoning: $t('ui.pages.providersPage.modelsTable.columnReasoning'),
		tools: $t('ui.pages.providersPage.modelsTable.columnTools'),
		vision: $t('ui.pages.providersPage.modelsTable.columnVision')
	});
	const activeDims = $derived(
		(filters.category !== '' ? 1 : 0) +
			(filters.status !== 'all' ? 1 : 0) +
			(filters.label !== 'all' ? 1 : 0) +
			(filters.pricing !== 'all' ? 1 : 0)
	);

	const statusOptions = $derived([
		{ value: 'all', label: $t('ui.pages.providersPage.modelsTable.segmentAll'), icon: 'list' },
		{
			value: 'on',
			label: $t('ui.pages.providersPage.modelsTable.segmentOn'),
			icon: 'circle-check'
		},
		{ value: 'off', label: $t('ui.pages.providersPage.modelsTable.segmentOff'), icon: 'circle-off' }
	] satisfies { value: ModelStatusFilter; label: string; icon: IconName }[]);

	const labelOptions = $derived([
		{ value: 'all', label: $t('ui.pages.providersPage.modelsTable.segmentAll') },
		{ value: 'unpriced', label: $t('ui.pages.providersPage.models.chipUnpriced') },
		{ value: 'overridden', label: $t('ui.pages.providersPage.models.chipOverridden') },
		{ value: 'unavailable', label: $t('ui.pages.providersPage.models.chipUnavailable') }
	] satisfies { value: ModelLabelFilter; label: string }[]);

	const pricingOptions = $derived([
		{ value: 'all', label: $t('ui.pages.providersPage.modelsTable.segmentAll') },
		{ value: 'paid', label: $t('ui.pages.providersPage.models.pricingPaid') },
		{ value: 'free', label: $t('ui.pages.providersPage.models.pricingFree') }
	] satisfies { value: ModelPricingFilter; label: string }[]);

	function categoryCount(option: '' | ModelCategoryOption): number {
		if (option === '') return counts.all;
		let n = 0;
		for (const model of models) {
			if (model.capabilities?.[option] === true) n++;
		}
		return n;
	}

	function labelCount(value: ModelLabelFilter): number {
		return value === 'all' ? counts.all : counts[value];
	}

	function pricingCount(value: ModelPricingFilter): number {
		return value === 'all' ? counts.all : counts[value];
	}

	const itemClass =
		'flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none data-highlighted:bg-hover';
	const sectionLabel =
		'px-2 pt-2 pb-0.5 text-[0.7rem] font-semibold tracking-wide text-muted-foreground';
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="relative inline-flex size-control shrink-0 items-center justify-center rounded-md border border-border text-muted-foreground hover:border-border-strong hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:border-border-strong data-[state=open]:text-foreground {activeDims >
		0
			? 'border-accent-line bg-accent-soft text-accent-ink'
			: ''}"
		aria-label={$t('ui.pages.providersPage.models.toolbarFilters')}
		title={$t('ui.pages.providersPage.models.toolbarFilters')}
	>
		<Icon name="filter" size={16} />
		{#if activeDims > 0}
			<span
				class="absolute -top-1.5 -end-1.5 flex size-4 items-center justify-center rounded-full bg-primary font-mono text-[0.6rem] font-semibold text-primary-foreground"
				aria-hidden="true"
			>
				{activeDims}
			</span>
		{/if}
	</DropdownMenu.Trigger>
	<MenuPanel class="z-50 max-h-80 min-w-52 overflow-y-auto surface-pop p-1" align="start">
		<p class={sectionLabel}>{$t('ui.pages.providersPage.models.category')}</p>
		<DropdownMenu.Item class={itemClass} onSelect={() => onfilters?.({ ...filters, category: '' })}>
			<span class="flex size-4 shrink-0 items-center justify-center text-muted-foreground">
				{#if filters.category === ''}<Icon name="check" size={14} />{/if}
			</span>
			<span class="flex-1">{$t('ui.pages.providersPage.models.allCategories')}</span>
			<span class="font-mono text-[0.65rem] text-muted-foreground">{categoryCount('')}</span>
		</DropdownMenu.Item>
		{#each MODEL_CATEGORY_OPTIONS as option (option)}
			<DropdownMenu.Item
				class={itemClass}
				onSelect={() => onfilters?.({ ...filters, category: option })}
			>
				<span class="flex size-4 shrink-0 items-center justify-center text-muted-foreground">
					{#if filters.category === option}<Icon name="check" size={14} />{/if}
				</span>
				<span class="flex-1 min-w-0 truncate">{categoryLabels[option]}</span>
				<span class="font-mono text-[0.65rem] text-muted-foreground">{categoryCount(option)}</span>
			</DropdownMenu.Item>
		{/each}
		<DropdownMenu.Separator class="my-1 h-px bg-border" />
		<p class={sectionLabel}>{$t('ui.pages.providersPage.modelsTable.status')}</p>
		{#each statusOptions as option (option.value)}
			<DropdownMenu.Item
				class={itemClass}
				onSelect={() => onfilters?.({ ...filters, status: option.value })}
			>
				<span class="flex size-4 shrink-0 items-center justify-center text-muted-foreground">
					{#if filters.status === option.value}<Icon name="check" size={14} />{/if}
				</span>
				<Icon name={option.icon} size={14} />
				<span class="flex-1">{option.label}</span>
				<span class="font-mono text-[0.65rem] text-muted-foreground">{counts[option.value]}</span>
			</DropdownMenu.Item>
		{/each}
		<DropdownMenu.Separator class="my-1 h-px bg-border" />
		<p class={sectionLabel}>{$t('ui.pages.providersPage.models.filterLabels')}</p>
		{#each labelOptions as option (option.value)}
			<DropdownMenu.Item
				class={itemClass}
				onSelect={() => onfilters?.({ ...filters, label: option.value })}
			>
				<span class="flex size-4 shrink-0 items-center justify-center text-muted-foreground">
					{#if filters.label === option.value}<Icon name="check" size={14} />{/if}
				</span>
				<span class="flex-1">{option.label}</span>
				<span class="font-mono text-[0.65rem] text-muted-foreground"
					>{labelCount(option.value)}</span
				>
			</DropdownMenu.Item>
		{/each}
		{#if paidFree}
			<DropdownMenu.Separator class="my-1 h-px bg-border" />
			<p class={sectionLabel}>{$t('ui.pages.providersPage.models.filterPricing')}</p>
			{#each pricingOptions as option (option.value)}
				<DropdownMenu.Item
					class={itemClass}
					onSelect={() => onfilters?.({ ...filters, pricing: option.value })}
				>
					<span class="flex size-4 shrink-0 items-center justify-center text-muted-foreground">
						{#if filters.pricing === option.value}<Icon name="check" size={14} />{/if}
					</span>
					<span class="flex-1">{option.label}</span>
					<span class="font-mono text-[0.65rem] text-muted-foreground"
						>{pricingCount(option.value)}</span
					>
				</DropdownMenu.Item>
			{/each}
		{/if}
		{#if activeDims > 0}
			<DropdownMenu.Separator class="my-1 h-px bg-border" />
			<DropdownMenu.Item
				class={itemClass}
				onSelect={() => onfilters?.({ ...EMPTY_MODEL_FILTERS, search: filters.search })}
			>
				<Icon name="x" size={14} />
				<span class="flex-1">{$t('ui.pages.providersPage.models.clearFilters')}</span>
			</DropdownMenu.Item>
		{/if}
	</MenuPanel>
</DropdownMenu.Root>
