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

	// The provider picker is a radio list: choosing marks a row and the
	// footer's Next advances, so a misread row never opens the form by itself.
	interface Props {
		templates: ProviderTemplate[];
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
		modelsdev,
		added,
		selected = '',
		query = '',
		onquery,
		onpick,
		oncustom,
		onretry
	}: Props = $props();

	const rows = $derived(buildTemplateRows(templates, added, query));
	const groups = $derived(visibleTemplateRows(rows));
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

	{#if groups.length === 0}
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
			{#each groups as group (group.section)}
				<div class="flex flex-col gap-1.5">
					<div
						class="sticky top-0 z-10 flex flex-wrap items-baseline gap-2 bg-popover/95 py-1 backdrop-blur"
					>
						<span class="text-[0.8125rem] font-semibold text-muted-foreground">
							{$t(SECTION_LABEL_KEY[group.section])}
						</span>
						<span class="text-[0.7rem] text-muted-foreground">{group.rows.length}</span>
						<span class="text-[0.7rem] text-muted-foreground">
							{$t(
								'ui.pages.providersPage.add.groupHint' +
									group.section.charAt(0).toUpperCase() +
									group.section.slice(1)
							)}
						</span>
					</div>
					<div
						class="grid gap-2 sm:grid-cols-2"
						role="radiogroup"
						aria-label={$t(SECTION_LABEL_KEY[group.section])}
					>
						{#each group.rows as row (row.id)}
							{@const chosen = selected === row.id}
							<button
								type="button"
								data-plain
								role="radio"
								aria-checked={chosen}
								disabled={row.disabledReason !== ''}
								class="flex items-center gap-3 rounded-xl border px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60 {chosen
									? 'border-accent bg-accent-soft'
									: row.custom
										? 'border-dashed border-border hover:border-border-strong'
										: 'border-border hover:border-border-strong'}"
								onclick={() => onpick(row)}
							>
								{#if row.custom}
									<span
										class="flex size-9 shrink-0 items-center justify-center rounded-xl border border-border text-muted-foreground"
									>
										<Icon name="plus" size={16} />
									</span>
								{:else}
									<ProviderLogo id={row.id} label={row.label} size="sm" />
								{/if}
								<span class="min-w-0 flex-1">
									<span class="flex items-center gap-2">
										<span class="truncate text-sm font-medium">{row.label}</span>
										{#if row.added}
											<span
												class="shrink-0 rounded border border-border px-1.5 py-0.5 text-[0.6rem] text-muted-foreground"
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
										<span class="block truncate text-[0.7rem] text-warn">
											{row.disabledReason}
										</span>
									{/if}
								</span>
								{#if chosen}
									<span
										class="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary text-primary-foreground"
										aria-hidden="true"
									>
										<Icon name="check" size={13} />
									</span>
								{/if}
							</button>
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
