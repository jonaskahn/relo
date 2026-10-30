<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import type { Model } from '$lib/types';
	import { api } from '$lib/api';
	import Button from '$lib/components/ui/button.svelte';
	import { buttonVariants } from '$lib/components/ui/button.svelte';
	import { cn } from '$lib/utils';
	import Input from '$lib/components/ui/input.svelte';
	import { DropdownMenu } from 'bits-ui';
	import MenuPanel from '$lib/components/ui/menu-panel.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ModelRow from './model-row.svelte';
	import ModelFilterMenu from './model-filter-menu.svelte';
	import ModelModal from './model-modal.svelte';
	import AddModelForm from './add-model-form.svelte';
	import CloneModelDialog from './clone-model-dialog.svelte';
	import { CONTEXT_PRESETS, contextLabel } from '$lib/context-window';
	import {
		EMPTY_MODEL_FILTERS,
		filterModels,
		hasActiveFilters,
		type ModelFilters
	} from '$lib/model-filters';

	interface Props {
		providerId?: string;
		models: Model[];
		loading?: boolean;
		failed?: string;
		showProvider?: boolean;
		providerLabels?: Record<string, string>;
		onreload: () => void;
		onenable?: (ids: string[]) => Promise<void>;
		ondisable?: (ids: string[]) => Promise<void>;
		onopenprovider?: (id: string) => void;
		// onallowoverrides changes a connection's override policy from a row in
		// the combined list; a connection pane leaves it unset.
		onallowoverrides?: (providerId: string, allowed: boolean) => void;
		hasMore?: boolean;
		onloadmore?: () => void;
		canAdd?: boolean;
		paidFree?: boolean;
		// A connection pane renders the search field and the filter menu in its
		// own toolbar while the list keeps filtering, so the pane owns the
		// filter and hears what this list changes it to.
		filters?: ModelFilters;
		onfilters?: (next: ModelFilters) => void;
		// hideSearchFilters drops this list's own search row when the pane
		// above renders them; bulk groups and the add control stay here.
		hideSearchFilters?: boolean;
		// toolbarStart and toolbarTitle let a page put its own row around the
		// controls: the all-models view renders Back and its title here, while
		// a connection tab keeps the plain control row.
		toolbarStart?: Snippet;
		toolbarTitle?: Snippet;
	}

	let {
		providerId = '',
		models,
		loading = false,
		failed = '',
		showProvider = false,
		providerLabels = {},
		onreload,
		onenable,
		ondisable,
		onopenprovider,
		onallowoverrides,
		hasMore = false,
		onloadmore,
		canAdd = false,
		paidFree = false,
		filters = { ...EMPTY_MODEL_FILTERS },
		onfilters,
		hideSearchFilters = false,
		toolbarStart,
		toolbarTitle
	}: Props = $props();

	// active is the model the modal holds; editing asks the modal to open on
	// its form rather than the read view.
	let active = $state<Model | null>(null);
	let modalOpen = $state(false);
	let editing = $state(false);
	let adding = $state(false);
	let working = $state(false);
	let notice = $state('');

	// cloning holds the row a dialog is open for, so a dialog never renders
	// without the model it acts on.
	let cloning = $state<Model | null>(null);

	function openModel(model: Model) {
		active = model;
		editing = false;
		modalOpen = true;
	}

	function editModel(model: Model) {
		active = model;
		editing = true;
		modalOpen = true;
	}

	const visible = $derived(filterModels(models, filters));
	const filtered = $derived(hasActiveFilters(filters));
	const cloneIds = $derived(
		models.filter((model) => model.provider_id === providerId).map((model) => model.model_id)
	);
	// providerModelIDs names every model of the connection this table shows,
	// which is what a connection-wide context window is written on.
	const providerModelIDs = $derived([
		...new Set(
			models.filter((model) => model.provider_id === providerId).map((model) => model.model_id)
		)
	]);

	const menuIconTrigger = cn(
		buttonVariants({ variant: 'outline', size: 'icon' }),
		'text-muted-foreground hover:text-foreground data-[state=open]:text-foreground'
	);

	const connectionBulk = $derived(providerId !== '' && providerModelIDs.length > 0);
	// The context-window and capability bulk menus stay off; the on/off menu is
	// gated on the enable/disable handlers being wired (a connection tab) below.
	const showBulkActions = false;
	const capabilityBulk = [
		{ field: 'tools', labelKey: 'ui.pages.providersPage.modelsTable.columnTools', icon: 'wrench' },
		{
			field: 'reasoning',
			labelKey: 'ui.pages.providersPage.modelsTable.columnReasoning',
			icon: 'cpu'
		},
		{ field: 'vision', labelKey: 'ui.pages.providersPage.modelsTable.columnVision', icon: 'eye' }
	] as const;

	const toolbarGroup = 'flex shrink-0 flex-wrap items-center gap-0.75 max-md:flex-nowrap';
	const toolbarSection =
		'flex shrink-0 flex-wrap items-center gap-0.75 border-s border-border ps-3 ms-0.5 max-md:flex-nowrap max-md:first:border-s-0 max-md:first:ps-0 max-md:first:ms-0';
	const toolbarActionsRow =
		'flex w-full min-w-0 flex-nowrap items-center gap-0.75 overflow-x-auto overscroll-x-contain pb-0.5 [-webkit-overflow-scrolling:touch] md:contents';

	// The sentinel watches the foot of the list and asks the page for the next
	// page once it scrolls into view. It re-arms after every page (the row
	// count and the loading flag change), so a sentinel still in view keeps
	// paging until the server runs out. It only fires when the page wired a
	// paging handler, so a connection pane that loads its models whole is
	// untouched.
	function watchMore(node: HTMLElement) {
		$effect(() => {
			void models.length;
			if (!onloadmore || !hasMore || loading) return;
			const observer = new IntersectionObserver((entries) => {
				if (entries.some((entry) => entry.isIntersecting)) onloadmore?.();
			});
			observer.observe(node);
			return () => observer.disconnect();
		});
	}

	async function toggle(model: Model, enabled: boolean) {
		if (!onenable || !ondisable) return;
		working = true;
		notice = '';
		try {
			await (enabled ? onenable : ondisable)([model.model_id]);
			onreload();
		} finally {
			working = false;
		}
	}

	// A saved capability reloads the roster, so the row shows the value the
	// daemon stored rather than the one that was picked.
	async function saveCapability(
		model: Model,
		field: 'tools' | 'reasoning' | 'vision',
		value: boolean | null
	) {
		await api(
			'/models/' + encodeURIComponent(model.provider_id) + '/' + encodeURIComponent(model.model_id),
			{ method: 'PATCH', body: JSON.stringify({ [field]: value }) }
		);
		notice = $t('ui.pages.providersPage.context.saved');
		onreload();
	}

	async function saveContext(model: Model, value: number | null) {
		await api(
			'/models/' + encodeURIComponent(model.provider_id) + '/' + encodeURIComponent(model.model_id),
			{ method: 'PATCH', body: JSON.stringify({ context_window: value }) }
		);
		toast.success($t('ui.pages.providersPage.context.saved'));
		onreload();
	}

	async function saveMaxOutput(model: Model, value: number | null) {
		await api(
			'/models/' + encodeURIComponent(model.provider_id) + '/' + encodeURIComponent(model.model_id),
			{ method: 'PATCH', body: JSON.stringify({ max_output: value }) }
		);
		toast.success($t('ui.pages.providersPage.maxOutput.saved'));
		onreload();
	}

	// The whole connection is sized at once rather than a row at a time.
	async function setCapabilityForAll(
		field: 'tools' | 'reasoning' | 'vision',
		value: boolean | null
	) {
		if (providerId === '' || providerModelIDs.length === 0) return;
		const name = $t(
			field === 'tools'
				? 'ui.pages.providersPage.modelsTable.columnTools'
				: field === 'reasoning'
					? 'ui.pages.providersPage.modelsTable.columnReasoning'
					: 'ui.pages.providersPage.modelsTable.columnVision'
		);
		const valueLabel =
			value === null
				? $t('ui.pages.providersPage.modelsTable.capabilityDefault')
				: value
					? $t('ui.pages.providersPage.modelsTable.capabilityOn')
					: $t('ui.pages.providersPage.modelsTable.capabilityOff');
		working = true;
		notice = '';
		try {
			await api('/connections/' + encodeURIComponent(providerId) + '/models/capabilities', {
				method: 'POST',
				body: JSON.stringify({ model_ids: providerModelIDs, [field]: value })
			});
			notice = $t('ui.pages.providersPage.modelsTable.capabilityAllSet', {
				values: {
					name,
					count: providerModelIDs.length,
					value: valueLabel
				}
			});
			onreload();
		} catch (failure) {
			notice = $t('ui.pages.providersPage.modelsTable.capabilityAllFailed', {
				values: { name, detail: failure instanceof Error ? failure.message : String(failure) }
			});
		} finally {
			working = false;
		}
	}

	// A null value clears every override on the connection.
	async function setContextForAll(value: number | null) {
		if (providerId === '' || providerModelIDs.length === 0) return;
		working = true;
		notice = '';
		try {
			await api('/connections/' + encodeURIComponent(providerId) + '/models/context', {
				method: 'POST',
				body: JSON.stringify({ model_ids: providerModelIDs, context_window: value })
			});
			notice =
				value === null
					? $t('ui.pages.providersPage.context.allCleared', {
							values: { count: providerModelIDs.length }
						})
					: $t('ui.pages.providersPage.context.allSet', {
							values: { count: providerModelIDs.length, value: contextLabel(value) }
						});
			onreload();
		} catch (failure) {
			notice = $t('ui.pages.providersPage.context.allFailed', {
				values: { detail: failure instanceof Error ? failure.message : String(failure) }
			});
		} finally {
			working = false;
		}
	}

	function onCloned(model: Model) {
		cloning = null;
		openModel(model);
		notice = $t('ui.pages.providersPage.clone.done', { values: { id: model.model_id } });
		onreload();
	}
