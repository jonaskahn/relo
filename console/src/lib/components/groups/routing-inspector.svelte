<script lang="ts">
	import { t } from 'svelte-i18n';

	import { api, ApiError } from '$lib/api';
	import { formatDateTime } from '$lib/format';
	import { coalesceReload } from '$lib/refresh';
	import {
		attemptChain,
		orderMembers,
		overflowMembers,
		recentRouteChains,
		selectCurrentMember,
		selectNextMember,
		type CurrentMember,
		type MemberView
	} from '$lib/route-inspector';
	import { subscribe } from '$lib/sse';
	import type {
		Account,
		Group,
		GroupMember,
		Model,
		Provider,
		QuotaWindow,
		RoutePreview,
		UsageAttemptRow,
		UsageLogRow
	} from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import RoutingFlow from './routing-flow.svelte';

	interface Props {
		open?: boolean;
		group: Group | null;
		providers: Provider[];
		meta: Record<string, Model>;
	}

	let { open = $bindable(false), group, providers, meta }: Props = $props();

	let routeGroup = $state<Group | null>(null);
	let preview = $state<RoutePreview | null>(null);
	let accounts = $state<Account[]>([]);
	let quota = $state<QuotaWindow[]>([]);
	let recent = $state<UsageLogRow[]>([]);
	let attempts = $state<ReadonlyMap<number, UsageAttemptRow[]>>(new Map());
	let loading = $state(false);
	let failure = $state('');
	let current = $state<CurrentMember>({ index: -1, accountLabel: '' });
	let quotaFailed = $state(false);

	const candidates = $derived(preview?.candidates ?? []);
	const members = $derived(routeGroup?.members ?? []);
	const ordered = $derived(orderMembers(members, candidates));
	const next = $derived(selectNextMember(ordered, candidates));
	const recentChains = $derived(recentRouteChains(recent, attempts));
	const latest = $derived(recent[0] ?? null);
	const overflow = $derived(overflowMembers(memberViews(ordered)));

	function memberViews(list: readonly GroupMember[]): MemberView[] {
		return list.map((member) => {
			const connection = providers.find((provider) => provider.id === member.provider_id);
			const model = meta[member.provider_id + '/' + member.model_id];
			const held = accounts.filter((account) => account.provider_id === member.provider_id);
			const credentialIDs = new Set(held.map((account) => account.id));
			return {
				member,
				providerLabel: connection?.label || member.provider_id,
				modelName: model?.name || member.model_id,
				accounts: held,
				windows: quota.filter(
					(window) =>
						window.connection_id === member.provider_id || credentialIDs.has(window.credential_id)
				)
			};
		});
	}

	$effect(() => {
		if (!open) return;
		routeGroup = group;
		void load();
		const reload = coalesceReload(() => load(false));
		const stopLogs = subscribe('logs', () => reload.schedule());
		const stopQuota = subscribe('quota', () => reload.schedule());
		return () => {
			stopLogs();
			stopQuota();
			reload.cancel();
		};
	});

	async function load(showLoading = true) {
		if (!group) return;
		if (showLoading) loading = true;
		failure = '';
		try {
			routeGroup = await api<Group>('/routes/' + encodeURIComponent(group.id));
			const [previewResult, requestsResult] = await Promise.allSettled([
				api<RoutePreview>('/routes/preview?model=' + encodeURIComponent(group.id)),
				api<{ items: UsageLogRow[] }>(
					'/activity/requests?group=' + encodeURIComponent(group.id) + '&limit=5'
				)
			]);
			if (previewResult.status === 'fulfilled') {
				preview = previewResult.value;
			} else if (!(
				previewResult.reason instanceof ApiError && previewResult.reason.status === 404
			)) {
				throw previewResult.reason;
			} else {
				preview = { model: group.id, kind: 'group', group_id: group.id, candidates: [] };
			}
			if (requestsResult.status === 'fulfilled') {
				recent = requestsResult.value.items ?? [];
				await readAttempts();
				const first = attempts.get(recent[0]?.ID ?? -1) ?? [];
				current = selectCurrentMember(ordered, first);
			} else {
				throw requestsResult.reason;
			}
			await Promise.allSettled([readAccounts(), readQuota()]);
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			if (showLoading) loading = false;
		}
	}

	async function readAccounts() {
		try {
			const data = await api<{ items: Account[] }>('/accounts');
			accounts = data.items ?? [];
		} catch {
			accounts = [];
		}
	}

	async function readQuota() {
		quotaFailed = false;
		try {
			const data = await api<{ items: QuotaWindow[] }>('/activity/quota');
			quota = data.items ?? [];
		} catch {
			quota = [];
			quotaFailed = true;
		}
	}

	async function readAttempts() {
		const results = await Promise.allSettled(
			recent.map((request) =>
				api<{ items: UsageAttemptRow[] }>('/activity/requests/' + request.ID + '/attempts')
			)
		);
		const nextMap = new Map<number, UsageAttemptRow[]>();
		results.forEach((result, index) => {
			if (result.status === 'fulfilled') nextMap.set(recent[index].ID, result.value.items ?? []);
		});
		attempts = nextMap;
	}

	function chosenFor(request: UsageLogRow): CurrentMember {
		return selectCurrentMember(ordered, attempts.get(request.ID) ?? []);
	}

	function chosenLabel(request: UsageLogRow): string {
		const chosen = chosenFor(request);
		return chosen.index >= 0
			? ordered[chosen.index].provider_id + '/' + ordered[chosen.index].model_id
			: '—';
	}

	function openLatestLog() {
		if (!latest) return;
		window.location.assign('/logs?section=agent&request=' + latest.ID);
	}
