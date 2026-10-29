<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ModelsDevState, ProviderTemplate } from '$lib/types';
	import Input from '$lib/components/ui/input.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import { ageOf } from '$lib/format';
	import {
		SECTION_LABEL_KEY,
		buildTemplateRows,
		countTemplateRows,
		visibleTemplateRows,
		type TemplateRow
	} from '$lib/provider-sections';
	import { isExperimentalConnection } from '$lib/provider-experimental';
	import BillingChip from '$lib/components/providers/billing-chip.svelte';
	import { Switch } from 'bits-ui';

	// The provider picker is a radio list: choosing marks a row and the
	// footer's Next advances, so a misread row never opens the form by itself.
	interface Props {
		templates: ProviderTemplate[];
		freeEnabled: boolean;
		freeBusy: boolean;
		onfreetoggle: (enabled: boolean) => void;
		modelsdev: ModelsDevState | null;
		added: Set<string>;
		selected?: string;
		query?: string;
		onquery: (value: string) => void;
		onpick: (row: TemplateRow) => void;
		oncustom: () => void;
		onretry: () => void;
	}

	let {
		templates,
		freeEnabled,
		freeBusy,
		onfreetoggle,
		modelsdev,
		added,
		selected = '',
		query = '',
		onquery,
		onpick,
		oncustom,
		onretry
	}: Props = $props();

	const rows = $derived(buildTemplateRows(templates, added, query, freeEnabled));
	const groups = $derived(visibleTemplateRows(rows).filter((group) => group.section !== 'free'));
	const total = $derived(countTemplateRows(rows));

	// The search is the first thing an operator uses, so it opens focused.
	function focusSearch(node: HTMLElement) {
		node.querySelector('input')?.focus();
	}

	function hint(row: TemplateRow): string {
		if (row.hintValue !== '') return row.hintValue;
		if (row.hintKey === '') return '';
		return $t(row.hintKey);
	}

	// A section heading keeps its rounded top corners where it sits in the
	// list, and squares them off the moment it pins against the top of the
	// pane: a rounded corner pinned flush to the edge reads as a gap. The
	// group's top is the heading's resting place, because sticky does not
	// move layout, so a heading pushed below it is stuck. The capture flag
	// catches the pane's scroll, which does not bubble.
	function flattenWhenStuck(node: HTMLElement) {
		const group = node.parentElement;
		// Filtering moves a heading without a scroll, and only the list the
		// heading lives in resizes when that happens.
		const list = group?.parentElement ?? null;
		if (group === null || list === null) return;
		const update = () => {
			const stuck = node.getBoundingClientRect().top - group.getBoundingClientRect().top > 0.5;
			if (stuck) node.setAttribute('data-stuck', 'true');
			else node.removeAttribute('data-stuck');
		};
		const resized = new ResizeObserver(update);
		resized.observe(list);
		const controller = new AbortController();
		const listen = { capture: true, passive: true, signal: controller.signal };
		window.addEventListener('scroll', update, listen);
		window.addEventListener('resize', update, listen);
		update();
		return () => {
			controller.abort();
			resized.disconnect();
		};
	}

	// A key row shows the format it speaks and how many models the catalog
	// knows for it, which is the size hint an operator compares rows by. The
	// count is the catalog's, not the provider's: what a connection serves is
	// whatever its own model list says once it is added.
	function modelCount(row: TemplateRow): string {
		const count = row.template?.modelsdev_models ?? 0;
		if (count === 0) return '';
		return $t('ui.pages.providersPage.add.rowModels', { values: { count } });
	}
</script>

