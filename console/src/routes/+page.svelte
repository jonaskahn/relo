<script lang="ts">
	import { onDestroy, onMount, untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { coalesceReload } from '$lib/refresh';
	import { subscribe } from '$lib/sse';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import RoutingUniverse from '$lib/components/dashboard/routing-universe.svelte';
	import DoctorChecks from '$lib/components/dashboard/doctor-checks.svelte';
	import RecentRequests from '$lib/components/dashboard/recent-requests.svelte';
	import HealthBoard from '$lib/components/dashboard/health-board.svelte';
	import SpendBoard from '$lib/components/dashboard/spend-board.svelte';
	import TrendChart from '$lib/components/dashboard/trend-chart.svelte';
	import TokenMixDonut from '$lib/components/dashboard/token-mix-donut.svelte';
	import KpiTile from '$lib/components/dashboard/kpi-tile.svelte';
	import AnimatedNumber from '$lib/components/ui/animated-number.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { formatCost, formatTokens, formatTokensExact, formatUptime } from '$lib/format';
	import {
		deltaPercent,
		healthCards,
		paidProviders,
		planProviders,
		readDashboardLive,
		rowTokens,
		spendRows,
		summaryWindow,
		windowStart,
		writeDashboardLive,
		isWindowPeriod,
		type WindowPeriod
	} from '$lib/dashboard';
	import { browserStorage } from '$lib/filter-panel';
	import { bucketDays, type ChartMetric } from '$lib/usage-charts';
	import type {
		Account,
		DoctorCheck,
		Provider,
		QuotaWindow,
		StatusResponse,
		UsageLogRow,
		UsageRollupRow
	} from '$lib/types';

	let status = $state.raw<StatusResponse | null>(null);
	let providers = $state.raw<Provider[]>([]);
	let doctor = $state.raw<DoctorCheck[]>([]);
	// The doctor panel reads on its own: its integrity check scales with the
	// database, so the page must never wait on it to paint its figures.
	let doctorLoading = $state(true);
	let doctorFailed = $state('');
	let accounts = $state.raw<Account[]>([]);
	let quotaWindows = $state.raw<QuotaWindow[]>([]);
	let accountsLoading = $state(true);
	// The body stays a spinner until the first read settles. Later refreshes
	// keep the loaded page on screen.
	let contentLoading = $state(true);
	let healthFailed = $state('');
	// summaryRows and previousRows hold the selected summary window and the
	// equal window before it, so every KPI figure above the fold shares one
	// range. The trend keeps its own seven UTC days.
	let summaryRows = $state.raw<UsageRollupRow[]>([]);
	let previousRows = $state.raw<UsageRollupRow[]>([]);
	let dailyAll = $state.raw<UsageRollupRow[]>([]);
	// traffic24 is the routing universe's own rolling day: the map is defined
	// as 24-hour traffic, whatever window the summary above it shows.
	let traffic24 = $state.raw<UsageRollupRow[]>([]);
	let recent = $state.raw<UsageLogRow[]>([]);
	let activityFailed = $state('');
	// live tails the request log the same way the logs page does. The choice
	// lives in this browser so a reload keeps the last on or off. Fifteen rows
	// fill the card without scrolling past the blocks beside it.
	let live = $state(true);
	const RECENT_LIMIT = 15;
	let pulsingProviders = $state<Set<string>>(new Set());
	let pulseTimers = new Map<string, ReturnType<typeof setTimeout>>();

	// One window drives the whole windowed page: the KPI tiles, the paid-spend
	// board and the trends. The routing map, doctor and request tail keep their
	// own scopes below. The rolling 24 hours is the default the desktop window
	// mirrors.
	let summaryPeriod = $state<WindowPeriod>('24h');
	// spendSet chooses which figure the board reads: the connections that pay
	// per token, or the ones a plan covers.
	type SpendSet = 'api' | 'plan';
	// Reports whether a choice the control handed back is one the board reads.
	function isSpendSet(value: string): value is SpendSet {
		return value === 'api' || value === 'plan';
	}
	let spendSet = $state<SpendSet>('api');
	let spendRowsState = $state<UsageRollupRow[]>([]);
	let spendLoading = $state(false);
	// The board fades in only when a window or set swap lands its new rows:
	// boardShownKey names the choice the rendered rows belong to, so a plain
	// refresh keeps the board quiet.
	let boardShownKey = $state(untrack(() => summaryPeriod + '|' + spendSet));
	let boardSwapped = $state(false);
	let trendMode = $state<ChartMetric>('requests');
	let dailyPaid = $state<UsageRollupRow[]>([]);
	let expandedSpend = $state('');
	let spendKeys = $state<UsageRollupRow[]>([]);
	let spendKeysLoading = $state(false);

	const DAY = 86_400_000;

	const configuredProviders = $derived(providers.filter((provider) => provider.configured));
	const paid = $derived(paidProviders(providers).filter((provider) => provider.configured));
	const plan = $derived(planProviders(providers).filter((provider) => provider.configured));
	const boardSet = $derived(spendSet === 'api' ? paid : plan);
	const paidSpendBoard = $derived(spendRows(boardSet, spendRowsState));

	const total = (rows: UsageRollupRow[], pick: (row: UsageRollupRow) => number) =>
		rows.reduce((sum, row) => sum + pick(row), 0);
	const sumRequests = (rows: UsageRollupRow[]) => total(rows, (row) => Number(row.Requests));
	const sumErrors = (rows: UsageRollupRow[]) => total(rows, (row) => Number(row.Errors));
	const sumCost = (rows: UsageRollupRow[]) => total(rows, (row) => Number(row.CostMicros));
	const sumTokens = (rows: UsageRollupRow[]) => total(rows, rowTokens);

	const requestsWindow = $derived(sumRequests(summaryRows));
	const errorsWindow = $derived(sumErrors(summaryRows));
	const paidIds = $derived(new Set(paid.map((provider) => provider.id)));
	const paidSpendWindow = $derived(sumCost(summaryRows.filter((row) => paidIds.has(row.Key))));
	const tokensWindow = $derived(sumTokens(summaryRows));
	const durationSum = $derived(total(summaryRows, (row) => Number(row.DurationMs)));
	const avgDuration = $derived(requestsWindow > 0 ? durationSum / requestsWindow : 0);
	// A rate with no requests behind it has nothing to claim, so it reports
	// nothing rather than a perfect score — the desktop window's rule too.
	const successRate = $derived(
		requestsWindow > 0 ? ((requestsWindow - errorsWindow) / requestsWindow) * 100 : null
	);
	const requestsDelta = $derived(deltaPercent(requestsWindow, sumRequests(previousRows)));
	const tokensDelta = $derived(deltaPercent(tokensWindow, sumTokens(previousRows)));
	// The board the accounts tile and the health card read: one card per
	// account, the ones that need attention first.
	const healthBoard = $derived(healthCards(accounts, configuredProviders, quotaWindows));
	const healthLimited = $derived(
		healthBoard.filter((card) => card.health.state === 'limited').length
	);
	const healthNeedsSignin = $derived(
		healthBoard.filter((card) => card.health.state === 'needs_signin').length
	);
	const healthReady = $derived(
		healthBoard.filter((card) => ['ready', 'near', 'probing'].includes(card.health.state)).length
	);

	function accountWindows(accountId: string): QuotaWindow[] {
		return quotaWindows.filter((window) => window.credential_id === accountId);
	}

	// The trend follows the page window at day grain: one bar per UTC day the
	// read returned, newest last. A window wider than the chart holds shares
	// its columns, so every total stays exact.
	const trendRows = $derived(trendMode === 'spend' ? dailyPaid : dailyAll);
	const trendDays = $derived(bucketDays(trendRows));
	// The three series the trend switch offers. Spend reads the paid
	// connections whatever the spend board below is showing.
	const trendOptions: { value: ChartMetric; label: string; icon: IconName }[] = $derived([
		{ value: 'requests', label: $t('ui.pages.dashboard.trendRequests'), icon: 'list' },
		{ value: 'tokens', label: $t('ui.pages.dashboard.trendTokens'), icon: 'sparkles' },
		{ value: 'spend', label: $t('ui.pages.dashboard.trendSpend'), icon: 'currency-dollar' }
	]);
	// openedSpend is the board row behind the operator's last click, with what
	// its access-key read answered.
	const openedSpend = $derived(
		expandedSpend === ''
			? null
			: { providerId: expandedSpend, keys: spendKeys, loading: spendKeysLoading }
	);

	// The chosen connections are named only when they fit the daemon's filter
	// cap; a wider roster reads every connection and the join keeps the ones
	// asked for.
	function spendUrl(
		chosen: readonly Provider[],
		start: number,
		until = 0,
		groupBy: 'provider' | 'day' = 'provider'
	): string {
		let url = '/activity/usage?group_by=' + groupBy + '&since_ms=' + start;
		if (until > 0) url += '&until_ms=' + until;
		if (chosen.length > 0 && chosen.length <= 32) {
			for (const provider of chosen) url += '&provider=' + encodeURIComponent(provider.id);
		}
		return url;
	}

	async function loadStatus() {
		try {
			const [s, p] = await Promise.all([
				api<StatusResponse>('/status'),
				api<{ items: Provider[] }>('/connections')
			]);
			status = s;
			providers = p.items ?? [];
		} catch (error) {
			activityFailed = error instanceof Error ? error.message : String(error);
		}
	}

	async function loadDoctor() {
		doctorLoading = true;
		doctorFailed = '';
		try {
			const report = await api<{ checks: DoctorCheck[] }>('/doctor');
			doctor = report.checks ?? [];
		} catch (error) {
			doctorFailed = error instanceof Error ? error.message : String(error);
		} finally {
			doctorLoading = false;
		}
	}

	async function loadHealth() {
		accountsLoading = true;
		healthFailed = '';
		try {
			const [accountList, windows] = await Promise.all([
				api<{ items: Account[] }>('/accounts'),
				api<{ items: QuotaWindow[] }>('/activity/quota')
			]);
			accounts = accountList.items ?? [];
			quotaWindows = windows.items ?? [];
		} catch (error) {
			healthFailed = error instanceof Error ? error.message : String(error);
		} finally {
			accountsLoading = false;
		}
	}

	async function loadSpend() {
		spendLoading = true;
		boardSwapped = false;
		try {
			const start = windowStart(summaryPeriod, Date.now());
			spendRowsState =
				(await api<{ items: UsageRollupRow[] }>(spendUrl(boardSet, start))).items ?? [];
			const key = summaryPeriod + '|' + spendSet;
			if (key !== boardShownKey) {
				boardShownKey = key;
				boardSwapped = true;
			}
		} catch {
			spendRowsState = [];
		} finally {
			spendLoading = false;
		}
	}

	async function loadTrendPaid() {
		try {
			// The trend's spend series stays API spend, so it reads the paid
			// connections whatever the board is showing.
			dailyPaid =
				(
					await api<{ items: UsageRollupRow[] }>(
						spendUrl(paid, windowStart(summaryPeriod, Date.now()), 0, 'day')
					)
				).items ?? [];
		} catch {
			dailyPaid = [];
		}
	}

	async function loadRecent() {
		try {
			const recentRows = await api<{ items: UsageLogRow[] }>(
				'/activity/requests?limit=' + RECENT_LIMIT
			);
			recent = recentRows.items ?? [];
		} catch (error) {
			activityFailed = error instanceof Error ? error.message : String(error);
		}
	}

	// The equal window before the selected one is what a KPI delta compares
	// against, so switching the period re-asks only this read.
	async function loadSummary() {
		activityFailed = '';
		try {
			const window = summaryWindow(summaryPeriod, Date.now());
			const currentRead = api<{ items: UsageRollupRow[] }>(
				'/activity/usage?group_by=provider&since_ms=' + window.start
			);
			const previousRead =
				window.prevUntil > 0
					? api<{ items: UsageRollupRow[] }>(
							'/activity/usage?group_by=provider&since_ms=' +
								window.prevStart +
								'&until_ms=' +
								window.prevUntil
						)
					: null;
			const [current, previous] = await Promise.all([currentRead, previousRead]);
			summaryRows = current.items ?? [];
			previousRows = previous?.items ?? [];
		} catch (error) {
			activityFailed = error instanceof Error ? error.message : String(error);
		}
	}

	// The trend follows the page window at day grain: it asks for the same
	// days the KPI strip covers, so the bars and the tiles agree.
	async function loadTrend() {
		try {
			dailyAll =
				(
					await api<{ items: UsageRollupRow[] }>(
						'/activity/usage?group_by=day&since_ms=' + windowStart(summaryPeriod, Date.now())
					)
				).items ?? [];
		} catch (error) {
			activityFailed = error instanceof Error ? error.message : String(error);
		}
	}

	// The routing map reads the rolling day independently, so switching the
	// summary window never redraws it.
	async function loadTraffic() {
		try {
			traffic24 =
				(
					await api<{ items: UsageRollupRow[] }>(
						'/activity/usage?group_by=provider&since_ms=' + (Date.now() - DAY)
					)
				).items ?? [];
		} catch (error) {
			activityFailed = error instanceof Error ? error.message : String(error);
		}
	}

	async function loadEverything(includeRecent = live) {
		try {
			await Promise.all([
				loadStatus(),
				loadHealth(),
				loadSummary(),
				loadTrend(),
				loadTraffic(),
				includeRecent ? loadRecent() : Promise.resolve()
			]);
			await Promise.all([loadSpend(), loadTrendPaid()]);
		} finally {
			contentLoading = false;
		}
	}

	function setLive(next: boolean) {
		live = next;
		writeDashboardLive(browserStorage(), next);
	}

	function pulseProvider(id: string) {
		if (pulseTimers.has(id)) clearTimeout(pulseTimers.get(id));
		pulsingProviders = new Set([...pulsingProviders, id]);
		pulseTimers.set(
			id,
			setTimeout(() => {
				pulsingProviders = new Set([...pulsingProviders].filter((entry) => entry !== id));
				pulseTimers.delete(id);
			}, 600)
		);
	}

	let refreshTimer: ReturnType<typeof setInterval>;
	let unsubLogs: (() => void) | undefined;
	let unsubQuota: (() => void) | undefined;
	// The streams announce every relayed request, so the reloads they trigger
	// are coalesced: one reload covers a burst of announcements.
	const reloadRecent = coalesceReload(() => loadRecent());
	const reloadHealth = coalesceReload(() => loadHealth());
	let refreshing = false;

	onMount(() => {
		live = readDashboardLive(browserStorage());
		void loadEverything(true);
		// Doctor reads once per visit: its integrity check is a whole-database
		// scan, and the checks it reports do not change between refreshes.
		void loadDoctor();
		refreshTimer = setInterval(() => {
			// A refresh still working answers what the next tick would ask,
			// so the tick that finds one running leaves the work to it.
			if (document.visibilityState !== 'visible' || refreshing) return;
			refreshing = true;
			void loadEverything().finally(() => (refreshing = false));
		}, 30_000);
		unsubLogs = subscribe('logs', (payload: unknown) => {
			const event = payload as { provider?: string } | null;
			if (event?.provider) pulseProvider(event.provider);
			if (live) reloadRecent.schedule();
		});
		unsubQuota = subscribe('quota', () => reloadHealth.schedule());
	});
	onDestroy(() => {
		clearInterval(refreshTimer);
		unsubLogs?.();
		unsubQuota?.();
		reloadRecent.cancel();
		reloadHealth.cancel();
		pulseTimers.forEach((timer) => clearTimeout(timer));
	});

	// The KPI tiles, the spend board and both trend reads follow the page window,
	// while the routing map, doctor and request tail keep their own scopes.
	function onSummaryPeriod(period: string) {
		if (isWindowPeriod(period)) summaryPeriod = period;
		void Promise.all([loadSummary(), loadSpend(), loadTrend(), loadTrendPaid()]);
	}

	// The expanded key row belongs to the set that was open, so it closes with
	// the swap.
	function onSpendSet(next: string) {
		if (isSpendSet(next)) spendSet = next;
		expandedSpend = '';
		void loadSpend();
	}

	async function onExpandSpend(providerId: string) {
		if (expandedSpend === providerId) {
			expandedSpend = '';
			return;
		}
		expandedSpend = providerId;
		spendKeys = [];
		spendKeysLoading = true;
		try {
			const start = windowStart(summaryPeriod, Date.now());
			spendKeys =
				(
					await api<{ items: UsageRollupRow[] }>(
						'/activity/usage?group_by=credential&since_ms=' +
							start +
							'&provider=' +
							encodeURIComponent(providerId)
					)
				).items ?? [];
		} catch {
			spendKeys = [];
		} finally {
			spendKeysLoading = false;
		}
	}
</script>

<svelte:head><title>{$t('ui.pages.dashboard.title')} · Relo</title></svelte:head>

<PageHeader title="ui.pages.dashboard.title" description="ui.pages.dashboard.description">
	<!-- The window control is a header action: right-aligned beside the
	     title and subtitle, wrapping to its own right-aligned row when the
	     header line lacks room. -->
	{#snippet actions()}
		<div class="min-w-0 max-w-full overflow-x-auto">
			<Segmented
				value={summaryPeriod}
				options={[
					{ value: '24h', label: $t('ui.common.window24h') },
					{ value: 'today', label: $t('ui.common.windowToday') },
					{ value: '7d', label: $t('ui.common.window7d') },
					{ value: '30d', label: $t('ui.common.window30d') },
					{ value: '60d', label: $t('ui.common.window60d') },
					{ value: '90d', label: $t('ui.common.window90d') },
					{ value: 'all', label: $t('ui.common.windowAll') }
				]}
				ariaLabel={$t('ui.pages.usagePage.rangeLabel')}
				onchange={onSummaryPeriod}
			/>
		</div>
	{/snippet}
</PageHeader>

{#snippet processing()}
	<div class="flex min-h-16 items-center justify-center py-4" role="status">
		<Icon name="loader" size={18} spin class="text-accent-strong" />
		<span class="sr-only">{$t('ui.common.loading')}</span>
	</div>
{/snippet}

<div class="page-frame page-stack pt-6">
	{#if activityFailed}
		<div
			class="flex items-center justify-between gap-3 rounded-card border border-danger/30 bg-danger/5 px-4 py-3 text-sm"
			role="alert"
		>
			<span class="flex min-w-0 items-center gap-2 text-danger">
				<Icon name="circle-alert" size={15} />
				<span class="min-w-0">{activityFailed}</span>
			</span>
			<Button variant="outline" size="sm" onclick={() => void loadEverything()}>
				<Icon name="refresh" size={14} />
				{$t('ui.common.retry')}
			</Button>
		</div>
	{/if}

	<div class="grid grid-cols-2 gap-[var(--grid-gap)] md:grid-cols-3 xl:grid-cols-6">
		<KpiTile
			icon="list"
			labelKey="ui.pages.dashboard.requests24h"
			loading={contentLoading}
			pending={processing}
		>
			<div class="mt-auto flex flex-wrap items-baseline gap-x-3 gap-y-1">
				<span class="font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink">
					<AnimatedNumber
						value={requestsWindow}
						format={(value) => Math.round(value).toLocaleString()}
					/>
				</span>
				{#if requestsDelta !== null}
					<span class="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground">
						<Icon name={requestsDelta >= 0 ? 'arrow-up' : 'arrow-down'} size={13} />
						{(requestsDelta >= 0 ? '+' : '') + requestsDelta}%
					</span>
				{/if}
			</div>
			<p class="min-h-4 text-xs text-muted-foreground">
				{#if errorsWindow > 0}{errorsWindow} {$t('ui.pages.dashboard.errors')}
				{:else if successRate !== null}{$t('ui.pages.dashboard.percentOk', {
						values: { value: successRate.toFixed(0) }
					})}
				{/if}
			</p>
		</KpiTile>

		<KpiTile
			icon="circle-check"
			labelKey="ui.pages.dashboard.successTile"
			loading={contentLoading}
			pending={processing}
		>
			<span
				class="mt-auto font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink"
			>
				<AnimatedNumber
					value={successRate ?? 0}
					format={(value) => (successRate === null ? '—' : value.toFixed(0) + '%')}
				/>
			</span>
			<p class="min-h-4 text-xs">
				{#if errorsWindow > 0}
					<span class="inline-flex items-center gap-1 text-danger">
						<Icon name="circle-alert" size={12} />{errorsWindow}
						{$t('ui.pages.dashboard.errors')}
					</span>
				{/if}
			</p>
		</KpiTile>

		<KpiTile
			icon="currency-dollar"
			labelKey="ui.pages.dashboard.paidSpend"
			loading={contentLoading}
			pending={processing}
		>
			<span
				class="mt-auto font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink"
			>
				<AnimatedNumber value={paidSpendWindow} format={(value) => formatCost(Math.round(value))} />
			</span>
			<p class="min-h-4 text-xs text-muted-foreground">
				{$t('ui.pages.dashboard.paidSpendHint', { values: { count: paid.length } })}
			</p>
		</KpiTile>

		<KpiTile
			icon="sparkles"
			labelKey="ui.pages.dashboard.tokens"
			loading={contentLoading}
			pending={processing}
		>
			<span
				class="mt-auto font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink"
				title={formatTokensExact(tokensWindow)}
				><AnimatedNumber
					value={tokensWindow}
					format={(value) => formatTokens(Math.round(value))}
				/></span
			>
			<p class="min-h-4 text-xs font-medium text-muted-foreground">
				{#if tokensDelta !== null}
					<span class="inline-flex items-center gap-1">
						<Icon name={tokensDelta >= 0 ? 'arrow-up' : 'arrow-down'} size={13} />
						{(tokensDelta >= 0 ? '+' : '') + tokensDelta}%
					</span>
				{/if}
			</p>
		</KpiTile>

		<KpiTile
			icon="clock"
			labelKey="ui.pages.dashboard.latency"
			loading={contentLoading}
			pending={processing}
		>
			<span
				class="mt-auto font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink"
			>
				<AnimatedNumber
					value={avgDuration}
					format={(value) => (value > 0 ? (value / 1000).toFixed(1) + 's' : '—')}
				/>
			</span>
			<p class="min-h-4 text-xs text-muted-foreground">
				{status ? formatUptime(status.uptime_seconds) : ''}
			</p>
		</KpiTile>

		<KpiTile
			icon="user"
			labelKey="ui.pages.dashboard.accountsKpi"
			loading={contentLoading}
			pending={processing}
		>
			<span
				class="mt-auto font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink"
			>
				<AnimatedNumber value={healthReady} format={(value) => String(Math.round(value))} /><span
					class="text-sm font-normal text-muted-foreground"
				>
					/ {accounts.length}</span
				>
			</span>
			<p class="min-h-4 text-xs">
				{#if healthLimited + healthNeedsSignin > 0}
					<span class="inline-flex items-center gap-1 text-warn">
						<Icon name="alert-triangle" size={12} />
						{healthLimited}
						{$t('ui.pages.dashboard.stateLimited')} · {healthNeedsSignin}
						{$t('ui.pages.dashboard.stateNeedsSignin')}
					</span>
				{/if}
			</p>
		</KpiTile>
	</div>

	<div class="grid min-w-0 grid-cols-1 gap-[var(--grid-gap)] lg:grid-cols-2">
		<HealthBoard
			cards={healthBoard}
			loading={contentLoading || (accountsLoading && accounts.length === 0)}
			failed={healthFailed}
			windowsOf={accountWindows}
			onretry={() => void loadHealth()}
			onadd={() => goto('/connections?add=1')}
			pending={processing}
		/>

		<SpendBoard
			rows={paidSpendBoard}
			loading={contentLoading || spendLoading}
			swapped={boardSwapped}
			set={spendSet}
			opened={openedSpend}
			onsetset={onSpendSet}
			onexpand={(providerId) => void onExpandSpend(providerId)}
			pending={processing}
		/>
	</div>

	<div class="grid min-w-0 grid-cols-1 gap-[var(--grid-gap)] lg:grid-cols-2">
		<TrendChart
			days={trendDays}
			metric={trendMode}
			options={trendOptions}
			loading={contentLoading}
			pending={processing}
			onmetric={(next) => (trendMode = next)}
		/>

		<TokenMixDonut rows={dailyAll} loading={contentLoading} pending={processing} />
	</div>

	<div class="grid min-w-0 grid-cols-1 gap-[var(--grid-gap)] lg:grid-cols-2">
		<div class="flex min-w-0 flex-col gap-[var(--grid-gap)]">
			<RoutingUniverse
				providers={configuredProviders}
				traffic={traffic24}
				pulsing={pulsingProviders}
				loading={contentLoading}
				pending={processing}
				onadd={() => goto('/connections?add=1')}
			/>

			<DoctorChecks
				checks={doctor}
				loading={doctorLoading}
				failed={doctorFailed}
				pending={processing}
				onretry={() => void loadDoctor()}
			/>
		</div>

		<RecentRequests
			rows={recent}
			loading={contentLoading}
			{live}
			pending={processing}
			ontoggle={setLive}
			onviewall={() => goto('/logs')}
		/>
	</div>
</div>