</script>

<div class="flex flex-col">
	<div
		class={toolbarStart || toolbarTitle
			? 'models-toolbar py-4'
			: 'filter-toolbar flex flex-col gap-3 py-4 md:flex-row md:flex-wrap md:items-end md:gap-3'}
	>
		{#if toolbarStart}
			<div class="models-start">{@render toolbarStart()}</div>
		{/if}
		{#if toolbarTitle}
			<div class="models-center min-w-0">{@render toolbarTitle()}</div>
		{/if}
		<div class={toolbarStart || toolbarTitle ? 'models-end' : 'contents'}>
			{#if !hideSearchFilters}
				<div class="models-search relative w-full min-w-0 shrink-0 md:min-w-48 md:flex-1">
					<span
						class="pointer-events-none absolute inset-y-0 start-2 flex items-center text-muted-foreground"
					>
						<Icon name="search" size={14} />
					</span>
					<Input
						class="ps-7"
						placeholder={$t('ui.pages.providersPage.models.search')}
						aria-label={$t('ui.pages.providersPage.models.search')}
						value={filters.search}
						oninput={(event) => onfilters?.({ ...filters, search: event.currentTarget.value })}
					/>
				</div>
			{/if}
			<div class={toolbarActionsRow}>
				{#if !hideSearchFilters}
					<div
						class={toolbarGroup}
						role="group"
						aria-label={$t('ui.pages.providersPage.models.toolbarFilters')}
					>
						<ModelFilterMenu {filters} {onfilters} {models} {paidFree} />
					</div>
				{/if}
				{#if connectionBulk && showBulkActions}
					<div
						class={toolbarSection}
						role="group"
						aria-label={$t('ui.pages.providersPage.models.toolbarConnectionActions')}
					>
						<DropdownMenu.Root>
							<DropdownMenu.Trigger
								class={menuIconTrigger}
								aria-label={$t('ui.pages.providersPage.context.setAll', {
									values: { count: providerModelIDs.length }
								})}
								title={$t('ui.pages.providersPage.context.setAll', {
									values: { count: providerModelIDs.length }
								})}
							>
								<Icon name="ruler-2" size={16} />
							</DropdownMenu.Trigger>
							<MenuPanel class="z-50 min-w-44 surface-pop p-1" align="end">
								<DropdownMenu.Item
									class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none"
									disabled={working}
									onSelect={() => void setContextForAll(null)}
								>
									<Icon name="refresh" size={14} />
									{$t('ui.pages.providersPage.context.reset')}
								</DropdownMenu.Item>
								<DropdownMenu.Separator class="my-1 h-px bg-border" />
								{#each CONTEXT_PRESETS as preset (preset)}
									<DropdownMenu.Item
										class="flex min-h-control cursor-pointer items-center rounded-control px-2 py-1.5 font-mono text-sm outline-none"
										disabled={working}
										onSelect={() => void setContextForAll(preset)}
									>
										{contextLabel(preset)}
									</DropdownMenu.Item>
								{/each}
							</MenuPanel>
						</DropdownMenu.Root>
						{#each capabilityBulk as capability (capability.field)}
							<DropdownMenu.Root>
								<DropdownMenu.Trigger
									class={menuIconTrigger}
									aria-label={$t('ui.pages.providersPage.modelsTable.capabilityAll', {
										values: { name: $t(capability.labelKey), count: providerModelIDs.length }
									})}
									title={$t('ui.pages.providersPage.modelsTable.capabilityAll', {
										values: { name: $t(capability.labelKey), count: providerModelIDs.length }
									})}
								>
									<Icon name={capability.icon} size={16} />
								</DropdownMenu.Trigger>
								<MenuPanel class="z-50 min-w-36 surface-pop p-1" align="end">
									<DropdownMenu.Item
										class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none"
										disabled={working}
										onSelect={() => void setCapabilityForAll(capability.field, null)}
									>
										<Icon name="refresh" size={14} />
										{$t('ui.pages.providersPage.modelsTable.capabilityDefault')}
									</DropdownMenu.Item>
									<DropdownMenu.Item
										class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none"
										disabled={working}
										onSelect={() => void setCapabilityForAll(capability.field, true)}
									>
										<Icon name="circle-check" size={14} />
										{$t('ui.pages.providersPage.modelsTable.capabilityOn')}
									</DropdownMenu.Item>
									<DropdownMenu.Item
										class="flex min-h-control cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-sm outline-none"
										disabled={working}
										onSelect={() => void setCapabilityForAll(capability.field, false)}
									>
										<Icon name="circle-off" size={14} />
										{$t('ui.pages.providersPage.modelsTable.capabilityOff')}
									</DropdownMenu.Item>
								</MenuPanel>
							</DropdownMenu.Root>
						{/each}
					</div>
				{/if}
				{#if canAdd}
					<Button variant="outline" size="sm" class="shrink-0" onclick={() => (adding = !adding)}>
						<Icon name="plus" size={14} />
						{$t('ui.pages.providersPage.models.addModel')}
					</Button>
				{/if}
			</div>
		</div>
	</div>

	{#if adding && providerId !== ''}
		<div class="pb-3">
			<AddModelForm
				{providerId}
				onadded={(model) => {
					adding = false;
					// A row is named by its provider and its model, so the added
					// one opens rather than leaving the list behind it.
					openModel(model);
					notice = $t('ui.pages.providersPage.models.addModelDone', {
						values: { id: model.model_id }
					});
					onreload();
				}}
				oncancel={() => (adding = false)}
			/>
		</div>
	{/if}

	{#if notice}
		<p class="pb-2 text-xs text-muted-foreground" role="status">{notice}</p>
	{/if}

	<div class="pb-6">
		{#if loading && models.length === 0}
			<p class="px-2 py-6 text-sm text-muted-foreground">
				{$t('ui.pages.providersPage.models.loading')}
			</p>
		{:else if failed}
			<div class="flex flex-col items-start gap-2 px-2 py-6">
				<p class="text-sm text-destructive">
					{$t('ui.pages.providersPage.models.loadFailed', { values: { detail: failed } })}
				</p>
				<Button variant="outline" size="sm" onclick={onreload}>
					<Icon name="refresh" size={14} />
					{$t('ui.pages.providersPage.models.retry')}
				</Button>
			</div>
		{:else if models.length === 0}
			<div class="flex flex-col items-start gap-2 px-2 py-6">
				<p class="text-sm text-muted-foreground">{$t('ui.pages.providersPage.models.empty')}</p>
				<div class="flex gap-2">
					<Button variant="outline" size="sm" onclick={onreload}>
						<Icon name="refresh" size={14} />
						{$t('ui.pages.providersPage.models.emptyAction')}
					</Button>
					{#if canAdd}
						<Button variant="ghost" size="sm" onclick={() => (adding = true)}>
							<Icon name="plus" size={14} />
							{$t('ui.pages.providersPage.models.addModel')}
						</Button>
					{/if}
				</div>
			</div>
		{:else if visible.length === 0}
			<div class="flex flex-col items-start gap-2 px-2 py-6">
				<p class="text-sm text-muted-foreground">{$t('ui.pages.providersPage.models.noMatch')}</p>
				{#if filtered}
					<Button
						variant="outline"
						size="sm"
						onclick={() => onfilters?.({ ...EMPTY_MODEL_FILTERS })}
					>
						<Icon name="refresh" size={14} />
						{$t('ui.pages.providersPage.models.clearFilters')}
					</Button>
				{/if}
			</div>
		{:else}
			<div class="flex flex-col gap-2 pb-2">
				{#each visible as model (model.provider_id + '/' + model.model_id)}
					<ModelRow
						{model}
						{showProvider}
						providerLabel={providerLabels[model.provider_id] ?? model.provider_id}
						ontoggle={toggle}
						onopen={openModel}
						onedit={editModel}
						onclone={providerId === '' ? undefined : (row) => (cloning = row)}
						onallowoverrides={providerId === '' && onallowoverrides
							? (row, allowed) => onallowoverrides(row.provider_id, allowed)
							: undefined}
						onsavecontext={saveContext}
						onsavemaxoutput={saveMaxOutput}
						onsavecapability={saveCapability}
					/>
				{/each}
			</div>
			{#if onloadmore && hasMore}
				<div
					class="flex items-center justify-center py-3 text-xs text-muted-foreground"
					{@attach watchMore}
				>
					{#if loading}
						<span class="flex items-center gap-2">
							<Icon name="loader" size={14} spin />
							{$t('ui.common.loading')}
						</span>
					{/if}
				</div>
			{/if}
		{/if}
	</div>

	{#if cloning}
		<CloneModelDialog
			open={cloning !== null}
			onOpenChange={(value: boolean) => {
				if (!value) cloning = null;
			}}
			source={cloning}
			{providerId}
			existingIds={cloneIds}
			oncloned={onCloned}
		/>
	{/if}

	<ModelModal
		bind:open={modalOpen}
		model={active}
		providerLabel={active ? (providerLabels[active.provider_id] ?? active.provider_id) : ''}
		allModels={providerId === '' ? models : []}
		startEditing={editing}
		{onopenprovider}
		onclone={(row) => {
			modalOpen = false;
			cloning = row;
		}}
		onchanged={(updated) => {
			if (
				active &&
				active.provider_id === updated.provider_id &&
				active.model_id === updated.model_id
			) {
				active = updated;
			}
			onreload();
		}}
	/>
</div>
