<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import { coalesceReload } from '$lib/refresh';
	import { subscribe } from '$lib/sse';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import RoutingUniverse from '$lib/components/dashboard/routing-universe.svelte';
	import AttentionBand from '$lib/components/dashboard/attention-band.svelte';
	import DoctorChecks from '$lib/components/dashboard/doctor-checks.svelte';
	import RecentRequests from '$lib/components/dashboard/recent-requests.svelte';
	import HealthBoard from '$lib/components/dashboard/health-board.svelte';
	import SpendChart from '$lib/components/dashboard/spend-chart.svelte';
	import TrendChart from '$lib/components/dashboard/trend-chart.svelte';
	import TokenMixDonut from '$lib/components/dashboard/token-mix-donut.svelte';
	import KpiTile from '$lib/components/dashboard/kpi-tile.svelte';
	import AnimatedNumber from '$lib/components/ui/animated-number.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import { formatCost, formatTokens, formatTokensExact, formatUptime } from '$lib/format';
	import {
		accountAttention,
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
	import type { TunnelTraffic } from '$lib/route-tunnel';
	import { bucketDays, type ChartMetric } from '$lib/usage-charts';
	import type {
		AccessKey,
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
	// clientRows holds the same window's traffic by the key that sent it, which is
	// what the routing tunnel names its agents from.
	let clientRows = $state.raw<UsageRollupRow[]>([]);
	// keys names the coding client each key belongs to, so the tunnel can name the
	// agent behind a key's traffic.
	let keys = $state.raw<AccessKey[]>([]);
	let dailyAll = $state.raw<UsageRollupRow[]>([]);
	let recent = $state.raw<UsageLogRow[]>([]);
	let activityFailed = $state('');
	// live tails the request log the same way the logs page does. The choice
	// lives in this browser so a reload keeps the last on or off.
	let live = $state(true);
	// The recent-request list shows the newest rows of the request log, filling the
	// column beside the three cards.
	const RECENT_LIMIT = 19;
	const recentList = $derived(recent.slice(0, RECENT_LIMIT));

	// One window drives the whole windowed page: the KPI tiles, the paid-spend
	// board, the trends and the routing tunnel. The doctor and the request tail
	// keep their own scopes below. The rolling 24 hours is the default the
	// desktop window mirrors.
	let summaryPeriod = $state<WindowPeriod>('24h');

	// The routing tunnel reads the same window as the tiles above it: the keys that
	// spoke, the connections that served them, and the newest request inside it.
	const tunnelTraffic = $derived<TunnelTraffic>({
		clients: clientRows,
		connections: summaryRows,
		newest: recent[0] ?? null,
		since: windowStart(summaryPeriod, Date.now())
	});

	let trendMode = $state<ChartMetric>('requests');
	let dailyPaid = $state<UsageRollupRow[]>([]);

	const configuredProviders = $derived(providers.filter((provider) => provider.configured));
	const paid = $derived(paidProviders(providers).filter((provider) => provider.configured));
	const plan = $derived(planProviders(providers).filter((provider) => provider.configured));
	// The spend charts read the window's own rollup: what the paid connections
	// cost, and what the plan-covered ones were worth.
	const apiSpendRows = $derived(spendRows(paid, summaryRows));
	const planUsageRows = $derived(spendRows(plan, summaryRows));

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
	const tokensInWindow = $derived(total(summaryRows, (row) => Number(row.InputTokens)));
	const tokensOutWindow = $derived(total(summaryRows, (row) => Number(row.OutputTokens)));
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
	// The accounts that need an operator, read before the figures: a sign-in that
	// expired, a window that is full, a window that is nearly full.
	const attentionItems = $derived(accountAttention(accounts, quotaWindows));

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
	// The trend's spend series reads the paid connections at day grain, so it
	// asks for the same days the window covers. The connections are named only
	// when they fit the daemon's filter cap; a wider roster reads every
	// connection and the join keeps the ones asked for.
	function paidDaysUrl(chosen: readonly Provider[], start: number): string {
		let url = '/activity/usage?group_by=day&since_ms=' + start;
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

	async function loadTrendPaid() {
		try {
			// The trend's spend series stays API spend, so it reads the paid
			// connections.
			dailyPaid =
				(
					await api<{ items: UsageRollupRow[] }>(
						paidDaysUrl(paid, windowStart(summaryPeriod, Date.now()))
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

	async function loadKeys() {
		try {
			keys = (await api<{ items: AccessKey[] }>('/clients/keys')).items ?? [];
		} catch {
			keys = [];
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
			const clientRead = api<{ items: UsageRollupRow[] }>(
				'/activity/usage?group_by=client&since_ms=' + window.start
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
			const [current, clients, previous] = await Promise.all([
				currentRead,
				clientRead,
				previousRead
			]);
			summaryRows = current.items ?? [];
			clientRows = clients.items ?? [];
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

	async function loadEverything(includeRecent = live) {
		try {
			await Promise.all([
				loadStatus(),
				loadHealth(),
				loadKeys(),
				loadSummary(),
				loadTrend(),
				loadTrendPaid(),
				includeRecent ? loadRecent() : Promise.resolve()
			]);
		} finally {
			contentLoading = false;
		}
	}

	function setLive(next: boolean) {
		live = next;
		writeDashboardLive(browserStorage(), next);
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
		unsubLogs = subscribe('logs', () => {
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
	});

	// The KPI tiles, the spend charts and both trend reads follow the page
	// window, while the doctor and the request tail keep their own scopes.
	function onSummaryPeriod(period: string) {
		if (isWindowPeriod(period)) summaryPeriod = period;
		void Promise.all([loadSummary(), loadTrend(), loadTrendPaid()]);
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

	<!-- The accounts that need an operator, read before the figures below. -->
	<AttentionBand
		items={attentionItems}
		onopen={(providerId) =>
			goto('/connections?provider=' + encodeURIComponent(providerId) + '&tab=accounts')}
	/>

	<!-- The six figures of the window lead the page. -->
	<div class="grid min-w-0 grid-cols-2 gap-[var(--grid-gap)] md:grid-cols-3 xl:grid-cols-6">
		<KpiTile
			icon="list"
			labelKey="ui.pages.dashboard.requests24h"
			loading={contentLoading}
			pending={processing}
			delta={requestsDelta}
		>
			<span class="font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink">
				<AnimatedNumber
					value={requestsWindow}
					format={(value) => Math.round(value).toLocaleString()}
				/>
			</span>
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
			<span class="font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink">
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
			<span class="font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink">
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
			delta={tokensDelta}
		>
			<span
				class="font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink"
				title={formatTokensExact(tokensWindow)}
				><AnimatedNumber
					value={tokensWindow}
					format={(value) => formatTokens(Math.round(value))}
				/></span
			>
			<p class="min-h-4 text-xs text-muted-foreground">
				{$t('ui.pages.usagePage.noteTokenMix', {
					values: {
						input: formatTokens(tokensInWindow),
						output: formatTokens(tokensOutWindow)
					}
				})}
			</p>
		</KpiTile>

		<KpiTile
			icon="clock"
			labelKey="ui.pages.dashboard.latency"
			loading={contentLoading}
			pending={processing}
		>
			<span class="font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink">
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
			<span class="font-mono text-[1.625rem] leading-none font-medium tabular-nums text-ink">
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

	<!-- The map takes the wider half; the trend and the token mix stack beside it. -->
	<div
		class="grid min-w-0 grid-cols-1 gap-[var(--grid-gap)] xl:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]"
	>
		<RoutingUniverse
			traffic={tunnelTraffic}
			{keys}
			providers={configuredProviders}
			{live}
			loading={contentLoading}
			pending={processing}
			onadd={() => goto('/connections?add=1')}
			class="min-w-0"
		/>

		<div class="flex min-w-0 flex-col gap-[var(--grid-gap)]">
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
	</div>

	<!-- The accounts the operator has to look after. -->
	<HealthBoard
		cards={healthBoard}
		loading={contentLoading || (accountsLoading && accounts.length === 0)}
		failed={healthFailed}
		windowsOf={accountWindows}
		onretry={() => void loadHealth()}
		onadd={() => goto('/connections?add=1')}
		pending={processing}
	/>

	<!-- What the window cost, and what the plans covered. -->
	<div class="grid min-w-0 grid-cols-1 gap-[var(--grid-gap)] lg:grid-cols-2">
		<SpendChart
			rows={apiSpendRows}
			titleKey="ui.pages.dashboard.spendApi"
			descriptionKey="ui.pages.dashboard.spendApiDesc"
			emptyKey="ui.pages.dashboard.spendEmpty"
			loading={contentLoading}
			pending={processing}
		/>

		<SpendChart
			rows={planUsageRows}
			titleKey="ui.pages.dashboard.spendPlan"
			descriptionKey="ui.pages.dashboard.spendPlanDesc"
			emptyKey="ui.pages.dashboard.spendPlanEmpty"
			loading={contentLoading}
			pending={processing}
		/>
	</div>

	<!-- Recent requests and the daemon's own checks. -->
	<div
		class="grid min-w-0 grid-cols-1 gap-[var(--grid-gap)] lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]"
	>
		<RecentRequests
			rows={recentList}
			loading={contentLoading}
			{live}
			pending={processing}
			ontoggle={setLive}
			onviewall={() => goto('/logs')}
		/>

		<DoctorChecks
			checks={doctor}
			loading={doctorLoading}
			failed={doctorFailed}
			pending={processing}
			onretry={() => void loadDoctor()}
		/>
	</div>
</div>
