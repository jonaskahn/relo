<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import { formatTokensExact } from '$lib/format';
	import {
		averageText,
		errorRateText,
		slowestText,
		tokenTexts,
		type UsageListRow,
		type UsageSort,
		type UsageSortKey
	} from '$lib/usage-list';

	// GroupTable is the roll-up under a grouped tab. Each row is its own card;
	// the page owns searching, ordering, and how much of the list is shown.
	interface Props {
		rows: UsageListRow[];
		total: number;
		spendLabel: string;
		sort: UsageSort;
		query: string;
		shown: number;
		totalRows: number;
		showAll: boolean;
		isDay: boolean;
		onsort: (key: UsageSortKey) => void;
		onquery: (query: string) => void;
		ontoggleall: () => void;
		onexpand: (key: string) => void;
		expanded: ReadonlySet<string>;
	}

	let {
		rows,
		total,
		spendLabel,
		sort,
		query,
		shown,
		totalRows,
		showAll,
		isDay,
		onsort,
		onquery,
		ontoggleall,
		onexpand,
		expanded
	}: Props = $props();

	const columns: Array<{ key: UsageSortKey; label: string }> = [
		{ key: 'requests', label: 'columnRequests' },
		{ key: 'errors', label: 'columnErrorRate' },
		{ key: 'tokens', label: 'columnTokensTotal' },
		{ key: 'latency', label: 'columnAvgDuration' },
		{ key: 'spend', label: 'columnCost' }
	];

	const maxWeight = $derived(Math.max(1, ...rows.map((entry) => entry.spend.weight)));

	function sortIcon(key: UsageSortKey): IconName {
		return sort.key === key && !sort.descending ? 'arrow-up' : 'arrow-down';
	}

	function meterWidth(entry: UsageListRow): number {
		const value =
			sort.key === 'requests'
				? Number(entry.row.Requests)
				: sort.key === 'errors'
					? (entry.row.Errors / Math.max(1, entry.row.Requests)) * 100
					: sort.key === 'tokens'
						? tokenTexts(entry.row).exact.replaceAll(',', '')
						: sort.key === 'latency'
							? entry.row.Requests > 0
								? entry.row.DurationMs / entry.row.Requests
								: 0
							: entry.spend.weight;
		const numeric = typeof value === 'string' ? Number(value) : value;
		const max =
			sort.key === 'requests'
				? Math.max(1, ...rows.map((item) => Number(item.row.Requests)))
				: sort.key === 'errors'
					? 100
					: sort.key === 'tokens'
						? Math.max(
								1,
								...rows.map((item) => Number(tokenTexts(item.row).exact.replaceAll(',', '')))
							)
						: sort.key === 'latency'
							? Math.max(
									1,
									...rows.map((item) =>
										item.row.Requests > 0 ? item.row.DurationMs / item.row.Requests : 0
									)
								)
							: maxWeight;
		return numeric > 0 ? Math.max(2, (numeric / max) * 100) : 0;
	}

	function sortChoice(value: string): UsageSortKey {
		const key = value.split(':')[0];
		if (
			key === 'name' ||
			key === 'requests' ||
			key === 'errors' ||
			key === 'tokens' ||
			key === 'latency' ||
			key === 'spend'
		) {
			return key;
		}
		return 'spend';
	}

	const limit = 8;
	const canToggle = !query && totalRows > limit;
</script>

<div class="flex flex-wrap items-end justify-between gap-3">
	<h2 class="section-heading">
		{$t('ui.pages.usagePage.groupsTitle')}
		<span class="font-normal text-muted-foreground">· {totalRows}</span>
	</h2>
	<p class="text-xs text-muted-foreground">
		{$t('ui.pages.usagePage.listCaption', { values: { value: spendLabel } })}
	</p>
</div>

<div class="flex flex-wrap items-center gap-3">
	<div class="relative min-w-0 flex-1 basis-72">
		<label class="sr-only" for="usage-search">{$t('ui.pages.usagePage.searchGroups')}</label>
		<Icon
			name="search"
			size={16}
			class="pointer-events-none absolute start-3 top-1/2 -translate-y-1/2 text-muted-foreground"
		/>
		<Input
			id="usage-search"
			type="search"
			autocomplete="off"
			class="ps-9"
			value={query}
			oninput={(event) => onquery(event.currentTarget.value)}
			placeholder={$t('ui.pages.usagePage.searchGroups')}
		/>
	</div>
	<div class="w-52 sm:hidden">
		<label class="sr-only" for="usage-sort">{$t('ui.pages.usagePage.sortBy')}</label>
		<NativeSelect
			id="usage-sort"
			value={sort.key + ':' + (sort.descending ? 'desc' : 'asc')}
			onchange={(event) => onsort(sortChoice(event.currentTarget.value))}
		>
			<option value="name:asc">{$t('ui.pages.usagePage.sortName')}</option>
			<option value="requests:desc">{$t('ui.pages.usagePage.sortRequests')}</option>
			<option value="errors:desc">{$t('ui.pages.usagePage.sortErrors')}</option>
			<option value="tokens:desc">{$t('ui.pages.usagePage.sortTokens')}</option>
			<option value="latency:desc">{$t('ui.pages.usagePage.sortLatency')}</option>
			<option value="spend:desc">{$t('ui.pages.usagePage.sortSpend')}</option>
		</NativeSelect>
	</div>
