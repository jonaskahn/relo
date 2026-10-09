<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Account, LoginMethod, Provider, QuotaWindow } from '$lib/types';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import AccountDot from '$lib/components/ui/account-dot.svelte';
	import { selectedAccount, windowsByCredential } from '$lib/provider-quota';
	import { accountNeedsSignIn } from '$lib/provider-status';
	import type { CardLayout } from '$lib/theme.svelte';
	import QuotaMeterRows from './quota-meter-rows.svelte';
	import SignInPanel from './sign-in-panel.svelte';

	// ProviderQuotaFooter is the bottom area of a connection card: the accounts
	// it holds as numbered dots, and the quota the chosen one reported. The
	// selected account name lives in the card header; this footer keeps the
	// dots and the meters. An account that has to sign in again uses that same
	// slot for the action.
	interface Props {
		provider: Provider;
		accounts?: Account[];
		windows?: QuotaWindow[];
		layout?: CardLayout;
		// needsCredentials tells an empty footer apart: a connection that signs
		// in is missing an account, a local engine never had one.
		needsCredentials?: boolean;
		picked?: number;
		onpick?: (index: number) => void;
		onreload?: () => void;
	}

	let {
		provider,
		accounts = [],
		windows = [],
		layout = 'vertical',
		needsCredentials = true,
		picked = 0,
		onpick,
		onreload
	}: Props = $props();

	const gutter = $derived(layout === 'horizontal' ? 'px-6' : 'px-4');
	// More packs four narrower cards, so eight numbers stay in view. Less packs
	// three wider cards, so ten stay in view. One more and this row scrolls.
	const visibleDots = $derived(layout === 'horizontal' ? 10 : 8);
	// size-7 is 1.75rem and gap-1.5 is 0.375rem. A quarter-rem of slack keeps
	// the last number in view from tripping a scrollbar.
	const dotsMax = $derived(
		`min(100%, ${visibleDots * 1.75 + (visibleDots - 1) * 0.375 + 0.25}rem)`
	);

	let signingIn = $state(false);

	const byCredential = $derived(windowsByCredential(windows));
	const current = $derived(selectedAccount(accounts, picked));
	const reported = $derived(current ? (byCredential.get(current.id) ?? []) : []);
	const currentNeedsSignIn = $derived(current ? accountNeedsSignIn(current) : false);

	const providerMethods = $derived<LoginMethod[]>(
		provider.login_methods && provider.login_methods.length > 0
			? provider.login_methods
			: (provider.login_flows ?? []).map((flow) => ({ flow, kind: 'browser' }))
	);

	function chipLabel(account: Account, index: number): string {
		const values = { index: index + 1, label: account.label };
		return accountNeedsSignIn(account)
			? $t('ui.pages.providersPage.list.cardAccountChipReauth', { values })
			: $t('ui.pages.providersPage.list.cardAccountChip', { values });
	}

	function pick(index: number) {
		onpick?.(index);
		signingIn = false;
	}
</script>

<div class="mt-2 flex min-h-0 flex-col border-t border-border pt-2 text-xs">
	<div class="flex min-h-7 w-full min-w-0 shrink-0 items-center {gutter}">
		<div
			class="flex w-max shrink-0 items-center gap-1.5 overflow-x-auto overflow-y-hidden overscroll-x-contain"
			style:max-width={dotsMax}
			role="group"
			aria-label={$t('ui.pages.providersPage.list.cardAccounts')}
		>
			{#if accounts.length === 0 && !needsCredentials}
				<AccountDot
					index={1}
					label={$t('ui.pages.providersPage.list.cardAccountChip', {
						values: { index: 1, label: provider.label }
					})}
					selected
					onclick={() => {}}
				/>
			{:else}
				{#each accounts as account, index (account.id)}
					<AccountDot
						index={index + 1}
						label={chipLabel(account, index)}
						selected={current?.id === account.id}
						warn={accountNeedsSignIn(account)}
						onclick={() => pick(index)}
					/>
				{/each}
			{/if}
		</div>
	</div>

	<div class="{gutter} pt-2">
		{#if currentNeedsSignIn && signingIn}
			<div class="w-full">
				<SignInPanel
					methods={providerMethods}
					label={current?.label ?? ''}
					lockLabel
					ondone={() => {
						signingIn = false;
						onreload?.();
					}}
					oncancel={() => (signingIn = false)}
				/>
			</div>
		{:else}
			<QuotaMeterRows windows={current && needsCredentials && !currentNeedsSignIn ? reported : []}>
				{#snippet empty()}
					{#if !needsCredentials && !currentNeedsSignIn}
						<p
							class="font-mono text-3xl leading-none text-muted-foreground"
							aria-label={$t('ui.pages.providersPage.accounts.quotaUnlimited')}
						>
							∞
						</p>
					{:else if !current}
						<p
							class="rounded-lg border border-dashed border-border px-3 py-1.5 text-muted-foreground"
						>
							{$t('ui.pages.providersPage.accounts.empty')}
						</p>
					{:else if currentNeedsSignIn}
						<Button type="button" size="sm" onclick={() => (signingIn = true)}>
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
</div>