</script>

<CenteredModal
	bind:open
	size="xl"
	title={routeGroup?.label || routeGroup?.id || group?.label || group?.id || ''}
	description={routeGroup
		? routeGroup.id + ' · ' + $t('ui.pages.groupsPage.strategyName.' + routeGroup.strategy)
		: undefined}
>
	{#snippet titleAside()}
		{#if routeGroup}
			<StatusBadge
				kind={routeGroup.enabled ? 'pass' : 'muted'}
				label={$t(
					routeGroup.enabled ? 'ui.pages.groupsPage.enabled' : 'ui.pages.groupsPage.chipDisabled'
				)}
			/>
			{#if !routeGroup.listed}
				<StatusBadge kind="muted" label={$t('ui.pages.groupsPage.chipUnlisted')} />
			{/if}
		{/if}
	{/snippet}

	{#if loading}
		<div class="space-y-3" role="status" aria-label={$t('ui.pages.groupsPage.inspector.loading')}>
			<div class="h-24 animate-pulse rounded-card bg-sunken"></div>
			<div class="h-40 animate-pulse rounded-card bg-sunken"></div>
		</div>
	{:else if failure}
		<div
			class="flex items-center gap-3 rounded-card border border-destructive/30 bg-destructive/10 p-4"
			role="alert"
		>
			<p class="flex-1 text-sm text-destructive">{failure}</p>
			<Button variant="outline" onclick={() => void load()}>
				<Icon name="refresh" size={14} />
				{$t('ui.common.retry')}
			</Button>
		</div>
	{:else}
		<div class="flex flex-col gap-5">
			{#if ordered.length === 0}
				<p class="empty-panel border-dashed text-sm text-muted-foreground">
					{$t('ui.pages.groupsPage.inspector.empty')}
				</p>
			{:else}
				<RoutingFlow
					groupLabel={routeGroup?.label || routeGroup?.id || ''}
					requestLabel={routeGroup?.client_id || routeGroup?.id || ''}
					members={overflow.visible}
					current={current.index}
					{next}
					hidden={overflow.hidden}
					chains={recentChains}
				/>
			{/if}

			{#if quotaFailed}
				<p class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.inspector.noQuota')}</p>
			{/if}

			<div>
				<h3 class="field-label">{$t('ui.pages.groupsPage.inspector.recent')}</h3>
				{#if recent.length === 0}
					<p class="mt-2 text-sm text-muted-foreground">
						{$t('ui.pages.groupsPage.inspector.noRecent')}
					</p>
				{:else}
					<ul class="mt-2 overflow-hidden rounded-card bg-surface shadow-float-1">
						{#each recent as request (request.ID)}
							<li
								class="flex flex-wrap items-center gap-x-3 gap-y-1 border-b border-border px-4 py-2.5 text-xs last:border-b-0"
							>
								<span class="mono-data text-muted-foreground"
									>{formatDateTime(request.TimestampMs)}</span
								>
								<a
									class="mono-data text-accent-deep hover:underline"
									href={`/logs?section=agent&request=${request.ID}`}
									>{request.RequestID || request.ID}</a
								>
								<span class="min-w-0 flex-1 truncate">{chosenLabel(request)}</span>
								<span class="mono-data text-muted-foreground"
									>{attemptChain(attempts.get(request.ID) ?? []) || '—'}</span
								>
								<span class="mono-data">{request.Status}</span>
								<span class="mono-data text-muted-foreground">{request.DurationMs}ms</span>
							</li>
						{/each}
					</ul>
				{/if}
			</div>
		</div>
	{/if}

	{#snippet footer()}
		<div class="modal-footer items-center">
			<div class="flex-1"></div>
			<Button variant="outline" onclick={() => void load()} disabled={loading}>
				<Icon name="refresh" size={14} />
				{$t('ui.pages.groupsPage.inspector.refresh')}
			</Button>
			<Button variant="outline" onclick={openLatestLog} disabled={latest === null}>
				<Icon name="external-link" size={14} />
				{$t('ui.pages.groupsPage.inspector.openLogs')}
			</Button>
			<Button onclick={() => (open = false)}>{$t('ui.common.close')}</Button>
		</div>
	{/snippet}
</CenteredModal>
