<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { page } from '$app/state';
	import type { TransitionConfig } from 'svelte/transition';

	import { api } from '$lib/api';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import FilterPanel from '$lib/components/ui/filter-panel.svelte';
	import FilterToggle from '$lib/components/ui/filter-toggle.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Select from '$lib/components/ui/select.svelte';
	import DonutCard from '$lib/components/usage/donut-card.svelte';
	import GroupTable from '$lib/components/usage/group-table.svelte';
	import HeatmapCard from '$lib/components/usage/heatmap-card.svelte';
	import InsightCards from '$lib/components/usage/insight-cards.svelte';
	import LatencyCard from '$lib/components/usage/latency-card.svelte';
	import MetricCards from '$lib/components/usage/metric-cards.svelte';
	import RankTable from '$lib/components/usage/rank-table.svelte';
	import ReliabilityCard from '$lib/components/usage/reliability-card.svelte';
	import ShareCard from '$lib/components/usage/share-card.svelte';
	import TokenMixCard from '$lib/components/usage/token-mix-card.svelte';
	import TrendCard from '$lib/components/usage/trend-card.svelte';
	import { Tabs } from 'bits-ui';
	import TabStrip from '$lib/components/ui/tab-strip.svelte';
	import { prefersReducedMotion, tabDirection, tabPaneIn, tabPaneOut } from '$lib/tab-motion';
	import { accountCaption, namedCaption } from '$lib/account-label';
	import { spendKindOf } from '$lib/dashboard';
	import {
		FILTER_PANEL_KEYS,
		browserStorage,
		filterPanelId,
		writePanelOpen
	} from '$lib/filter-panel';
	import { formatCost, formatTimestamp, formatTokens, formatTokensExact } from '$lib/format';
	import {
		CHART_METRICS,
		bucketDays,
		donutSegments,
		heatmapCells,
		latencySeries,
		metricValueOf,
		rankedShares,
		reliabilitySeries,
		tokenMixSeries,
		type ChartMetric
	} from '$lib/usage-charts';
	import {
		DEFAULT_USAGE_FILTERS,
		activeFilterCount,
		clearRollupSpend,
		clearSummarySpend,
		mergeRollups,
		nonPaidProviderIds,
		planProviderIds,
		providerPlanMicros,
		providerReadChunks,
		spendScopeOf,
		splitSpend,
		statusParam,
		subtractRollupSpend,
		subtractSummarySpend,
		tabGroup,
		totalsOf,
		usageProvidersQuery,
		usageQuery,
		isUsageOrigin,
		isUsageRange,
		isUsageTab,
		USAGE_TABS,
		type SpendSplit,
		type UsageFilters,
		type UsageStatusFilter,
		type UsageSummary,
		type UsageTab
	} from '$lib/usage-filters';
	import {
		defaultSort,
		parseSort,
		searchUsageRows,
		shareSegments,
		sortParam,
		sortUsageRows,
		usageInsights,
		type UsageListRow,
		type UsageSort,
		type UsageSortKey
	} from '$lib/usage-list';
	import {
		chosenMetrics,
		metricLabelKey,
		metricValues,
		type SpendReadout
	} from '$lib/usage-metrics';
	import type {
		Account,
		AccessKey,
		Provider,
		UsageMetricsChoice,
		UsageRead,
		UsageRollupRow
	} from '$lib/types';

	// The tabs the page shows, and the grouping each one reads.
	const tabLabels: Record<UsageTab, string> = {
		overview: 'tabOverview',
		day: 'tabDay',
		provider: 'tabProvider',
		model: 'tabModel',
		account: 'tabAccount',
		client: 'tabClient'
	};

	// TAB_ICONS gives every tab its mark, so the strip reads at a glance
	// even before the label is read.
	const TAB_ICONS: Record<UsageTab, IconName> = {
		overview: 'layout-dashboard',
		day: 'clock',
		provider: 'server',
		model: 'cpu',
		account: 'user',
		client: 'key'
	};

	// stripTabs feeds the shared tab strip: the same icon and label every
	// pane is known by, in strip order.
	const stripTabs = $derived(
		USAGE_TABS.map((entry) => ({
			value: entry,
			label: $t('ui.pages.usagePage.' + tabLabels[entry]),
			icon: TAB_ICONS[entry]
		}))
	);

	const initialTab = page.url.searchParams.get('tab');
	const startingTab: UsageTab =
		initialTab !== null && isUsageTab(initialTab) ? initialTab : 'overview';
	let tab = $state<UsageTab>(startingTab);
	let filters = $state<UsageFilters>({ ...DEFAULT_USAGE_FILTERS });
	let groups = $state.raw<UsageRollupRow[]>([]);
	// daily holds the trend the overview and the day tab draw. In all-time mode
	// both ask for the recent 30 days, because a bar per day of every year is
	// not a chart.
	let daily = $state.raw<UsageRollupRow[]>([]);
	// planDaily is the same days for the connections a plan covers. It lets a
	// plan-usage card draw its own line after the API trend removed plan cost.
	let planDaily = $state.raw<UsageRollupRow[]>([]);
	let byProvider = $state.raw<UsageRollupRow[]>([]);
	let byModel = $state.raw<UsageRollupRow[]>([]);
	let summary = $state.raw<UsageSummary | null>(null);
	let providers = $state.raw<Provider[]>([]);
	let accounts = $state.raw<Account[]>([]);
	let clients = $state.raw<AccessKey[]>([]);
	let loading = $state(true);
	let failure = $state('');
	// lastLoadedMs is when the numbers on screen were last read. The ledger
	// aggregates at write time, so every visit, tab and filter change re-asks
	// the daemon and this caption says how fresh the answer is.
	let lastLoadedMs = $state(0);

	// The metric choice lives in the shared startup file: the daemon reads it
	// back and the picker writes it.
	let chosen = $state<string[]>([]);
	let available = $state.raw<string[]>([]);
	let metricsMin = $state(6);
	let metricsMax = $state(10);
	let pickerOpen = $state(false);
	let draft = $state<string[]>([]);
	let savingMetrics = $state(false);
	let metricsFailure = $state('');

	// chartMetric is the series every chart on the page draws, and hoveredKey
	// names the bar a reader is pointing at, which the readout above the trend
	// answers with. Both are page state: neither is stored.
	let chartMetric = $state<ChartMetric>('spend');
	// spend divides one read's cost between API spend and plan usage, and
	// planByProvider gives each plan connection the plan usage its own row
	// reports, which is what a provider row shows instead of a dollar amount
	// API spend would have left out.
	let spend = $state<SpendSplit>({ apiSpendMicros: 0, planUsageMicros: 0, nonPaidMicros: 0 });
	let planByProvider = $state<Record<string, number>>({});

	// The grouped list narrows and orders the rows already read. The address
	// carries the query and sort, while expansion and Show all stay transient.
	let listQuery = $state(page.url.searchParams.get('q') ?? '');
	let listSort = $state<UsageSort>(
		parseSort(
			page.url.searchParams.get('sort'),
			defaultSort(startingTab === 'overview' ? 'day' : startingTab)
		)
	);
	let listShowAll = $state(false);
	let expandedKeys = $state<Set<string>>(new Set());
	let shareMetric = $state<'requests' | 'tokens' | 'spend' | 'errors'>('spend');

	// The account list is the one of the chosen connection, so a filter never
	// offers an account that belongs to a different one.
	const accountOptions = $derived(
		filters.provider
			? accounts.filter((account) => account.provider_id === filters.provider)
			: accounts
	);

	const listRows = $derived.by<UsageListRow[]>(() => {
		if (!groupTab) return [];
		return groups.map((row) => ({
			key: row.Key,
			row,
			label: groupCaption(row.Key),
			id: groupTitle(row.Key) ?? row.Key,
			searchText:
				tab === 'account'
					? providerLabelOf(accountProvider(row.Key))
					: tab === 'client'
						? clients.find((key) => key.id === row.Key)?.name
						: undefined,
			spend: spendReadout(row, tab === 'provider')
		}));
	});
	const filteredListRows = $derived(searchUsageRows(listRows, listQuery));
	const orderedListRows = $derived(sortUsageRows(filteredListRows, listSort));
	const visibleListRows = $derived(
		listQuery || listShowAll ? orderedListRows : orderedListRows.slice(0, 8)
	);
	const insights = $derived(usageInsights(listRows, 'spend'));
	const shares = $derived(shareSegments(listRows, shareMetric));
	// The share caption states the figure for the metric not being shared.
	const shareTotal = $derived(shares.reduce((sum, segment) => sum + segment.value, 0));
	const listSpendTotal = $derived(listRows.reduce((sum, entry) => sum + entry.spend.weight, 0));
	const shareOptions = $derived(
		(
			[
				['requests', 'chart.requests', 'list'],
				['tokens', 'chart.tokens', 'sparkles'],
				['spend', spendLabel(), 'currency-dollar'],
				['errors', 'chart.errors', 'circle-alert']
			] as const
		).map(([value, label, icon]) => ({
			value,
			label: label.startsWith('chart.') ? $t('ui.pages.usagePage.' + label) : label,
			icon
		}))
	);

	function accountProvider(id: string): string {
		return accounts.find((account) => account.id === id)?.provider_id ?? '';
	}

	function spendLabel(): string {
		return tab === 'provider'
			? $t('ui.pages.usagePage.estimatedValue')
			: $t('ui.pages.usagePage.metric.spend');
	}

	function formatShareValue(
		value: number,
		metric: 'requests' | 'tokens' | 'spend' | 'errors'
	): string {
		if (metric === 'spend') return formatCost(value);
		if (metric === 'tokens') return formatTokens(value);
		return Math.round(value).toLocaleString();
	}

	// The address names the local query and sort so a reload keeps the list.
	// Expansion and the row limit are not address state.
	function writeListAddress() {
		const url = new URL(window.location.href);
		if (tab === 'overview') url.searchParams.delete('tab');
		else url.searchParams.set('tab', tab);
		if (listQuery.trim()) url.searchParams.set('q', listQuery);
		else url.searchParams.delete('q');
		url.searchParams.set('sort', sortParam(listSort));
		window.history.replaceState(null, '', url);
	}

	function resetListState() {
		listQuery = '';
		listSort = defaultSort(tab === 'overview' ? 'day' : tab);
		listShowAll = false;
		expandedKeys = new Set();
		if (tab !== 'overview') shareMetric = 'spend';
		writeListAddress();
	}

	function chooseListSort(key: UsageSortKey) {
		listSort =
			listSort.key === key
				? { key, descending: !listSort.descending }
				: { key, descending: key !== 'name' };
		writeListAddress();
	}

	function chooseListSortValue(next: UsageSort) {
		listSort = next;
		writeListAddress();
	}

	function chooseListQuery(query: string) {
		listQuery = query;
		writeListAddress();
	}

	function toggleRow(key: string) {
		const next = new Set(expandedKeys);
		if (next.has(key)) next.delete(key);
		else next.add(key);
		expandedKeys = next;
	}

	async function loadOptions() {
		try {
			const [connectionList, accountList, keyList] = await Promise.all([
				api<{ items: Provider[] }>('/connections'),
				api<{ items: Account[] }>('/accounts'),
				api<{ items: AccessKey[] }>('/clients/keys')
			]);
			providers = connectionList.items ?? [];
			accounts = accountList.items ?? [];
			clients = keyList.items ?? [];
		} catch {
			// A filter source that fails leaves the select empty rather than
			// blocking the totals, which do not depend on it.
			providers = [];
			accounts = [];
			clients = [];
		}
	}

	async function loadMetrics() {
		try {
			const choice = await api<UsageMetricsChoice>('/ui/usage-metrics');
			available = choice.available ?? [];
			metricsMin = choice.min ?? 6;
			metricsMax = choice.max ?? 10;
			chosen = chosenMetrics(choice.metrics ?? [], available);
		} catch {
			// A metric choice that cannot be read leaves the overview empty
			// rather than blocking the totals beside it.
			available = [];
		}
	}

	async function load() {
		loading = true;
		failure = '';
		const group = tabGroup(tab);
		const scope = spendScopeOf(filters, providers);
		const planIds = planProviderIds(providers);
		const nonPaidIds = nonPaidProviderIds(providers);
		// One connection's own rows answer its own split, so only a filter that
		// spans connections needs the plan and non-paid rollups read.
		const splits = scope === 'all' && nonPaidIds.length > 0;
		try {
			const now = Date.now();
			const totals = await api<UsageSummary>(
				'/activity/usage/summary?' + usageQuery(filters, now, false)
			);
			summary = totals;
			// The trend draws the chosen range; the readout is at day grain.
			const trendFilters = {
				...filters,
				groupBy: 'day' as const
			};
			const wantsTrend = group === null || group === 'day';
			// A set of connections wider than one read accepts is read in
			// chunks, and the rows add up across them. A refused read would
			// answer nothing, which reports a plan's traffic as API spend.
			const readProviders = (
				readFilters: UsageFilters,
				ids: readonly string[]
			): Promise<UsageRead[]> =>
				Promise.all(
					providerReadChunks(ids).map((chunk) =>
						api<UsageRead>('/activity/usage?' + usageProvidersQuery(readFilters, now, chunk))
					)
				);
			// The overview and the day tab draw the trend. The top connections
			// and the top models belong to the overview, which is the one tab
			// that shows them. A tab that draws no trend keeps the one it has,
			// because the overview beside it is still mounted and a cleared
			// series would flash an empty chart behind the tab that is open.
			const [
				trend,
				providerRows,
				modelRows,
				rollup,
				planProviders,
				planTrend,
				nonPaidProviders,
				nonPaidTrend,
				nonPaidModels,
				nonPaidGroups
			] = await Promise.all([
				wantsTrend ? api<UsageRead>('/activity/usage?' + usageQuery(trendFilters, now)) : null,
				group === null
					? api<UsageRead>(
							'/activity/usage?' + usageQuery({ ...filters, groupBy: 'provider' }, now)
						)
					: null,
				group === null
					? api<UsageRead>('/activity/usage?' + usageQuery({ ...filters, groupBy: 'model' }, now))
					: null,
				group === null
					? null
					: api<UsageRead>('/activity/usage?' + usageQuery({ ...filters, groupBy: group }, now)),
				splits && planIds.length > 0
					? readProviders({ ...filters, groupBy: 'provider' }, planIds)
					: null,
				splits && group === null && planIds.length > 0
					? readProviders(trendFilters, planIds)
					: null,
				splits ? readProviders({ ...filters, groupBy: 'provider' }, nonPaidIds) : null,
				splits && wantsTrend && group !== 'day' ? readProviders(trendFilters, nonPaidIds) : null,
				splits && group === null
					? readProviders({ ...filters, groupBy: 'model' }, nonPaidIds)
					: null,
				splits && group !== null && group !== 'provider'
					? readProviders({ ...filters, groupBy: group }, nonPaidIds)
					: null
			]);
			if (trend) {
				daily = trend.items ?? [];
			}
			if (group === null) {
				byProvider = providerRows?.items ?? [];
				byModel = modelRows?.items ?? [];
				groups = [];
			} else {
				groups = rollup?.items ?? [];
				byProvider = [];
				byModel = [];
			}
			planDaily = mergeRollups((planTrend ?? []).map((read) => read.items ?? []));
			planByProvider = {};
			if (scope === 'plan') {
				// The filter names one connection a plan covers, so the whole
				// summary is plan usage and no API spend at all.
				const planMicros = Number(summary?.cost_micros ?? 0);
				spend = { apiSpendMicros: 0, planUsageMicros: planMicros, nonPaidMicros: planMicros };
				planByProvider = { [filters.provider]: planMicros };
				planDaily = daily;
				summary = clearSummarySpend(summary);
				if (wantsTrend) daily = clearRollupSpend(daily);
				byProvider = clearRollupSpend(byProvider);
				byModel = clearRollupSpend(byModel);
				groups = clearRollupSpend(groups);
			} else if (scope === 'local') {
				// A local engine prices nothing that either figure counts.
				spend = { apiSpendMicros: 0, planUsageMicros: 0, nonPaidMicros: 0 };
				planDaily = [];
				summary = clearSummarySpend(summary);
				if (wantsTrend) daily = clearRollupSpend(daily);
				byProvider = clearRollupSpend(byProvider);
				byModel = clearRollupSpend(byModel);
				groups = clearRollupSpend(groups);
			} else if (splits) {
				// Each set is read in as many chunks as the cap allows, so the
				// rows are the whole set however wide it is.
				const planRows = mergeRollups((planProviders ?? []).map((read) => read.items ?? []));
				const nonPaidRows = mergeRollups((nonPaidProviders ?? []).map((read) => read.items ?? []));
				spend = splitSpend(Number(summary?.cost_micros ?? 0), nonPaidRows, providers);
				planByProvider = providerPlanMicros(planRows, providers);
				summary = subtractSummarySpend(summary, spend.nonPaidMicros);
				const nonPaidDays = mergeRollups((nonPaidTrend ?? []).map((read) => read.items ?? []));
				const nonPaidDayRows = mergeRollups((nonPaidGroups ?? []).map((read) => read.items ?? []));
				if (wantsTrend) {
					daily = subtractRollupSpend(daily, group === 'day' ? nonPaidDayRows : nonPaidDays);
				}
				planDaily = mergeRollups((planTrend ?? []).map((read) => read.items ?? []));
				if (group === null) {
					byProvider = subtractRollupSpend(byProvider, nonPaidRows);
					byModel = subtractRollupSpend(
						byModel,
						mergeRollups((nonPaidModels ?? []).map((read) => read.items ?? []))
					);
				} else if (group === 'provider') {
					// The rows themselves keep the plan figure for their spend
					// cell, so the subtraction only reaches the comparison bars.
					groups = subtractRollupSpend(groups, nonPaidRows);
				} else {
					groups = subtractRollupSpend(groups, nonPaidDayRows);
				}
			} else {
				// Only pay-as-you-go connections, so the summary is API spend.
				spend = {
					apiSpendMicros: Number(summary?.cost_micros ?? 0),
					planUsageMicros: 0,
					nonPaidMicros: 0
				};
				planDaily = [];
			}
			lastLoadedMs = now;
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
			groups = [];
			daily = [];
			planDaily = [];
			byProvider = [];
			byModel = [];
			summary = null;
			spend = { apiSpendMicros: 0, planUsageMicros: 0, nonPaidMicros: 0 };
			planByProvider = {};
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		void (async () => {
			await Promise.all([loadOptions(), loadMetrics()]);
			await load();
		})();
	});

	// chooseTab moves the page between the overview and one grouping, and
	// reads that tab's rows. tabDir names the travel direction of the swap.
	let tabDir = $state<1 | -1>(1);
	// tabbed turns the pane travel on after the first swap, so a fresh
	// visit paints still and only a tab change slides.
	let tabbed = $state(false);
	function chooseTab(next: string | undefined) {
		if (next === undefined || next === tab || !isUsageTab(next)) return;
		tabDir = tabDirection(USAGE_TABS, tab, next);
		tabbed = true;
		tab = next;
		resetListState();
		void load();
	}

	function paneIntro(node: HTMLElement): TransitionConfig {
		if (!tabbed) return { duration: 0 };
		return tabPaneIn(node, { direction: tabDir, reducedMotion: prefersReducedMotion() });
	}

	// The swap travels both ways instead of the old pane vanishing under the new
	// one.
	function paneOutro(node: HTMLElement): TransitionConfig {
		if (!tabbed) return { duration: 0 };
		return tabPaneOut(node, { direction: tabDir, reducedMotion: prefersReducedMotion() });
	}

	function chooseRange(next: string) {
		if (!isUsageRange(next)) return;
		if (next === filters.range) return;
		filters.range = next;
		expandedKeys = new Set();
		void load();
	}

	// The two selects never name a pair that cannot exist.
	function chooseProvider(next: string) {
		filters.provider = next;
		if (next && filters.account) {
			const held = accounts.find((account) => account.id === filters.account);
			if (held && held.provider_id !== next) filters.account = '';
		}
		expandedKeys = new Set();
		void load();
	}

	function chooseStatus(next: UsageStatusFilter) {
		filters.status = next;
		expandedKeys = new Set();
		void load();
	}

	function statusChoice(value: string): UsageStatusFilter {
		if (value === '2xx') return 'ok';
		if (value === '4xx,5xx') return 'error';
		return '';
	}

	function clearFilters() {
		filters = {
			...filters,
			provider: '',
			account: '',
			model: '',
			client: '',
			origin: '',
			status: ''
		};
		expandedKeys = new Set();
		void load();
	}

	// The page owns the filter toggle, which opens the fields grid below its
	// toolbar row. Which way it stood is remembered in this browser.
	let filtersOpen = $state(false);

	function toggleFilters() {
		filtersOpen = !filtersOpen;
		writePanelOpen(browserStorage(), FILTER_PANEL_KEYS.usage, filtersOpen);
	}

	function openPicker() {
		draft = [...chosen];
		metricsFailure = '';
		pickerOpen = true;
	}

	function toggleMetric(id: string) {
		if (draft.includes(id)) {
			draft = draft.filter((value) => value !== id);
			return;
		}
		draft = [...draft, id];
	}

	function moveMetric(index: number, offset: number) {
		const next = [...draft];
		const target = index + offset;
		if (target < 0 || target >= next.length) return;
		[next[index], next[target]] = [next[target], next[index]];
		draft = next;
	}

	async function saveMetrics() {
		savingMetrics = true;
		metricsFailure = '';
		try {
			const stored = await api<UsageMetricsChoice>('/ui/usage-metrics', {
				method: 'PUT',
				body: JSON.stringify({ metrics: draft })
			});
			chosen = chosenMetrics(stored.metrics ?? draft, stored.available ?? available);
			pickerOpen = false;
		} catch (error) {
			metricsFailure = error instanceof Error ? error.message : String(error);
		} finally {
			savingMetrics = false;
		}
	}

	const totals = $derived(totalsOf(summary, spend.planUsageMicros));
	const spendKinds = $derived(
		new Map(providers.map((provider) => [provider.id, spendKindOf(provider)]))
	);

	// A provider row names one connection, so it states what that connection
	// costs: API spend when the connection pays per token, plan usage when a plan
	// covers it, and nothing at all when a local engine made the request. Every
	// other grouping mixes connections, so it states the API spend the non-paid
	// rows left.
	function rowSpendMicros(row: UsageRollupRow, providerRow: boolean): number {
		if (!providerRow) return Number(row.CostMicros);
		const kind = spendKinds.get(row.Key);
		if (kind === 'plan') return planByProvider[row.Key] ?? 0;
		if (kind === 'local') return 0;
		return Number(row.CostMicros);
	}

	// A dash stands where a local engine made the request: formatCost of zero is
	// $0.00, which would read as a price a local engine does not charge.
	function spendText(row: UsageRollupRow, providerRow: boolean): string {
		if (providerRow && spendKinds.get(row.Key) === 'local') return '—';
		return formatCost(rowSpendMicros(row, providerRow));
	}

	function spendReadout(row: UsageRollupRow, providerRow: boolean): SpendReadout {
		return { text: spendText(row, providerRow), weight: rowSpendMicros(row, providerRow) };
	}

	// The retried card is hidden from the overview; the picker and the config
	// keep offering it, so an operator's stored choice is untouched.
	const cards = $derived(metricValues(chosen, totals).filter((card) => card.id !== 'retried'));
	// The trend reads the newest day last, so the bars walk left to right over
	// whatever range the page is showing. A window wider than the chart holds
	// shares its columns, so every total stays exact.
	const trendDays = $derived(bucketDays(daily));
	// The plan cards need the same day grain after the API trend dropped plan
	// cost, so they read the plan rollup's own days.
	const planDays = $derived(bucketDays(planDaily));
	// The four overview charts all read the trend's own days, so no extra read
	// is needed to draw them.
	const mixPoints = $derived(tokenMixSeries(trendDays));
	const relPoints = $derived(reliabilitySeries(trendDays));
	const latPoints = $derived(latencySeries(trendDays));
	// The overview donut ranks connections by the tokens they carried; the
	// spend share beside it ranks them by cost. Both read the same rollup, so
	// neither chart needs a read of its own.
	const spendShares = $derived(
		rankedShares(byProvider.map((row) => ({ key: row.Key, value: rowSpendMicros(row, true) })))
	);
	const tokenShares = $derived(
		rankedShares(byProvider.map((row) => ({ key: row.Key, value: metricValueOf(row, 'tokens') })))
	);
	const tokenDonut = $derived(donutSegments(tokenShares));
	const tokenDonutTotal = $derived(tokenShares.reduce((sum, share) => sum + share.value, 0));
	// donut lays the spend shares end to end around one turn, and heatWeeks
	// reads every day the range returned so the calendar keeps real dates.
	const donut = $derived(donutSegments(spendShares));
	const donutTotal = $derived(spendShares.reduce((sum, share) => sum + share.value, 0));
	const heatWeeks = $derived(
		heatmapCells(daily.map((row) => ({ key: row.Key, value: Number(row.Requests) })))
	);
	const chartOptions = $derived(
		CHART_METRICS.map((metric): { value: ChartMetric; label: string; icon: IconName } => ({
			value: metric,
			label: $t('ui.pages.usagePage.chart.' + metric),
			icon:
				metric === 'requests'
					? 'list'
					: metric === 'tokens'
						? 'sparkles'
						: metric === 'spend'
							? 'currency-dollar'
							: metric === 'errors'
								? 'circle-alert'
								: 'clock'
		}))
	);
	const chartMetricLabel = $derived($t('ui.pages.usagePage.chart.' + chartMetric));
	// The top lists read the same rollups the overview already loaded.
	const topProviders = $derived(topBy(byProvider, true));
	const topModels = $derived(topBy(byModel, false));
	const activeFilters = $derived(activeFilterCount(filters));
	const statusValue = $derived(statusParam(filters.status));
	const groupTab = $derived(tabGroup(tab) !== null);
	const draftValid = $derived(draft.length >= metricsMin && draft.length <= metricsMax);

	function providerLabelOf(id: string): string {
		if (id === '') return '';
		const provider = providers.find((entry) => entry.id === id);
		return provider?.label?.trim() || provider?.template_id || id;
	}

	function providerLogoId(id: string): string {
		return providers.find((provider) => provider.id === id)?.template_id || id;
	}

	// Account and provider rows prefer the stored label, and an account is
	// "name (provider)". Other groupings keep the key they grouped by.
	function groupCaption(key: string): string {
		if (tab === 'account') {
			const account = accounts.find((entry) => entry.id === key);
			return namedCaption(key, account?.label, providerLabelOf(account?.provider_id ?? ''));
		}
		if (tab === 'provider') return namedCaption(key, providerLabelOf(key) || key);
		if (tab === 'client') {
			const client = clients.find((entry) => entry.id === key);
			return client?.name?.trim() || key || '—';
		}
		return key || '—';
	}

	function groupTitle(key: string): string | undefined {
		if (key === '') return undefined;
		return groupCaption(key) !== key ? key : undefined;
	}

	// A provider list states what each connection costs, and every other list
	// states the API spend its rows carry.
	function topBy(rows: UsageRollupRow[], providerRow: boolean): UsageRollupRow[] {
		return [...rows]
			.filter((row) => row.Key !== '')
			.sort(
				(a, b) =>
					rowSpendMicros(b, providerRow) - rowSpendMicros(a, providerRow) || b.Requests - a.Requests
			)
			.slice(0, 5);
	}
