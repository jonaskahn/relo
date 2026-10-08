<script lang="ts">
	import { goto } from '$app/navigation';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { api } from '$lib/api';
	import type { DetailLayer, Model, ModelDetails, ModelOverride, Prices } from '$lib/types';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import { DropdownMenu } from 'bits-ui';
	import MenuPanel from '$lib/components/ui/menu-panel.svelte';
	import { Switch } from 'bits-ui';
	import Icon from '$lib/components/ui/icon.svelte';
	import PriceLayers from './price-layers.svelte';
	import PricedAsPicker from './priced-as-picker.svelte';
	import TokenSizeField from './token-size-field.svelte';
	import { formatDollars, formatTokens, formatUnitPrice, parseDollars } from '$lib/format';
	import { contextLabel, parseContextInput, roundTokenCount } from '$lib/context-window';
	import { SOURCE_LABEL_KEY, headingPriceSource, tilePriceSource } from '$lib/model-sources';
	import { effectiveDetailValue } from '$lib/model-detail';
	import { PRICE_TAG_KEY, categoriesOf, isPriced, priceTag } from '$lib/model-filters';

	// ModelModal reads one model and edits the same one: the footer's Edit
	// swaps the body to the form while the header, summary and actions stay,
	// so nothing an operator reads is lost when they change it.
	interface Props {
		open?: boolean;
		model: Model | null;
		providerLabel?: string;
		allModels?: Model[];
		// startEditing opens the form instead of the read view, once the detail
		// layers have arrived: the form must never open on a list row, which
		// carries no override layer to compare against.
		startEditing?: boolean;
		onchanged: (updated: Model) => void;
		onopenprovider?: (providerId: string) => void;
		onclone?: (model: Model) => void;
	}

	let {
		open = $bindable(false),
		model,
		providerLabel = '',
		allModels = [],
		startEditing = false,
		onchanged,
		onopenprovider,
		onclone
	}: Props = $props();

	// detailTimeoutMs bounds one detail read, which is what turns a request
	// nothing answers into a failure an operator can retry.
	const detailTimeoutMs = 10_000;

	let view = $state<'read' | 'edit'>('read');
	let detail = $state<Model | null>(null);
	let loading = $state(false);
	let failed = $state('');
	let reading: { controller: AbortController } | null = null;
	// loadedKey names the model the modal has read, so a list refresh that
	// hands the modal the same model again never resets the open form.
	let loadedKey = $state('');

	let draft = $state(editDraft());
	let sizeProblems = $state({ contextWindow: '', maxInput: '', maxOutput: '' });
	let saving = $state(false);
	let saveFailed = $state('');
	let discardOpen = $state(false);

	const shown = $derived(detail ?? model);
	const layers = $derived<ModelDetails | null>(shown?.details ?? model?.details ?? null);
	const categories = $derived(
		categoriesOf(allModels).length > 0
			? categoriesOf(allModels)
			: categoriesOf(shown ? [shown] : [])
	);
	const headingSource = $derived(headingPriceSource(shown?.prices));
	const groupRefs = $derived(shown?.group_refs ?? []);
	const dirty = $derived(
		model !== null && JSON.stringify(draft.body()) !== JSON.stringify(draft.initial)
	);
	// The form needs the override layer the read view loads; a list row does
	// not carry one, so Edit stays disabled until the detail read answers.
	const editable = $derived(detail !== null || (shown?.details ?? null) !== null);

	// overrideEntries is what the operator pinned on this model: the one
	// section that tells an override apart from the provider's own values.
	const overrideEntries = $derived.by((): { id: string; label: string; value: string }[] => {
		if (shown === null) return [];
		const entries: { id: string; label: string; value: string }[] = [];
		const label = (key: string) => $t('ui.pages.providersPage.models.' + key);
		const yes = $t('ui.common.yes');
		const no = $t('ui.common.no');
		const push = (id: string, name: string, value: string | null | undefined) => {
			if (value === null || value === undefined || value === '') return;
			entries.push({ id, label: name, value });
		};
		const layer = layers?.override ?? null;
		if (layer !== null) {
			push('name', label('detailName'), layer.name);
			push('description', label('detailDescription'), layer.description);
			push('category', label('detailCategory'), layer.category);
			push(
				'context',
				label('detailContext'),
				layer.context_window === null ? null : contextLabel(roundTokenCount(layer.context_window))
			);
			push(
				'max_input',
				label('detailMaxInput'),
				layer.max_input === null ? null : contextLabel(roundTokenCount(layer.max_input))
			);
			push(
				'max_output',
				label('detailMaxOutput'),
				layer.max_output === null ? null : formatTokens(layer.max_output)
			);
			push('tools', label('detailTools'), layer.tools === null ? null : layer.tools ? yes : no);
			push(
				'reasoning',
				label('detailReasoning'),
				layer.reasoning === null ? null : layer.reasoning ? yes : no
			);
			push('vision', label('detailVision'), layer.vision === null ? null : layer.vision ? yes : no);
		}
		const money = (value: number | null | undefined) =>
			value === null || value === undefined ? null : formatUnitPrice(value);
		const prices = shown.prices.override;
		push('price_input', label('input'), money(prices.input));
		push('price_output', label('output'), money(prices.output));
		push('price_cache_read', label('cacheRead'), money(prices.cache_read));
		push('price_cache_write', label('cacheWrite'), money(prices.cache_write));
		push('price_threshold', label('threshold'), money(prices.ext_threshold));
		push('price_ext_input', label('input') + ' ↑', money(prices.ext_input));
		push('price_ext_output', label('output') + ' ↑', money(prices.ext_output));
		push('price_ext_cache_read', label('cacheRead') + ' ↑', money(prices.ext_cache_read));
		push('price_ext_cache_write', label('cacheWrite') + ' ↑', money(prices.ext_cache_write));
		return entries;
	});

	// A new model resets the modal to its read view and reads the detail
	// layers, so a second visit never shows what the last one left behind.
	// The key is the model's identity, not the object: an update that reaches
	// the modal through the list must not reset an open form.
	$effect(() => {
		if (!open) {
			loadedKey = '';
			return;
		}
		const target = model;
		if (target === null) return;
		const key = target.provider_id + '/' + target.model_id;
		if (key === loadedKey) return;
		loadedKey = key;
		reset();
		void loadDetail(target);
	});

	function reset() {
		view = 'read';
		detail = null;
		failed = '';
		saveFailed = '';
		sizeProblems = { contextWindow: '', maxInput: '', maxOutput: '' };
		draft = editDraft();
	}

	async function loadDetail(target: Model) {
		const id = target.model_id;
		if (id === '') return;
		reading?.controller.abort();
		const load = { controller: new AbortController() };
		reading = load;
		loading = true;
		failed = '';
		let timedOut = false;
		const timer = setTimeout(() => {
			timedOut = true;
			load.controller.abort();
		}, detailTimeoutMs);
		try {
			const full = await api<Model>(
				'/models/' + encodeURIComponent(target.provider_id) + '/' + encodeURIComponent(id),
				{ signal: load.controller.signal }
			);
			if (reading === load) detail = full;
			if (reading === load && startEditing && view === 'read') {
				draft.load(full);
				view = 'edit';
			}
		} catch (error) {
			if (reading !== load) return;
			failed = timedOut
				? $t('ui.pages.providersPage.models.loadTimedOut')
				: error instanceof Error
					? error.message
					: String(error);
		} finally {
			clearTimeout(timer);
			if (reading === load) {
				reading = null;
				loading = false;
			}
		}
	}

	// The fields shown with the layer each value came from.
	const fields: { key: keyof DetailLayer; labelKey: string }[] = [
		{ key: 'name', labelKey: 'ui.pages.providersPage.models.detailName' },
		{ key: 'family', labelKey: 'ui.pages.providersPage.models.detailFamily' },
		{ key: 'category', labelKey: 'ui.pages.providersPage.models.detailCategory' },
		{ key: 'context_window', labelKey: 'ui.pages.providersPage.models.detailContext' },
		{ key: 'max_input', labelKey: 'ui.pages.providersPage.models.detailMaxInput' },
		{ key: 'max_output', labelKey: 'ui.pages.providersPage.models.detailMaxOutput' },
		{ key: 'tools', labelKey: 'ui.pages.providersPage.models.detailTools' },
		{ key: 'reasoning', labelKey: 'ui.pages.providersPage.models.detailReasoning' },
		{ key: 'vision', labelKey: 'ui.pages.providersPage.models.detailVision' },
		{ key: 'status', labelKey: 'ui.pages.providersPage.models.detailStatus' },
		{ key: 'release_date', labelKey: 'ui.pages.providersPage.models.detailRelease' }
	];
	// The overview tiles read the numbers; the capability row reads the flags.
	const overviewFields = fields.filter((field) =>
		['category', 'context_window', 'max_input', 'max_output'].includes(field.key)
	);
	const capabilityFields = fields.filter((field) =>
		['tools', 'reasoning', 'vision'].includes(field.key)
	);

	function show(key: keyof DetailLayer): string {
		if (shown === null) return '—';
		const value = effectiveDetailValue(shown, layers, key);
		if (value === null || value === undefined) return '—';
		if (key === 'context_window' || key === 'max_input') {
			return contextLabel(roundTokenCount(Number(value)));
		}
		if (key === 'max_output') {
			return formatTokens(Number(value));
		}
		if (typeof value === 'boolean') {
			return value ? $t('ui.common.yes') : $t('ui.common.no');
		}
		return String(value);
	}

	function sourceOf(key: keyof DetailLayer): string {
		const map = layers?.effective_source ?? null;
		const name = map ? (map[key as keyof typeof map] ?? '') : '';
		if (name === '') return '';
		return SOURCE_LABEL_KEY[name] ?? '';
	}

	function priceSource(key: 'input' | 'output'): string {
		return shown ? tilePriceSource(shown.prices.effective_source[key]) : '';
	}

	// Keeping the layer the form opened from is what lets Save write exactly the
	// fields an operator changed.
	function editDraft() {
		function emptyLayer(): DetailLayer {
			return {
				name: null,
				description: null,
				family: null,
				category: null,
				context_window: null,
				max_input: null,
				max_output: null,
				tools: null,
				reasoning: null,
				vision: null,
				status: null,
				release_date: null
			};
		}
		function tri(value: boolean | null | undefined): '' | 'yes' | 'no' {
			if (value === null || value === undefined) return '';
			return value ? 'yes' : 'no';
		}
		function parseTri(value: '' | 'yes' | 'no'): boolean | null {
			if (value === '') return null;
			return value === 'yes';
		}
		function sizeLabel(value: number | null | undefined): string {
			if (value == null) return '';
			return contextLabel(roundTokenCount(value));
		}
		function sizeProblem(raw: string): string {
			const trimmed = raw.trim();
			if (trimmed === '') return '';
			const parsed = parseContextInput(trimmed);
			if (parsed.ok) return '';
			return 'ui.pages.providersPage.context.error.' + parsed.reason;
		}
		function tokenCount(raw: string): number | null {
			const trimmed = raw.trim();
			if (trimmed === '') return null;
			const parsed = parseContextInput(trimmed);
			if (!parsed.ok) return null;
			return roundTokenCount(parsed.value);
		}

		return {
			enabled: false,
			name: '',
			description: '',
			category: '',
			contextWindow: '',
			maxInput: '',
			maxOutput: '',
			tools: '' as '' | 'yes' | 'no',
			reasoning: '' as '' | 'yes' | 'no',
			vision: '' as '' | 'yes' | 'no',
			input: '',
			output: '',
			cacheRead: '',
			cacheWrite: '',
			extThreshold: '',
			extInput: '',
			extOutput: '',
			extCacheRead: '',
			extCacheWrite: '',
			initial: '',
			body(): { enabled: boolean; override: ModelOverride } {
				return {
					enabled: this.enabled,
					override: {
						name: draft.name.trim() === '' ? null : draft.name.trim(),
						description: draft.description.trim() === '' ? null : draft.description.trim(),
						category: this.category === '' ? null : this.category,
						context_window: tokenCount(this.contextWindow),
						max_input: tokenCount(this.maxInput),
						max_output: tokenCount(this.maxOutput),
						tools: parseTri(this.tools),
						reasoning: parseTri(this.reasoning),
						vision: parseTri(this.vision),
						prices: {
							input: parseDollars(this.input),
							output: parseDollars(this.output),
							cache_read: parseDollars(this.cacheRead),
							cache_write: parseDollars(this.cacheWrite),
							ext_threshold: parseDollars(this.extThreshold),
							ext_input: parseDollars(this.extInput),
							ext_output: parseDollars(this.extOutput),
							ext_cache_read: parseDollars(this.extCacheRead),
							ext_cache_write: parseDollars(this.extCacheWrite)
						}
					}
				};
			},
			problems(): { contextWindow: string; maxInput: string; maxOutput: string } {
				return {
					contextWindow: sizeProblem(this.contextWindow),
					maxInput: sizeProblem(this.maxInput),
					maxOutput: sizeProblem(this.maxOutput)
				};
			},
			load(source: Model) {
				const layer: DetailLayer = source.details?.override ?? emptyLayer();
				const prices: Prices = source.prices.override;
				this.enabled = source.enabled;
				this.name = layer.name ?? '';
				this.description = layer.description ?? '';
				this.category = layer.category ?? '';
				this.contextWindow = sizeLabel(layer.context_window);
				this.maxInput = sizeLabel(layer.max_input);
				this.maxOutput = sizeLabel(layer.max_output);
				this.tools = tri(layer.tools);
				this.reasoning = tri(layer.reasoning);
				this.vision = tri(layer.vision);
				this.input = formatDollars(prices.input);
				this.output = formatDollars(prices.output);
				this.cacheRead = formatDollars(prices.cache_read);
				this.cacheWrite = formatDollars(prices.cache_write);
				this.extThreshold = formatDollars(prices.ext_threshold);
				this.extInput = formatDollars(prices.ext_input);
				this.extOutput = formatDollars(prices.ext_output);
				this.extCacheRead = formatDollars(prices.ext_cache_read);
				this.extCacheWrite = formatDollars(prices.ext_cache_write);
				this.initial = JSON.stringify(this.body());
			}
		};
	}

	function openEdit() {
		const source = detail ?? (shown?.details ? shown : null);
		if (source === null) return;
		draft.load(source);
		saveFailed = '';
		view = 'edit';
	}

	function writeDraft(source: Model, initial: { enabled: boolean; override: ModelOverride }) {
		const current = draft.body();
		const body: Record<string, unknown> = {};
		if (current.enabled !== initial.enabled) body.enabled = current.enabled;
		if (JSON.stringify(current.override) !== JSON.stringify(initial.override)) {
			body.override = current.override;
		}
		return body;
	}

	async function saveEdit() {
		const source = model;
		if (source === null) return;
		const problems = draft.problems();
		sizeProblems = problems;
		if (problems.contextWindow || problems.maxInput || problems.maxOutput) return;
		saving = true;
		saveFailed = '';
		try {
			const body = writeDraft(source, JSON.parse(draft.initial));
			if (Object.keys(body).length === 0) {
				view = 'read';
				return;
			}
			const updated = await api<Model>(
				'/models/' +
					encodeURIComponent(source.provider_id) +
					'/' +
					encodeURIComponent(source.model_id),
				{ method: 'PATCH', body: JSON.stringify(body) }
			);
			detail = updated;
			draft.load(updated);
			onchanged(updated);
			view = 'read';
			toast.success($t('ui.pages.providersPage.models.overrideSaved'));
		} catch (error) {
			saveFailed = error instanceof Error ? error.message : String(error);
		} finally {
			saving = false;
		}
	}

	function requestClose() {
		if (view === 'edit' && dirty) {
			discardOpen = true;
			return;
		}
		view = 'read';
	}

	function discard() {
		discardOpen = false;
		view = 'read';
	}

	function askClone() {
		if (model === null || !onclone) return;
		open = false;
		onclone(model);
	}