</div>

{#if rows.length === 0}
	<div class="empty-panel section-surface border-dashed">
		{#if query}
			{$t('ui.pages.usagePage.noMatches')}
		{:else}
			{$t('ui.pages.usagePage.empty')}
		{/if}
	</div>
{:else}
	<div
		class="hidden grid-cols-[2.5rem_minmax(12rem,2.4fr)_repeat(5,minmax(5.5rem,1fr))] gap-4 border-b border-line px-4 pb-2 text-xs font-medium text-muted-foreground lg:grid"
		role="row"
	>
		<span aria-hidden="true"></span>
		<button
			type="button"
			class="flex min-w-0 items-center gap-1 text-start hover:text-ink"
			onclick={() => onsort('name')}
		>
			{$t(isDay ? 'ui.pages.usagePage.columnDate' : 'ui.pages.usagePage.columnGroup')}
			{#if sort.key === 'name'}<Icon name={sortIcon('name')} size={12} />{/if}
		</button>
		{#each columns as column (column.key)}
			<button
				type="button"
				class="flex items-center justify-end gap-1 text-end hover:text-ink"
				onclick={() => onsort(column.key)}
			>
				{$t('ui.pages.usagePage.' + column.label)}
				{#if sort.key === column.key}<Icon name={sortIcon(column.key)} size={12} />{/if}
			</button>
		{/each}
	</div>

	<ul class="flex flex-col gap-2.5">
		{#each rows as entry (entry.key)}
			{@const open = expanded.has(entry.key)}
			{@const tokens = tokenTexts(entry.row)}
			<li class="section-surface overflow-hidden">
				<div
					class="grid gap-x-4 gap-y-3 p-4 lg:grid-cols-[2.5rem_minmax(12rem,2.4fr)_repeat(5,minmax(5.5rem,1fr))] lg:items-center"
				>
					<button
						type="button"
						class="hidden size-control items-center justify-center rounded-control border border-line text-muted-foreground hover:bg-hover hover:text-ink lg:flex"
						aria-expanded={open}
						aria-controls={'usage-detail-' + entry.key}
						aria-label={open
							? $t('ui.pages.usagePage.hideDetails', { values: { name: entry.label } })
							: $t('ui.pages.usagePage.showDetails', { values: { name: entry.label } })}
						onclick={() => onexpand(entry.key)}
					>
						<Icon
							name="chevron-right"
							size={15}
							class="transition-transform duration-150 {open ? 'rotate-90' : ''}"
						/>
					</button>

					<div class="min-w-0 lg:col-start-2">
						<div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
							<span
								class="min-w-0 break-words font-semibold text-ink"
								title={entry.id !== entry.label ? entry.id : undefined}
							>
								{entry.label}
							</span>
							{#if entry.searchText}
								<span class="badge bg-sunken text-muted-foreground">{entry.searchText}</span>
							{/if}
						</div>
						<p class="mt-0.5 text-xs text-muted-foreground lg:hidden">
							<span class="mono-data text-ink">{entry.spend.text}</span>
						</p>
					</div>

					{#each columns as column, index (column.key)}
						<div class="min-w-0 lg:text-end {index > 1 ? 'hidden lg:block' : ''}">
							<p class="text-xs text-muted-foreground lg:hidden">
								{$t('ui.pages.usagePage.' + column.label)}
							</p>
							{#if column.key === 'requests'}
								<p class="mono-data text-sm text-ink">{entry.row.Requests.toLocaleString()}</p>
								{#if entry.row.RetriedRequests > 0}
									<p class="text-xs text-muted-foreground">
										{entry.row.RetriedRequests.toLocaleString()}
										{$t('ui.pages.usagePage.columnRetried').toLowerCase()}
									</p>
								{/if}
							{:else if column.key === 'errors'}
								<p
									class="flex items-center gap-1 text-sm lg:justify-end {entry.row.Errors > 0
										? 'text-destructive'
										: 'text-muted-foreground'}"
								>
									{#if entry.row.Errors > 0}<Icon name="circle-alert" size={13} />{/if}
									<span class="mono-data">{errorRateText(entry.row)}</span>
								</p>
							{:else if column.key === 'tokens'}
								<p class="mono-data text-sm text-ink" title={tokens.exact}>{tokens.compact}</p>
								<p class="text-xs text-muted-foreground">
									{formatTokensExact(entry.row.InputTokens)}
									{$t('ui.pages.usagePage.summaryTokensIn').toLowerCase()} · {formatTokensExact(
										entry.row.OutputTokens
									)}
									{$t('ui.pages.usagePage.summaryTokensOut').toLowerCase()}
								</p>
							{:else if column.key === 'latency'}
								<p class="mono-data text-sm text-ink">{averageText(entry.row)}</p>
								<p class="text-xs text-muted-foreground">
									{$t('ui.pages.usagePage.columnMaxDuration').toLowerCase()}
									{slowestText(entry.row)}
								</p>
							{:else}
								<p class="mono-data text-sm text-ink">{entry.spend.text}</p>
								<p class="text-xs text-muted-foreground">
									{#if total > 0 && entry.spend.weight > 0}
										{((entry.spend.weight / total) * 100).toFixed(1)}% {$t(
											'ui.pages.usagePage.ofTotal'
										)}
									{/if}
								</p>
							{/if}
						</div>
					{/each}

					<div class="lg:col-span-6 lg:col-start-2">
						<div class="h-1.5 overflow-hidden rounded-full bg-accent-soft" aria-hidden="true">
							<div class="h-full rounded-full bg-accent" style="width:{meterWidth(entry)}%"></div>
						</div>
					</div>

					<button
						type="button"
						class="flex h-control items-center justify-center gap-1.5 rounded-control border border-line text-sm text-muted-foreground hover:bg-hover hover:text-ink lg:hidden"
						aria-expanded={open}
						aria-controls={'usage-detail-' + entry.key}
						onclick={() => onexpand(entry.key)}
					>
						<Icon
							name="chevron-right"
							size={15}
							class="transition-transform duration-150 {open ? 'rotate-90' : ''}"
						/>
						{open ? $t('ui.pages.usagePage.hideDetails') : $t('ui.pages.usagePage.details')}
					</button>
				</div>

				{#if open}
					<dl
						id={'usage-detail-' + entry.key}
						class="grid grid-cols-2 gap-x-6 gap-y-3 border-t border-line bg-sunken p-4 text-sm lg:grid-cols-4"
					>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.metric.tokens_input')}
							</dt>
							<dd class="mono-data mt-0.5">{formatTokensExact(entry.row.InputTokens)}</dd>
						</div>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.metric.tokens_output')}
							</dt>
							<dd class="mono-data mt-0.5">{formatTokensExact(entry.row.OutputTokens)}</dd>
						</div>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.cacheReadShort')}
							</dt>
							<dd class="mono-data mt-0.5">{formatTokensExact(entry.row.CacheReadTokens)}</dd>
						</div>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.cacheWriteShort')}
							</dt>
							<dd class="mono-data mt-0.5">{formatTokensExact(entry.row.CacheWriteTokens)}</dd>
						</div>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.columnAttempts')}
							</dt>
							<dd class="mono-data mt-0.5">{entry.row.Attempts.toLocaleString()}</dd>
						</div>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.columnRetried')}
							</dt>
							<dd class="mono-data mt-0.5">{entry.row.RetriedRequests.toLocaleString()}</dd>
						</div>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.columnMaxDuration')}
							</dt>
							<dd class="mono-data mt-0.5">{slowestText(entry.row)}</dd>
						</div>
						<div>
							<dt class="text-xs text-muted-foreground">
								{$t('ui.pages.usagePage.summaryErrors')}
							</dt>
							<dd class="mono-data mt-0.5">{entry.row.Errors.toLocaleString()}</dd>
						</div>
					</dl>
				{/if}
			</li>
		{/each}
	</ul>

	{#if canToggle}
		<div class="flex flex-wrap items-center justify-between gap-3 border-t border-line pt-3">
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.usagePage.showingGroups', { values: { shown, total: totalRows } })}
			</p>
			<button
				type="button"
				class="h-control rounded-control border border-line bg-surface px-3 text-sm font-medium text-ink hover:bg-hover"
				onclick={ontoggleall}
			>
				{showAll
					? $t('ui.pages.usagePage.showFewer')
					: $t('ui.pages.usagePage.showAll', { values: { count: totalRows } })}
			</button>
		</div>
	{:else if query && rows.length > 0}
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.usagePage.showingGroups', { values: { shown, total: totalRows } })}
		</p>
	{/if}
{/if}