</script>

<svelte:head><title>{$t('ui.pages.usage.title')} · Relo</title></svelte:head>

<PageHeader title="ui.pages.usage.title" description="ui.pages.usage.description">
	{#snippet meta()}
		{#if lastLoadedMs === 0}
			{$t('ui.common.loading')}
		{:else}
			{$t('ui.pages.usagePage.updatedAt', { values: { time: formatTimestamp(lastLoadedMs) } })}
		{/if}
	{/snippet}
	{#snippet actions()}
		<IconAction
			icon="refresh"
			label={$t('ui.pages.usagePage.refresh')}
			variant="outline"
			onclick={() => void load()}
			disabled={loading}
			spin={loading}
		/>
	{/snippet}
</PageHeader>

<div class="page-frame page-stack pt-6">
	<Tabs.Root value={tab} onValueChange={chooseTab} class="flex flex-col gap-5">
		<div
			class="toolbar-row [&_.toolbar-end]:flex-wrap max-sm:[&_.toolbar-end]:w-full max-sm:[&_.toolbar-start]:w-full max-sm:[&_.toolbar-center]:order-last max-sm:[&_.toolbar-center]:col-span-full"
		>
			<div class="toolbar-start">
				<FilterToggle
					open={filtersOpen}
					activeCount={activeFilters}
					controls={filterPanelId(FILTER_PANEL_KEYS.usage)}
					onclick={toggleFilters}
				/>
				{#if activeFilters > 0}
					<Button variant="ghost" size="sm" onclick={clearFilters}>
						<Icon name="refresh" size={14} />
						{$t('ui.pages.usagePage.clearFilters')}
					</Button>
				{/if}
			</div>
			<div class="toolbar-center">
				<TabStrip tabs={stripTabs} value={tab} ariaLabel={$t('ui.pages.usagePage.groupBy')} />
			</div>
			<div class="toolbar-end">
				<Select
					id="usage-range"
					ariaLabel={$t('ui.pages.usagePage.rangeLabel')}
					class="w-40"
					value={filters.range}
					onValueChange={(next) => chooseRange(next)}
					options={[
						{ value: '24h', label: $t('ui.common.window24h') },
						{ value: 'today', label: $t('ui.common.windowToday') },
						{ value: '7', label: $t('ui.pages.usagePage.range7') },
						{ value: '30', label: $t('ui.pages.usagePage.range30') },
						{ value: '60', label: $t('ui.common.window60d') },
						{ value: '90', label: $t('ui.common.window90d') },
						{ value: 'all', label: $t('ui.pages.usagePage.rangeAll') }
					]}
				/>
				{#if tab === 'overview'}
					<IconAction
						icon="selector"
						label={$t('ui.pages.usagePage.chooseMetrics')}
						variant="outline"
						onclick={openPicker}
					/>
				{/if}
			</div>
		</div>

		<FilterPanel storageKey={FILTER_PANEL_KEYS.usage} columns={4} bind:open={filtersOpen}>
			<Field id="usage-provider" label={$t('ui.pages.usagePage.filterProvider')}>
				<Select
					id="usage-provider"
					value={filters.provider}
					onValueChange={(next) => chooseProvider(next)}
					options={[
						{ value: '', label: $t('ui.pages.usagePage.filterAll') },
						...providers.map((provider) => ({
							value: provider.id,
							label: provider.label || provider.id
						}))
					]}
				/>
			</Field>
			<Field id="usage-account" label={$t('ui.pages.usagePage.filterAccount')}>
				<Select
					id="usage-account"
					value={filters.account}
					onValueChange={(next) => {
						filters.account = next;
						void load();
					}}
					options={[
						{ value: '', label: $t('ui.pages.usagePage.filterAll') },
						...accountOptions.map((account) => ({
							value: account.id,
							label: accountCaption(account.id, account.label, providerLabelOf(account.provider_id))
						}))
					]}
				/>
			</Field>
			<Field id="usage-model" label={$t('ui.pages.usagePage.filterModel')}>
				<Input
					id="usage-model"
					bind:value={filters.model}
					onchange={() => load()}
					placeholder="gpt-5.5"
				/>
			</Field>
			<Field id="usage-client" label={$t('ui.pages.usagePage.filterClient')}>
				<Select
					id="usage-client"
					value={filters.client}
					onValueChange={(next) => {
						filters.client = next;
						void load();
					}}
					options={[
						{ value: '', label: $t('ui.pages.usagePage.filterAll') },
						...clients.map((key) => ({ value: key.id, label: key.name || key.id }))
					]}
				/>
			</Field>
			<Field id="usage-origin" label={$t('ui.pages.usagePage.filterOrigin')}>
				<Select
					id="usage-origin"
					value={filters.origin}
					onValueChange={(next) => {
						if (!isUsageOrigin(next)) return;
						filters.origin = next;
						void load();
					}}
					options={[
						{ value: '', label: $t('ui.pages.usagePage.filterAll') },
						{ value: 'external', label: $t('ui.pages.usagePage.originExternal') },
						{ value: 'internal', label: $t('ui.pages.usagePage.originInternal') }
					]}
				/>
			</Field>
			<Field id="usage-status" label={$t('ui.pages.usagePage.filterStatus')}>
				<Select
					id="usage-status"
					value={statusValue}
					onValueChange={(next) => chooseStatus(statusChoice(next))}
					options={[
						{ value: '', label: $t('ui.pages.usagePage.filterAll') },
						{ value: '2xx', label: $t('ui.pages.usagePage.statusOk') },
						{ value: '4xx,5xx', label: $t('ui.pages.usagePage.statusError') }
					]}
				/>
			</Field>
		</FilterPanel>

		{#if loading && lastLoadedMs === 0}
			<p class="text-sm text-muted-foreground">{$t('ui.common.loading')}</p>
		{:else if failure && summary === null}
			<p
				class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
				role="alert"
			>
				{failure}
			</p>
		{:else if summary === null || summary.requests === 0}
			<div class="empty-panel section-surface border-dashed">{$t('ui.pages.usagePage.empty')}</div>
		{:else}
			<Tabs.Content value="overview" class="flex flex-col gap-5 outline-none">
				{#if tab === 'overview'}
					<div in:paneIntro out:paneOutro class="flex flex-col gap-5">
						<MetricCards
							{cards}
							days={trendDays}
							{planDays}
							sparklineNote={filters.range === 'all'
								? $t('ui.pages.usagePage.sparklineWindow')
								: ''}
						/>

						<TrendCard
							days={trendDays}
							metric={chartMetric}
							metricLabel={chartMetricLabel}
							options={chartOptions}
							onmetric={(next) => (chartMetric = next)}
						/>

						<div class="grid items-stretch gap-[var(--grid-gap)] lg:grid-cols-2">
							<TokenMixCard points={mixPoints} />
							<ReliabilityCard points={relPoints} />
							<LatencyCard days={trendDays} points={latPoints} />
							<DonutCard
								titleKey="ui.pages.usagePage.spendShareTitle"
								segments={donut}
								totalText={formatCost(donutTotal)}
								captionOf={(key) => namedCaption(key, providerLabelOf(key))}
								logoOf={providerLogoId}
								valueOf={formatCost}
								centerKey="ui.pages.usagePage.summaryTotalTokens"
							/>
							<DonutCard
								titleKey="ui.pages.usagePage.tokenByProviderTitle"
								segments={tokenDonut}
								totalText={formatTokens(tokenDonutTotal)}
								captionOf={(key) => namedCaption(key, providerLabelOf(key))}
								logoOf={providerLogoId}
								valueOf={formatTokens}
								centerKey="ui.pages.usagePage.summaryTotalTokens"
							/>
							<HeatmapCard weeks={heatWeeks} />
						</div>

						<div class="grid items-stretch gap-[var(--grid-gap)] lg:grid-cols-2">
							<RankTable
								titleKey="ui.pages.usagePage.topProviders"
								rows={topProviders}
								captionOf={(row) => namedCaption(row.Key, providerLabelOf(row.Key))}
								spendOf={(row) => spendReadout(row, true)}
							/>
							<RankTable
								titleKey="ui.pages.usagePage.topModels"
								rows={topModels}
								captionOf={(row) => row.Key}
								spendOf={(row) => spendReadout(row, false)}
							/>
						</div>
					</div>
				{/if}
			</Tabs.Content>

			{#if groupTab}
				<Tabs.Content value={tab} class="flex flex-col gap-4 outline-none">
					{#key tab}
						<div in:paneIntro out:paneOutro class="flex flex-col gap-4">
							<div class="flex flex-wrap items-baseline gap-x-5 gap-y-2">
								<p class="text-sm text-muted-foreground">
									<span class="font-semibold text-foreground tabular-nums"
										>{totals.requests.toLocaleString()}</span
									>
									{$t('ui.pages.usagePage.unitRequests')}
									{#if totals.errors > 0}
										<span class="text-destructive">
											· {totals.errors.toLocaleString()}
											{$t('ui.pages.usagePage.summaryErrors')}</span
										>
									{/if}
								</p>
								<p class="text-sm text-muted-foreground">
									<span
										class="font-semibold text-foreground tabular-nums"
										title={formatTokensExact(totals.totalTokens)}
										>{formatTokens(totals.totalTokens)}</span
									>
									{$t('ui.pages.usagePage.unitTokens')}
								</p>
								<p class="text-sm text-muted-foreground">
									<span class="font-semibold text-foreground tabular-nums"
										>{formatCost(totals.spendMicros)}</span
									>
									{$t('ui.pages.usagePage.metric.spend')}
									<span class="font-semibold text-foreground tabular-nums"
										>· {formatCost(totals.planUsageMicros)}</span
									>
									{$t('ui.pages.usagePage.metric.plan_usage')}
								</p>
							</div>

							{#if groups.length === 0}
								<div class="empty-panel section-surface border-dashed">
									{$t('ui.pages.usagePage.empty')}
								</div>
							{:else}
								{#if tab === 'day'}
									<TrendCard
										days={trendDays}
										metric={chartMetric}
										metricLabel={chartMetricLabel}
										options={chartOptions}
										onmetric={(next) => (chartMetric = next)}
									/>
								{:else}
									<ShareCard
										segments={shares}
										total={formatShareValue(shareTotal, shareMetric)}
										metricLabel={$t(
											shareMetric === 'spend' && tab === 'provider'
												? 'ui.pages.usagePage.estimatedValue'
												: shareMetric === 'spend'
													? 'ui.pages.usagePage.metric.spend'
													: 'ui.pages.usagePage.chart.' + shareMetric
										)}
										metricValue={shareMetric}
										options={shareOptions}
										formatValue={formatShareValue}
										groupLabel={$t(
											tab === 'provider'
												? 'ui.pages.usagePage.tabProvider'
												: tab === 'model'
													? 'ui.pages.usagePage.tabModel'
													: tab === 'account'
														? 'ui.pages.usagePage.tabAccount'
														: 'ui.pages.usagePage.tabClient'
										)}
										onmetric={(metric) => (shareMetric = metric)}
									/>
								{/if}

								<InsightCards {insights} shareLabel={spendLabel()} />

								<GroupTable
									rows={visibleListRows}
									total={listSpendTotal}
									spendLabel={spendLabel()}
									sort={listSort}
									query={listQuery}
									shown={visibleListRows.length}
									totalRows={listRows.length}
									showAll={listShowAll}
									isDay={tab === 'day'}
									onsort={chooseListSort}
									onchoosesort={chooseListSortValue}
									onquery={chooseListQuery}
									ontoggleall={() => (listShowAll = !listShowAll)}
									onexpand={toggleRow}
									expanded={expandedKeys}
								/>
							{/if}
						</div>
					{/key}
				</Tabs.Content>
			{/if}

			{#if totals.unpricedRequests > 0}
				<p class="text-xs text-muted-foreground">
					{totals.unpricedRequests.toLocaleString()}
					{$t('ui.pages.usagePage.unpriced')} · {$t('ui.pages.usagePage.estimateNote')}
				</p>
			{/if}
		{/if}
	</Tabs.Root>
</div>

<CenteredModal
	bind:open={pickerOpen}
	size="standard"
	title={$t('ui.pages.usagePage.metricsTitle')}
	description={$t('ui.pages.usagePage.metricsDescription', {
		values: { min: metricsMin, max: metricsMax }
	})}
>
	<div class="space-y-4">
		{#if metricsFailure}
			<p
				class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive"
				role="alert"
			>
				{metricsFailure}
			</p>
		{/if}

		<div class="rounded-panel bg-sunken p-3">
			<p class="text-[0.8125rem] font-medium text-muted-foreground">
				{$t('ui.pages.usagePage.metricsChosen', {
					values: { count: draft.length, max: metricsMax }
				})}
			</p>
			{#if draft.length === 0}
				<p class="mt-2 text-sm text-muted-foreground">{$t('ui.pages.usagePage.metricsNone')}</p>
			{:else}
				<ul class="mt-2 space-y-1">
					{#each draft as id, index (id)}
						<li
							class="flex items-center gap-2 rounded-md border border-line bg-surface px-2 py-1.5"
						>
							<span class="flex-1 truncate text-sm">{$t(metricLabelKey(id))}</span>
							<Button
								variant="ghost"
								size="icon"
								aria-label={$t('ui.pages.usagePage.metricsMoveUp')}
								onclick={() => moveMetric(index, -1)}
								disabled={index === 0}
							>
								<Icon name="arrow-up" size={14} />
							</Button>
							<Button
								variant="ghost"
								size="icon"
								aria-label={$t('ui.pages.usagePage.metricsMoveDown')}
								onclick={() => moveMetric(index, 1)}
								disabled={index === draft.length - 1}
							>
								<Icon name="arrow-down" size={14} />
							</Button>
							<Button
								variant="ghost"
								size="icon"
								aria-label={$t('ui.pages.usagePage.metricsRemove')}
								onclick={() => toggleMetric(id)}
							>
								<Icon name="minus" size={14} />
							</Button>
						</li>
					{/each}
				</ul>
			{/if}
		</div>

		<div class="rounded-panel bg-sunken p-3">
			<p class="text-[0.8125rem] font-medium text-muted-foreground">
				{$t('ui.pages.usagePage.metricsAvailable')}
			</p>
			<div class="mt-2 flex flex-wrap gap-2">
				{#each available as id (id)}
					<Button
						variant="chip"
						size="sm"
						aria-pressed={draft.includes(id)}
						disabled={!draft.includes(id) && draft.length >= metricsMax}
						onclick={() => toggleMetric(id)}
					>
						{$t(metricLabelKey(id))}
					</Button>
				{/each}
			</div>
		</div>

		<p class="text-xs text-muted-foreground">
			{#if draft.length < metricsMin}
				{$t('ui.pages.usagePage.metricsTooFew', { values: { min: metricsMin } })}
			{:else if draft.length > metricsMax}
				{$t('ui.pages.usagePage.metricsTooMany', { values: { max: metricsMax } })}
			{:else}
				{$t('ui.pages.usagePage.metricsStored')}
			{/if}
		</p>

		<div class="flex flex-wrap items-center justify-end gap-2">
			<Button variant="outline" size="sm" onclick={() => (draft = [...chosen])}>
				<Icon name="refresh" size={14} />
				{$t('ui.pages.usagePage.metricsReset')}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => (pickerOpen = false)}>
				<Icon name="x" size={14} />
				{$t('ui.common.cancel')}
			</Button>
			<Button size="sm" onclick={saveMetrics} disabled={!draftValid || savingMetrics}>
				<Icon name={savingMetrics ? 'loader' : 'check'} size={14} spin={savingMetrics} />
				{savingMetrics ? $t('ui.common.saving') : $t('ui.common.save')}
			</Button>
		</div>
	</div>
</CenteredModal>