{#snippet templateRow(row: TemplateRow)}
	{@const billingProvider = {
		id: row.id,
		template_id: row.id,
		kind: row.template?.kind ?? '',
		auth: row.template?.auth ?? '',
		origin: row.template?.origin ?? '',
		base_url: row.template?.default_base_url ?? '',
		modelsdev_provider_id: row.template?.modelsdev_provider_id ?? ''
	}}
	{@const chosen = selected === row.id}
	<button
		type="button"
		data-plain
		role="radio"
		aria-checked={chosen}
		disabled={row.disabledReason !== ''}
		class="flex items-center gap-3 rounded-xl bg-control px-4 py-3 text-left shadow-float-1 transition-[background-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {chosen
			? 'bg-accent-soft ring-1 ring-accent-border'
			: row.custom
				? 'border-[1.5px] border-dashed border-line-strong'
				: 'hover:bg-control-hover'}"
		onclick={() => onpick(row)}
	>
		{#if row.custom}
			<span
				class="flex size-9 shrink-0 items-center justify-center rounded-xl bg-sunken text-muted-foreground shadow-float-1"
			>
				<Icon name="plus" size={16} />
			</span>
		{:else}
			<ProviderLogo id={row.id} label={row.label} size="sm" />
		{/if}
		<span class="min-w-0 flex-1">
			<span class="flex items-center gap-2">
				<span class="truncate text-sm font-medium">{row.label}</span>
				{#if isExperimentalConnection(row.id)}
					<span data-experimental-chip class="badge shrink-0 bg-sunken text-muted-foreground">
						{$t('ui.pages.providersPage.list.tagExperimental')}
					</span>
				{/if}
				{#if row.added}
					<span
						class="shrink-0 rounded bg-sunken px-1.5 py-0.5 text-[0.6rem] text-muted-foreground"
					>
						{$t('ui.pages.providersPage.add.added')}
					</span>
				{/if}
			</span>
			<span class="block truncate font-mono text-[0.7rem] text-muted-foreground">
				{hint(row)}{#if modelCount(row)}
					· {modelCount(row)}{/if}
			</span>
			{#if row.disabledReason}
				<span class="block truncate text-[0.7rem] text-warn">{row.disabledReason}</span>
			{/if}
		</span>
		{#if chosen}
			<span
				data-check
				data-checkbox
				data-selected
				data-selected-check
				class="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
				aria-hidden="true"
			>
				<Icon name="check" size={13} />
			</span>
		{/if}
		<BillingChip provider={billingProvider} class="self-start" />
	</button>
{/snippet}

{#snippet groupHeading(label: string, count: number, hint: string)}
	<div
		{@attach flattenWhenStuck}
		class="min-h-11 sticky top-0 z-10 flex flex-wrap items-center gap-2 rounded-lg bg-popover/95 px-2 py-2 backdrop-blur data-[stuck=true]:rounded-t-none"
	>
		<span class="text-[0.9375rem] font-semibold text-muted-foreground">{label}</span>
		<span class="text-[0.7rem] text-muted-foreground">{count}</span>
		<span class="text-[0.7rem] text-muted-foreground">{hint}</span>
	</div>
{/snippet}

<div class="flex flex-col gap-3">
	<div {@attach focusSearch}>
		<Input
			placeholder={$t('ui.pages.providersPage.add.searchPlaceholder', {
				values: { count: templates.length }
			})}
			aria-label={$t('ui.pages.providersPage.add.searchPlaceholder', {
				values: { count: templates.length }
			})}
			value={query}
			oninput={(event) => onquery(event.currentTarget.value)}
		/>
	</div>

	{#if total === 0}
		<div class="flex flex-col items-center gap-2 py-8">
			<p class="text-sm text-muted-foreground">
				{$t('ui.pages.providersPage.add.noMatchTitle', { values: { query: query.trim() } })}
			</p>
			<Button variant="outline" size="sm" onclick={oncustom}>
				<Icon name="settings" size={14} />
				{$t('ui.pages.providersPage.add.customEndpoint')}
			</Button>
		</div>
	{:else}
		<div class="flex flex-col gap-5">
			<div class="flex flex-col gap-1.5">
				<div
					{@attach flattenWhenStuck}
					class="min-h-11 sticky top-0 z-10 flex flex-wrap items-center gap-2 rounded-lg bg-popover/95 px-2 py-2 backdrop-blur data-[stuck=true]:rounded-t-none"
				>
					<span class="text-[0.9375rem] font-semibold text-muted-foreground">
						{$t(SECTION_LABEL_KEY.free)}
					</span>
					{#if freeEnabled}
						<span class="text-[0.7rem] text-muted-foreground">{rows.free.length}</span>
						<span class="text-[0.7rem] text-muted-foreground">
							{$t('ui.pages.providersPage.add.groupHintFree')}
						</span>
					{:else}
						<span class="text-[0.7rem] text-muted-foreground">
							{$t('ui.pages.providersPage.add.freeOffHint')}
						</span>
					{/if}
					<Switch.Root
						class="ms-auto"
						checked={freeEnabled}
						disabled={freeBusy}
						onCheckedChange={(next) => onfreetoggle(next)}
						aria-label={$t(SECTION_LABEL_KEY.free)}
						data-free-toggle
					/>
				</div>
				{#if freeEnabled && rows.free.length > 0}
					<div
						class="grid gap-2 sm:grid-cols-2"
						role="radiogroup"
						aria-label={$t(SECTION_LABEL_KEY.free)}
					>
						{#each rows.free as row (row.id)}
							{@render templateRow(row)}
						{/each}
					</div>
				{/if}
			</div>
			{#each groups as group (group.section)}
				<div class="flex flex-col gap-1.5">
					{@render groupHeading(
						$t(SECTION_LABEL_KEY[group.section]),
						group.rows.length,
						$t(
							'ui.pages.providersPage.add.groupHint' +
								group.section.charAt(0).toUpperCase() +
								group.section.slice(1)
						)
					)}
					<div
						class="grid gap-2 sm:grid-cols-2"
						role="radiogroup"
						aria-label={$t(SECTION_LABEL_KEY[group.section])}
					>
						{#each group.rows as row (row.id)}
							{@render templateRow(row)}
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{/if}

	<div
		class="flex flex-wrap items-center gap-2 rounded-panel bg-sunken px-3 py-2 text-[0.7rem] text-muted-foreground"
	>
		{#if modelsdev?.available}
			<span>
				{$t('ui.pages.providersPage.add.modelsdevLine', {
					values: { age: ageOf(modelsdev.fetched_at_ms) }
				})}
			</span>
		{:else}
			<span>{$t('ui.pages.providersPage.add.modelsdevUnreachable')}</span>
			<Button variant="ghost" size="sm" onclick={onretry}>
				<Icon name="refresh" size={14} />
				{$t('ui.pages.providersPage.add.modelsdevRetry')}
			</Button>
		{/if}
		<span class="ms-auto">
			{$t('ui.pages.providersPage.list.providerCount', { values: { count: total } })}
		</span>
	</div>
</div>
