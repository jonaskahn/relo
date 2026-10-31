<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Snippet } from 'svelte';
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
	import { FREE_TEMPLATE_COUNT, isSingleFreeConnection } from '$lib/provider-free';
	import BillingChip from '$lib/components/providers/billing-chip.svelte';
	import FreeAvailableChip from '$lib/components/providers/free-available-chip.svelte';
	import { Switch } from 'bits-ui';
	import Segmented from '$lib/components/ui/segmented.svelte';

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

	let filter = $state<'all' | 'free'>('all');
	// The switch shows the committed setting. Enabling is only a request until
	// the consent dialog is accepted; dismissing that dialog must not leave the
	// picker in an enabled-looking state.
	const freeSwitchChecked = $derived(freeEnabled);
	const rows = $derived(buildTemplateRows(templates, added, query, freeEnabled, filter === 'free'));
	const groups = $derived(visibleTemplateRows(rows).filter((group) => group.section !== 'free'));
	const total = $derived(countTemplateRows(rows));
	const freeCount = $derived(freeEnabled ? rows.free.length : FREE_TEMPLATE_COUNT);

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

	// A keyless free lane is the one connection shape that can exist only once.
	// The row stays visible and focusable so its tooltip can explain why.
	function singleAdded(row: TemplateRow): boolean {
		return isSingleFreeConnection(row.id) && row.added;
	}

	function rowDisabled(row: TemplateRow): boolean {
		return row.disabledReason !== '' || singleAdded(row);
	}

	function requestFree(next: boolean) {
		// The switch reflects only the committed setting. A press to enable is a
		// consent request; accepting the dialog re-renders this switch checked
		// through the parent's freeEnabled prop.
		if (next !== freeEnabled) onfreetoggle(next);
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
	<div
		class="group relative"
		title={singleAdded(row) ? $t('ui.pages.providersPage.add.freeAlreadyAdded') : ''}
	>
		<button
			type="button"
			data-plain
			role="radio"
			aria-checked={chosen}
			aria-disabled={rowDisabled(row)}
			aria-describedby={singleAdded(row) ? 'free-already-added-' + row.id : undefined}
			class="flex w-full items-center gap-3 rounded-xl bg-control px-4 py-3 text-left shadow-float-1 transition-[background-color,box-shadow] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {chosen
				? 'bg-accent-soft ring-1 ring-accent-border'
				: row.custom
					? 'border-[1.5px] border-dashed border-line-strong'
					: 'hover:bg-control-hover'} {rowDisabled(row) ? 'cursor-not-allowed opacity-70' : ''}"
			onclick={() => {
				if (!rowDisabled(row)) onpick(row);
			}}
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
						<span
							data-experimental-chip
							class="badge shrink-0 bg-warn/10 text-ink ring-1 ring-warn/35"
						>
							{$t('ui.pages.providersPage.list.tagExperimental')}
						</span>
					{/if}
					{#if row.added}
						<span
							class="shrink-0 rounded bg-ok/10 px-1.5 py-0.5 text-[0.6rem] font-semibold text-ink ring-1 ring-ok/35"
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
			<FreeAvailableChip
				provider={{ ...billingProvider, counts: { free_models: row.template?.free_models ?? 0 } }}
				class="self-start"
			/>
		</button>
		{#if singleAdded(row)}
			<span
				id={'free-already-added-' + row.id}
				class="pointer-events-none absolute -top-1 end-3 z-20 hidden rounded-control bg-ink px-2 py-1 text-[0.7rem] text-surface shadow-float-2 group-hover:block group-focus-within:block"
			>
				{$t('ui.pages.providersPage.add.freeAlreadyAdded')}
			</span>
		{/if}
	</div>
{/snippet}

{#snippet freeToggle()}
	<Switch.Root
		checked={freeSwitchChecked}
		disabled={freeBusy}
		onCheckedChange={requestFree}
		aria-label={$t(SECTION_LABEL_KEY.free)}
		data-free-toggle
	/>
{/snippet}

{#snippet groupHeading(
	label: string,
	count: number,
	hint: string,
	free = false,
	trailing?: Snippet
)}
	<div
		{@attach flattenWhenStuck}
		class="free-heading min-h-11 sticky top-0 z-10 flex flex-wrap items-center gap-2 rounded-lg border-b border-line bg-surface px-2 py-2 data-[stuck=true]:rounded-t-none {free
			? 'free-heading-glow'
			: ''}"
	>
		{#if free}
			<svg class="free-glow" aria-hidden="true" focusable="false">
				<rect pathLength="100" rx="7" class="free-glow-blur" />
				<rect pathLength="100" rx="7" class="free-glow-line" />
			</svg>
		{/if}
		<span class="text-[0.9375rem] font-semibold text-ink">{label}</span>
		<span class="text-[0.7rem] text-muted-foreground">{count}</span>
		<span class="text-[0.7rem] text-muted-foreground">{hint}</span>
		{#if trailing}
			<span class="ms-auto">{@render trailing()}</span>
		{/if}
	</div>
{/snippet}

<div class="flex flex-col gap-3">
	<div class="flex items-center gap-2">
		<div class="min-w-0 flex-1" {@attach focusSearch}>
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
		<Segmented
			value={filter}
			options={[
				{ value: 'all', label: $t('ui.pages.providersPage.add.filterAll') },
				{ value: 'free', label: $t('ui.pages.providersPage.add.filterFree') }
			]}
			ariaLabel={$t('ui.pages.providersPage.add.filterLabel')}
			onchange={(next) => (filter = next)}
		/>
	</div>

	{#if total === 0}
		<div class="flex flex-col items-center gap-2 py-8">
			<p class="text-sm text-muted-foreground">
				{$t('ui.pages.providersPage.add.noMatchTitle', { values: { query: query.trim() } })}
			</p>
			{#if filter !== 'free'}
				<Button variant="outline" size="sm" onclick={oncustom}>
					<Icon name="settings" size={14} />
					{$t('ui.pages.providersPage.add.customEndpoint')}
				</Button>
			{/if}
		</div>
	{:else}
		<div class="flex flex-col gap-5">
			<div class="flex flex-col gap-1.5">
				{@render groupHeading(
					$t(SECTION_LABEL_KEY.free),
					freeCount,
					freeEnabled
						? $t('ui.pages.providersPage.add.groupHintFree')
						: $t('ui.pages.providersPage.add.freeOffHint'),
					true,
					freeToggle
				)}
				{#if freeEnabled && rows.free.length > 0}
					<div
						class="grid grid-cols-1 gap-2 sm:grid-cols-2"
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
						class="grid grid-cols-1 gap-2 sm:grid-cols-2"
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

<style>
	/* A short accent dash traces the free heading border once when the picker
	   opens, then fades out. The normalized pathLength lets the dash length stay
	   proportional to the heading width. Reduced motion keeps the full accent
	   border static instead of animating it. */
	.free-heading-glow {
		position: relative;
		isolation: isolate;
		--free-glow-line-length: 18;
	}
	.free-glow {
		pointer-events: none;
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		overflow: visible;
	}
	.free-glow rect {
		x: 1px;
		y: 1px;
		width: calc(100% - 2px);
		height: calc(100% - 2px);
		fill: none;
		stroke-linecap: round;
		stroke-dasharray: var(--free-glow-line-length) calc(100 - var(--free-glow-line-length));
		opacity: 0;
		animation: free-glow-trace 2.6s ease-in-out 1 forwards;
	}
	.free-glow-line {
		stroke: var(--accent);
		stroke-width: 1.5px;
	}
	.free-glow-blur {
		stroke: var(--accent-glow);
		stroke-width: 5px;
		filter: blur(3px);
	}
	@keyframes free-glow-trace {
		0% {
			stroke-dashoffset: 0;
			opacity: 0;
		}
		10%,
		85% {
			opacity: 1;
		}
		to {
			stroke-dashoffset: -100;
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.free-glow rect {
			animation: none;
			stroke-dasharray: none;
			stroke: var(--accent-border);
			opacity: 1;
		}
		.free-glow-blur {
			display: none;
		}
	}
</style>