</script>

{#if model}
	<CenteredModal
		bind:open
		size="wide"
		title={model.name || model.model_id}
		description={providerLabel ? providerLabel + ' · ' + model.model_id : model.model_id}
		class="h-[min(720px,100%)] max-sm:h-[92dvh]"
		onOpenChange={(value: boolean) => {
			if (!value) requestClose();
		}}
	>
		<div class="flex flex-col gap-6">
			{#if view === 'read'}
				<header class="space-y-1">
					{#if shown?.description}
						<p class="max-w-prose text-sm leading-relaxed text-muted-foreground">
							{shown.description}
						</p>
					{/if}
					{#if shown?.upstream_model_id && shown.upstream_model_id !== shown.model_id}
						<p class="font-mono text-xs text-muted-foreground">
							{$t('ui.pages.providersPage.modelsTable.tagUpstream', {
								values: { id: shown.upstream_model_id }
							})}
						</p>
					{/if}
				</header>

				{#if loading}
					<p class="text-xs text-muted-foreground" role="status">
						{$t('ui.pages.providersPage.models.loadingDetails')}
					</p>
				{:else if failed}
					<div class="flex flex-wrap items-center gap-2">
						<p class="text-xs text-destructive" role="alert">
							{$t('ui.pages.providersPage.models.loadFailed', { values: { detail: failed } })}
						</p>
						<Button variant="outline" size="sm" onclick={() => model && void loadDetail(model)}>
							<Icon name="refresh" size={14} />
							{$t('ui.pages.providersPage.models.retry')}
						</Button>
					</div>
				{/if}

				<section class="space-y-3">
					<h3 class="section-heading">{$t('ui.pages.providersPage.models.overview')}</h3>
					<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
						{#each overviewFields as field (field.key)}
							<div class="min-w-0 rounded-xl bg-muted/60 px-3 py-3">
								<div class="text-xs text-muted-foreground">{$t(field.labelKey)}</div>
								<div class="mt-1 break-words text-sm font-medium tabular-nums">
									{show(field.key)}
								</div>
							</div>
						{/each}
					</div>
					<div class="flex flex-wrap gap-2">
						{#each capabilityFields as field (field.key)}
							{#if effectiveDetailValue(shown ?? model, layers, field.key) === true}
								<span class="badge border border-border px-2 py-1 text-xs"
									>{$t(field.labelKey)}</span
								>
							{/if}
						{/each}
					</div>
					{#if (shown?.active_accounts ?? 0) > 0}
						<p class="text-xs {shown?.routable ? 'text-muted-foreground' : 'text-warn'}">
							{(shown?.serving_accounts ?? 0) > 0
								? $t('ui.pages.providersPage.models.coverage', {
										values: {
											serving: shown?.serving_accounts ?? 0,
											active: shown?.active_accounts ?? 0
										}
									})
								: $t('ui.pages.providersPage.models.coverageNone')}
						</p>
					{/if}
				</section>

				<section class="space-y-3">
					<div class="flex flex-wrap items-center gap-2">
						<h3 class="section-heading">{$t('ui.pages.providersPage.models.pricing')}</h3>
						{#if headingSource}
							<span class="badge border border-border font-sans text-muted-foreground">
								{$t(SOURCE_LABEL_KEY[headingSource])}
							</span>
						{/if}
					</div>
					<div class="grid grid-cols-2 gap-3">
						<div class="min-w-0 rounded-xl bg-muted/60 px-3 py-3">
							<div class="text-xs text-muted-foreground">
								{$t('ui.pages.providersPage.models.input')}
							</div>
							<div class="mt-1 font-mono text-base font-semibold tabular-nums">
								{formatUnitPrice(shown?.prices.effective.input ?? null)}
							</div>
							{#if priceSource('input')}<div class="mt-1 text-xs text-muted-foreground">
									{$t(priceSource('input'))}
								</div>{/if}
						</div>
						<div class="min-w-0 rounded-xl bg-muted/60 px-3 py-3">
							<div class="text-xs text-muted-foreground">
								{$t('ui.pages.providersPage.models.output')}
							</div>
							<div class="mt-1 font-mono text-base font-semibold tabular-nums">
								{formatUnitPrice(shown?.prices.effective.output ?? null)}
							</div>
							{#if priceSource('output')}<div class="mt-1 text-xs text-muted-foreground">
									{$t(priceSource('output'))}
								</div>{/if}
						</div>
					</div>
					<p class="text-xs text-muted-foreground">
						{$t('ui.pages.providersPage.models.perMillion')}
					</p>
				</section>

				<section class="space-y-3">
					<div class="flex flex-wrap items-center gap-2">
						<h3 class="section-heading">{$t('ui.pages.providersPage.models.overrides')}</h3>
						{#if overrideEntries.length > 0}
							<span class="badge border border-border text-muted-foreground">
								{$t('ui.pages.providersPage.models.chipOverridden')}
							</span>
						{/if}
					</div>
					{#if overrideEntries.length === 0}
						<p class="rounded-panel bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
							{$t('ui.pages.providersPage.models.overridesNone')}
						</p>
					{:else}
						<dl class="grid gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
							{#each overrideEntries as entry (entry.id)}
								<div class="min-w-0">
									<dt class="text-xs text-muted-foreground">{entry.label}</dt>
									<dd class="mt-0.5 truncate text-sm" title={entry.value}>{entry.value}</dd>
								</div>
							{/each}
						</dl>
					{/if}
				</section>

				<section class="space-y-3">
					<h3 class="section-heading">{$t('ui.pages.providersPage.models.sources')}</h3>
					<dl class="grid gap-x-8 gap-y-3 sm:grid-cols-2 xl:grid-cols-3">
						{#each fields as field (field.key)}
							<div class="min-w-0 rounded-lg bg-muted/50 px-3 py-2">
								<dt class="text-xs text-muted-foreground">{$t(field.labelKey)}</dt>
								<dd class="mt-1 flex flex-wrap items-baseline gap-x-2 gap-y-1 text-sm">
									<span class="min-w-0 break-words">{show(field.key)}</span>
									{#if sourceOf(field.key)}<span class="text-xs text-muted-foreground"
											>{$t(sourceOf(field.key))}</span
										>{/if}
								</dd>
							</div>
						{/each}
					</dl>
				</section>

				<div class="flex flex-wrap items-center gap-2 text-xs">
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							class="relative inline-flex size-control shrink-0 items-center justify-center rounded-control border border-line text-muted-foreground hover:border-line-strong hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring data-[state=open]:border-line-strong data-[state=open]:text-ink"
							aria-label={$t('ui.pages.providersPage.models.groups')}
							title={$t('ui.pages.providersPage.models.groups')}
						>
							<Icon name="route" size={16} />
							{#if groupRefs.length > 0}
								<span
									class="absolute -top-1 -end-1 flex size-4 items-center justify-center rounded-full bg-accent-soft text-[0.6rem] font-semibold text-accent-ink"
									aria-hidden="true"
								>
									{groupRefs.length}
								</span>
							{/if}
						</DropdownMenu.Trigger>
						<MenuPanel class="z-50 min-w-48 surface-pop p-1" align="start">
							{#if groupRefs.length === 0}
								<p class="px-2 py-1.5 text-sm text-muted-foreground">
									{$t('ui.pages.providersPage.models.noGroups')}
								</p>
							{:else}
								{#each groupRefs as group (group)}
									<DropdownMenu.Item
										class="flex min-h-control cursor-pointer items-center rounded-control px-2 py-1.5 font-mono text-sm outline-none data-highlighted:bg-hover"
										onSelect={() => goto('/groups')}
									>
										{group}
									</DropdownMenu.Item>
								{/each}
							{/if}
						</MenuPanel>
					</DropdownMenu.Root>
					{#if onopenprovider}
						<Button variant="ghost" size="sm" onclick={() => onopenprovider?.(model.provider_id)}>
							<Icon name="external-link" size={14} />
							{$t('ui.pages.providersPage.models.openProvider')}
						</Button>
					{/if}
				</div>

				<details class="group rounded-panel bg-muted/40 px-3 py-2">
					<summary
						class="flex cursor-pointer items-center gap-2 py-1 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
					>
						<Icon
							name="chevron-right"
							size={15}
							class="text-muted-foreground transition-transform group-open:rotate-90"
						/>
						{$t('ui.pages.providersPage.models.compareSources')}
					</summary>
					<div class="pt-2"><PriceLayers prices={shown?.prices ?? model.prices} /></div>
				</details>

				<details class="group rounded-panel bg-muted/40 px-3 py-2">
					<summary
						class="flex cursor-pointer items-center gap-2 py-1 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
					>
						<Icon
							name="chevron-right"
							size={15}
							class="text-muted-foreground transition-transform group-open:rotate-90"
						/>
						{$t('ui.pages.providersPage.models.advanced')}
					</summary>
					<div class="grid gap-4 pt-2">
						<PricedAsPicker model={shown ?? model} onchange={onchanged} />
					</div>
				</details>
			{:else}
				<div class="flex flex-col gap-6">
					<section class="space-y-3">
						<h3 class="section-heading">{$t('ui.pages.providersPage.models.editIdentity')}</h3>
						<div class="field-grid">
							<Field id="model-name" label={$t('ui.pages.providersPage.models.overrideName')}>
								<Input id="model-name" bind:value={draft.name} />
							</Field>
							<Field
								id="model-category"
								label={$t('ui.pages.providersPage.models.overrideCategory')}
							>
								<NativeSelect id="model-category" bind:value={draft.category}>
									<option value="">{$t('ui.pages.providersPage.models.overrideUnset')}</option>
									{#each categories as category (category)}
										<option value={category}>{category}</option>
									{/each}
								</NativeSelect>
							</Field>
							<Field
								id="model-description"
								label={$t('ui.pages.providersPage.models.detailDescription')}
							>
								<Input id="model-description" bind:value={draft.description} />
							</Field>
						</div>
						<div class="flex items-center gap-2 text-sm">
							<Switch.Root
								bind:checked={draft.enabled}
								aria-label={$t('ui.pages.providersPage.modelsTable.toggleLabel')}
							/>
							<button
								type="button"
								data-plain
								class="text-sm font-medium"
								onclick={() => (draft.enabled = !draft.enabled)}
							>
								{$t('ui.pages.providersPage.modelsTable.statusOn')}
							</button>
						</div>
					</section>

					<section class="space-y-3">
						<h3 class="section-heading">{$t('ui.pages.providersPage.models.editPricing')}</h3>
						<div class="field-grid">
							<Field
								id="model-price-input"
								label={$t('ui.pages.providersPage.models.input') +
									' · ' +
									$t('ui.pages.providersPage.models.perMillion')}
							>
								<Input id="model-price-input" inputmode="decimal" bind:value={draft.input} />
							</Field>
							<Field
								id="model-price-output"
								label={$t('ui.pages.providersPage.models.output') +
									' · ' +
									$t('ui.pages.providersPage.models.perMillion')}
							>
								<Input id="model-price-output" inputmode="decimal" bind:value={draft.output} />
							</Field>
						</div>
						<details class="group rounded-panel bg-muted/40 px-3 py-2">
							<summary
								class="flex cursor-pointer items-center gap-2 py-1 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
							>
								<Icon
									name="chevron-right"
									size={15}
									class="text-muted-foreground transition-transform group-open:rotate-90"
								/>
								{$t('ui.pages.providersPage.models.moreRates')}
							</summary>
							<div class="field-grid pt-3">
								{#each [{ key: 'cacheRead', labelKey: 'cacheRead' }, { key: 'cacheWrite', labelKey: 'cacheWrite' }, { key: 'extThreshold', labelKey: 'threshold' }, { key: 'extInput', labelKey: 'input' }, { key: 'extOutput', labelKey: 'output' }, { key: 'extCacheRead', labelKey: 'cacheRead' }] as field (field.key)}
									<Field
										id={'model-price-' + field.key}
										label={$t('ui.pages.providersPage.models.' + field.labelKey)}
									>
										<Input
											id={'model-price-' + field.key}
											inputmode="decimal"
											bind:value={draft[field.key as 'input']}
										/>
									</Field>
								{/each}
							</div>
						</details>
					</section>

					<section class="space-y-3">
						<h3 class="section-heading">{$t('ui.pages.providersPage.models.editLimits')}</h3>
						<div class="field-grid">
							<TokenSizeField
								id="model-edit-context"
								label={$t('ui.pages.providersPage.models.overrideContext')}
								bind:value={draft.contextWindow}
								presets
								editLabel={$t('ui.pages.providersPage.context.edit', {
									values: {
										value: draft.contextWindow || $t('ui.pages.providersPage.context.unset')
									}
								})}
								error={sizeProblems.contextWindow ? $t(sizeProblems.contextWindow) : ''}
							/>
							<TokenSizeField
								id="model-edit-maxinput"
								label={$t('ui.pages.providersPage.models.overrideMaxInput')}
								bind:value={draft.maxInput}
								presets
								notice={$t('ui.pages.providersPage.models.overrideMaxInputNotice')}
								editLabel={$t('ui.pages.providersPage.models.overrideEditMaxInput', {
									values: { value: draft.maxInput || $t('ui.pages.providersPage.context.unset') }
								})}
								error={sizeProblems.maxInput ? $t(sizeProblems.maxInput) : ''}
							/>
							<TokenSizeField
								id="model-edit-maxoutput"
								label={$t('ui.pages.providersPage.models.overrideMaxOutput')}
								bind:value={draft.maxOutput}
								presets
								editLabel={$t('ui.pages.providersPage.maxOutput.edit', {
									values: { value: draft.maxOutput || $t('ui.pages.providersPage.context.unset') }
								})}
								error={sizeProblems.maxOutput ? $t(sizeProblems.maxOutput) : ''}
							/>
						</div>
					</section>

					<section class="space-y-3">
						<h3 class="section-heading">{$t('ui.pages.providersPage.models.editCapabilities')}</h3>
						<div class="field-grid">
							{#each [{ key: 'tools', labelKey: 'overrideTools' }, { key: 'reasoning', labelKey: 'overrideReasoning' }, { key: 'vision', labelKey: 'overrideVision' }] as field (field.key)}
								<Field
									id={'model-cap-' + field.key}
									label={$t('ui.pages.providersPage.models.' + field.labelKey)}
								>
									<NativeSelect
										id={'model-cap-' + field.key}
										bind:value={draft[field.key as 'tools' | 'reasoning' | 'vision']}
									>
										<option value="">{$t('ui.pages.providersPage.models.overrideUnset')}</option>
										<option value="yes">{$t('ui.pages.providersPage.models.overrideYes')}</option>
										<option value="no">{$t('ui.pages.providersPage.models.overrideNo')}</option>
									</NativeSelect>
								</Field>
							{/each}
						</div>
					</section>

					{#if saveFailed}
						<p class="flex items-center gap-2 text-sm text-destructive" role="alert">
							<Icon name="alert-triangle" size={15} />
							{saveFailed}
						</p>
					{/if}
				</div>
			{/if}
		</div>

		{#snippet footer()}
			<div class="flex flex-wrap items-center justify-between gap-3">
				<div class="flex min-w-0 items-center gap-2" role="status">
					<span class="mono-data shrink-0 text-sm">
						{shown
							? formatUnitPrice(shown.prices.effective.input) +
								' / ' +
								formatUnitPrice(shown.prices.effective.output)
							: '—'}
					</span>
					{#if shown && isPriced(shown.prices)}
						<span class="badge border border-border text-muted-foreground">
							{$t(PRICE_TAG_KEY[priceTag(shown.prices)])}
						</span>
					{:else}
						<span class="badge border border-warn/40 text-warn">
							{$t('ui.pages.providersPage.models.tagUnpriced')}
						</span>
					{/if}
					{#if shown?.overridden}
						<span class="badge border border-border text-muted-foreground">
							{$t('ui.pages.providersPage.models.chipOverridden')}
						</span>
					{/if}
				</div>
				<div class="flex shrink-0 items-center gap-2">
					{#if view === 'edit'}
						<Button variant="ghost" disabled={saving} onclick={() => (view = 'read')}>
							<Icon name="arrow-left" size={14} />
							{$t('ui.pages.providersPage.pane.back')}
						</Button>
						<Button disabled={saving} onclick={() => void saveEdit()}>
							<Icon name={saving ? 'loader' : 'check'} size={14} spin={saving} />
							{$t('ui.pages.providersPage.models.overrideSave')}
						</Button>
					{:else}
						{#if onclone}
							<Button variant="ghost" onclick={askClone}>
								<Icon name="copy-plus" size={14} />
								{$t('ui.pages.providersPage.modelsTable.clone')}
							</Button>
						{/if}
						<Button disabled={!editable} onclick={openEdit}>
							<Icon name="pencil" size={14} />
							{$t('ui.pages.providersPage.models.edit')}
						</Button>
					{/if}
				</div>
			</div>
		{/snippet}
	</CenteredModal>
{/if}

<ConfirmDialog
	bind:open={discardOpen}
	title={$t('ui.pages.providersPage.models.discardTitle')}
	body={$t('ui.pages.providersPage.models.discardBody')}
	confirmLabel={$t('ui.pages.providersPage.models.discardLeave')}
	tone="destructive"
	icon="alert-triangle"
	onconfirm={discard}
/>
