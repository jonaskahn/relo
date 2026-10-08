<script lang="ts">
	import { onDestroy, onMount, untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { ApiError, api } from '$lib/api';
	import { consoleState } from '$lib/console-state.svelte';
	import { accountNeedsSignIn } from '$lib/provider-status';
	import { coalesceReload } from '$lib/refresh';
	import { subscribe } from '$lib/sse';
	import { cardLayoutCard, cardLayoutGrid } from '$lib/theme.svelte';
	import type { Account, LoginMethod, Provider, QuotaWindow } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import CardContent from '$lib/components/ui/card-content.svelte';
	import CardDescription from '$lib/components/ui/card-description.svelte';
	import CardHeader from '$lib/components/ui/card-header.svelte';
	import CardTitle from '$lib/components/ui/card-title.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import AccountContextControl from './account-context-control.svelte';
	import QuotaMeterRows from './quota-meter-rows.svelte';
	import SignInPanel from './sign-in-panel.svelte';

	interface Props {
		provider: Provider;
		accounts: Account[];
		loading?: boolean;
		failed?: string;
		// windows is a reading the caller already holds. The tab shows it until
		// the next probe or live response replaces it.
		windows?: QuotaWindow[];
		quotaRevision?: number;
		// The connection pane renders the Add account trigger in its own
		// toolbar next to Back, so it owns whether the panel is open.
		adding?: boolean;
		onaddingchange?: (next: boolean) => void;
		onreload: () => void;
		onremoved: () => void;
		onaddkey: () => void;
		onquotas?: () => void;
	}

	let {
		provider,
		accounts,
		loading = false,
		failed = '',
		windows = [],
		quotaRevision = 0,
		adding = false,
		onaddingchange,
		onreload,
		onremoved,
		onaddkey,
		onquotas
	}: Props = $props();

	let working = $state(false);
	let notice = $state('');
	let removing = $state<Account | null>(null);
	let removingBusy = $state(false);
	let removeFailed = $state('');
	let removeOpen = $state(false);
	let quotas = $state<QuotaWindow[]>(untrack(() => windows));
	// signingIn is the account whose sign-in panel is open. One card signs in
	// at a time, and cancel returns that card to the button.
	let signingIn = $state('');
	let unsubscribe: (() => void) | null = null;
	// Every live response can announce a fresh reading, so the windows reload
	// once per burst rather than once per announcement.
	const reloadQuotas = coalesceReload(() => loadQuotas());

	const signIn = $derived(provider.auth === 'oauth');
	const needsCredential = $derived(provider.auth !== 'none');

	// The daemon names the flows a provider signs in with; a provider stored
	// without methods falls back to the flows it was saved with.
	const providerMethods = $derived<LoginMethod[]>(
		provider.login_methods && provider.login_methods.length > 0
			? provider.login_methods
			: (provider.login_flows ?? []).map((flow) => ({ flow, kind: 'browser' }))
	);

	const kindKeys: Record<string, string> = {
		oauth: 'ui.pages.providersPage.accounts.kindSignin',
		api_key: 'ui.pages.providersPage.accounts.kindKey',
		aws_keys: 'ui.pages.providersPage.accounts.kindAWS',
		gcp_service_account: 'ui.pages.providersPage.accounts.kindGCP'
	};

	// The quota of one account is whatever the daemon has stored for its
	// credential, which a probe or a live response wrote.
	const windowsOf = $derived.by(() => {
		const byCredential = new Map<string, QuotaWindow[]>();
		for (const window of quotas) {
			const held = byCredential.get(window.credential_id) ?? [];
			byCredential.set(window.credential_id, [...held, window]);
		}
		return byCredential;
	});

	// Reading this account's own list again is what picks up an entitlement the
	// provider changed without updating every connection.
	async function readModels(account: Account) {
		working = true;
		notice = '';
		try {
			const models = await api<{ models: string[] }>(
				'/accounts/' + encodeURIComponent(account.id) + '/models/refresh',
				{ method: 'POST' }
			);
			notice = $t('ui.pages.providersPage.accounts.modelsRead', {
				values: { count: models.models?.length ?? 0, label: account.label }
			});
			onreload();
		} catch (error) {
			notice = $t('ui.pages.providersPage.accounts.modelsReadFailed', {
				values: { detail: error instanceof Error ? error.message : String(error) }
			});
		} finally {
			working = false;
		}
	}

	onMount(() => {
		unsubscribe = subscribe('quota', () => reloadQuotas.schedule());
	});

	onDestroy(() => {
		unsubscribe?.();
		reloadQuotas.cancel();
	});

	// The quota belongs to the connection on screen, so a switch to another
	// connection reads again: the windows that were open are not an answer
	// about this one's accounts.
	$effect(() => {
		void provider.id;
		void quotaRevision;
		void loadQuotas();
	});

	async function loadQuotas() {
		try {
			const data = await api<{ items: QuotaWindow[] }>('/activity/quota');
			quotas = (data.items ?? []).filter((window) => window.connection_id === provider.id);
		} catch {
			// The quota is a report about an account, so a read that fails
			// leaves the account itself untouched.
		}
	}

	// A new account usually reaches more models than the connection served
	// before, so the roster is read again with it and the pane then reports the
	// connection as it now stands.
	async function refreshModels() {
		try {
			await api('/connections/' + encodeURIComponent(provider.id) + '/models/refresh', {
				method: 'POST'
			});
		} catch {
			// The account is stored either way, so a listing that failed is
			// reported by the connection's own status rather than here.
		}
		onreload();
		await loadQuotas();
		onquotas?.();
	}

	const statusKeys: Record<string, string> = {
		active: 'ui.pages.providersPage.accounts.statusActive',
		paused: 'ui.pages.providersPage.accounts.statusPaused',
		needs_reauth: 'ui.pages.providersPage.accounts.statusReauth'
	};

	// A sign-in whose token was refused, or whose connection could not refresh,
	// is signed in again from the quota slot. The header badge stays the
	// account's own status.
	function needsSignIn(account: Account): boolean {
		return accountNeedsSignIn(account, provider.last_refresh_error);
	}

	async function setStatus(account: Account, status: string) {
		working = true;
		notice = '';
		try {
			await api('/accounts/' + encodeURIComponent(account.id), {
				method: 'PATCH',
				body: JSON.stringify({ status })
			});
			onreload();
		} catch (error) {
			notice = $t('ui.pages.providersPage.accounts.failed', {
				values: { detail: error instanceof Error ? error.message : String(error) }
			});
		} finally {
			working = false;
		}
	}

	async function remove() {
		const account = removing;
		removeFailed = '';
		if (!account) return;
		removingBusy = true;
		try {
			await api('/accounts/' + encodeURIComponent(account.id), { method: 'DELETE' });
			removing = null;
			removeOpen = false;
			if (accounts.length === 1 && needsCredential) {
				try {
					await api<Provider>('/connections/' + encodeURIComponent(provider.id));
					onreload();
				} catch (error) {
					if (error instanceof ApiError && error.status === 404) onremoved();
					else onreload();
				}
			} else onreload();
		} catch (error) {
			removeFailed = error instanceof Error ? error.message : String(error);
		} finally {
			removingBusy = false;
		}
	}
</script>

<div class="flex flex-col gap-4 py-5">
	<div class="flex flex-wrap items-center justify-between gap-2">
		<span class="text-xs text-muted-foreground">
			{$t('ui.pages.providersPage.list.accountCount', { values: { count: accounts.length } })}
		</span>
	</div>

	{#if adding && signIn}
		<div class="flex flex-col gap-2">
			<p class="text-xs text-muted-foreground">
				{$t('ui.pages.providersPage.add.signInTargetHint', { values: { label: provider.label } })}
			</p>
			<SignInPanel
				methods={providerMethods}
				ondone={() => {
					onaddingchange?.(false);
					void refreshModels();
				}}
				oncancel={() => onaddingchange?.(false)}
			/>
		</div>
	{:else if adding}
		<div class="flex flex-wrap items-center gap-2 rounded-lg border border-border p-3">
			<p class="min-w-0 flex-1 text-xs text-muted-foreground">
				{$t('ui.pages.providersPage.add.targetHint', { values: { label: provider.label } })}
			</p>
			<Button
				size="sm"
				onclick={() => {
					onaddingchange?.(false);
					onaddkey();
				}}
			>
				<Icon name="key" size={14} />
				{$t('ui.pages.providersPage.add.targetTitle', { values: { label: provider.label } })}
			</Button>
			<Button variant="ghost" size="sm" onclick={() => onaddingchange?.(false)}>
				<Icon name="x" size={14} />
				{$t('ui.common.cancel')}
			</Button>
		</div>
	{/if}

	{#if !needsCredential}
		<p class="text-sm text-muted-foreground">
			{$t('ui.pages.providersPage.accounts.noCredential')}
		</p>
	{:else if loading}
		<p class="text-sm text-muted-foreground">{$t('ui.common.loading')}</p>
	{:else if failed}
		<p class="text-sm text-destructive">
			{$t('ui.pages.providersPage.accounts.loadFailed', { values: { detail: failed } })}
		</p>
	{:else if accounts.length === 0}
		<p class="text-sm text-muted-foreground">{$t('ui.pages.providersPage.accounts.empty')}</p>
	{:else}
		{#snippet kindBadge(account: Account)}
			{#if account.kind === 'oauth'}
				<span
					class="inline-flex items-center rounded border border-border px-1.5 py-1 text-muted-foreground"
					title={$t('ui.pages.providersPage.accounts.kindSigninLabel')}
				>
					<Icon name="lock" size={13} />
					<span class="sr-only">{$t('ui.pages.providersPage.accounts.kindSigninLabel')}</span>
				</span>
			{:else}
				<span
					class="rounded border border-border px-1.5 py-0.5 text-[0.65rem] text-muted-foreground"
				>
					{$t(kindKeys[account.kind] ?? 'ui.pages.providersPage.accounts.kindKey')}
				</span>
			{/if}
		{/snippet}

		{#snippet statusBadge(account: Account)}
			<StatusBadge
				kind={account.status === 'active' ? 'pass' : account.status === 'paused' ? 'warn' : 'fail'}
				label={statusKeys[account.status] ? $t(statusKeys[account.status]) : account.status}
			/>
		{/snippet}

		<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
			{#each accounts as account (account.id)}
				<Card class="section-surface {cardLayoutCard(consoleState.settings.cardLayout)} gap-3 py-3">
					<CardHeader class="shrink-0 gap-1.5">
						{#if consoleState.settings.cardLayout === 'horizontal'}
							<div class="flex min-w-0 items-center justify-between gap-2">
								<div class="flex min-w-0 items-center gap-2">
									<CardTitle class="truncate text-sm">{account.label}</CardTitle>
									{@render kindBadge(account)}
								</div>
								{@render statusBadge(account)}
							</div>
						{:else}
							<div class="flex min-w-0 flex-col gap-1.5">
								<CardTitle class="truncate text-sm">{account.label}</CardTitle>
								<div class="flex min-w-0 flex-wrap items-center gap-2">
									{@render kindBadge(account)}
									{@render statusBadge(account)}
								</div>
							</div>
						{/if}
						<CardDescription class="flex flex-wrap items-center gap-2 truncate text-xs">
							<span class="truncate font-mono">{account.secret_mask ?? '••••'}</span>
							{#if provider.models_per_account}
								<span>·</span>
								<span>
									{account.models_known
										? $t('ui.pages.providersPage.accounts.modelsKnown', {
												values: { count: account.models_count ?? 0 }
											})
										: $t('ui.pages.providersPage.accounts.modelsUnknown')}
								</span>
							{/if}
						</CardDescription>
					</CardHeader>
					<CardContent class="flex flex-col gap-2 text-xs">
						<div>
							{#if needsSignIn(account) && signingIn === account.id}
								<div class="w-full">
									<SignInPanel
										methods={providerMethods}
										label={account.label}
										lockLabel
										ondone={() => {
											signingIn = '';
											void refreshModels();
										}}
										oncancel={() => (signingIn = '')}
									/>
								</div>
							{:else}
								<QuotaMeterRows
									windows={needsSignIn(account) ? [] : (windowsOf.get(account.id) ?? [])}
									showSource
								>
									{#snippet empty()}
										{#if needsSignIn(account)}
											<Button type="button" size="sm" onclick={() => (signingIn = account.id)}>
												<Icon name="refresh" size={14} />
												{$t('ui.pages.providersPage.accounts.statusReauth')}
											</Button>
										{:else}
											<p
												class="rounded-lg border border-dashed border-border px-3 py-1.5 text-muted-foreground"
											>
												{$t('ui.pages.providersPage.accounts.quotaNone')}
											</p>
										{/if}
									{/snippet}
								</QuotaMeterRows>
							{/if}
						</div>
						<!-- Row actions are icon buttons: what changes the account's
						     state sits on the left, and what sizes or rereads it sits
						     on the right, so the two groups never read as one. -->
						<div class="flex items-center gap-1 border-t border-border pt-3">
							<IconAction
								icon={account.status === 'active' ? 'player-pause' : 'player-play'}
								label={account.status === 'active'
									? $t('ui.pages.providersPage.accounts.pause')
									: $t('ui.pages.providersPage.accounts.resume')}
								disabled={working}
								onclick={() =>
									setStatus(account, account.status === 'active' ? 'paused' : 'active')}
							/>
							<Button
								variant="destructive"
								size="sm"
								disabled={working}
								onclick={() => {
									removeFailed = '';
									removing = account;
									removeOpen = true;
								}}
							>
								<Icon name="trash" size={13} />
								{$t('ui.pages.providersPage.accounts.remove')}
							</Button>
							<div class="ms-auto flex items-center gap-1">
								<AccountContextControl
									{account}
									disabled={working}
									onnotify={(message) => (notice = message)}
								/>
								{#if provider.models_per_account}
									<IconAction
										icon="refresh"
										label={$t('ui.pages.providersPage.accounts.readModels')}
										disabled={working}
										onclick={() => readModels(account)}
									/>
								{/if}
							</div>
						</div>
					</CardContent>
				</Card>
			{/each}
		</div>
	{/if}

	{#if notice}
		<p class="text-xs text-muted-foreground" role="status">{notice}</p>
	{/if}
</div>

<ConfirmDialog
	bind:open={removeOpen}
	title={removing
		? $t('ui.pages.providersPage.accounts.removeTitle', { values: { label: removing.label } })
		: ''}
	body={accounts.length === 1 && needsCredential
		? $t('ui.pages.providersPage.accounts.removeLastBody', { values: { provider: provider.label } })
		: $t('ui.pages.providersPage.accounts.removeBody')}
	confirmLabel={$t('ui.pages.providersPage.accounts.remove')}
	tone="destructive"
	icon="trash"
	busy={removingBusy}
	error={removeFailed}
	onconfirm={remove}
/>
