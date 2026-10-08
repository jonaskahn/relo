<script lang="ts">
	import type { Snippet } from 'svelte';
	import { t } from 'svelte-i18n';

	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import QuotaMeterRows from '$lib/components/providers/quota-meter-rows.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import {
		boardOverflowLabel,
		formatCountdown,
		visibleBoard,
		type AccountHealthState,
		type HealthCard
	} from '$lib/dashboard';
	import type { QuotaWindow } from '$lib/types';

	// HealthBoard is the accounts half of the dashboard's second row: one card
	// per account, the ones that need attention first, each opening its
	// connection. The page owns the reads and hands the board what they
	// answered, so the board decides nothing about traffic.
	interface Props {
		cards: HealthCard[];
		loading: boolean;
		failed: string;
		windowsOf: (accountId: string) => QuotaWindow[];
		onretry: () => void;
		onadd: () => void;
		pending: Snippet;
	}

	let { cards, loading, failed, windowsOf, onretry, onadd, pending }: Props = $props();

	// The one state a card shows, as the pill at its end reads it.
	const chip: Record<AccountHealthState, { kind: 'pass' | 'warn' | 'fail'; labelKey: string }> = {
		needs_signin: { kind: 'fail', labelKey: 'ui.pages.dashboard.stateNeedsSignin' },
		paused: { kind: 'warn', labelKey: 'ui.pages.dashboard.statePaused' },
		limited: { kind: 'warn', labelKey: 'ui.pages.dashboard.stateLimited' },
		exhausted: { kind: 'warn', labelKey: 'ui.pages.dashboard.stateExhausted' },
		near: { kind: 'warn', labelKey: 'ui.pages.dashboard.stateNear' },
		ready: { kind: 'pass', labelKey: 'ui.pages.dashboard.stateReady' }
	};

	const overflow = $derived(boardOverflowLabel(cards.length));
	const shown = $derived(visibleBoard(cards));
</script>

<div class="section-surface accounts-board flex min-w-0 flex-col">
	<div class="px-4 pt-4 pb-3 sm:px-5">
		<h2 class="section-heading">{$t('ui.pages.dashboard.accountsBoardTitle')}</h2>
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.dashboard.accountsBoardDesc')}{#if !loading && overflow}{' '}{overflow}{/if}
		</p>
	</div>
	<div class="flex flex-col gap-2 px-4 pb-4 sm:px-5">
		{#if loading}
			{@render pending()}
		{:else if failed}
			<div class="flex items-center justify-between gap-3">
				<p class="text-sm text-danger">{failed}</p>
				<Button variant="outline" size="sm" onclick={onretry}>
					<Icon name="refresh" size={14} />
					{$t('ui.common.retry')}
				</Button>
			</div>
		{:else if cards.length === 0}
			<div
				class="flex flex-col items-center gap-3 rounded-panel border-[1.5px] border-dashed border-accent-border bg-accent-soft px-4 py-8 text-center"
			>
				<p class="text-sm text-muted-foreground">{$t('ui.pages.dashboard.accountsEmpty')}</p>
				<Button size="sm" onclick={onadd}>
					<Icon name="plus" size={14} />
					{$t('ui.pages.providersPage.header.addProvider')}
				</Button>
			</div>
		{:else}
			<!-- Two cards per row once the board itself is wide enough: it is
			     half the page on a wide screen and the rail can take a column,
			     so the board measures itself, not the viewport. -->
			<div class="accounts-grid">
				{#each shown as card (card.account.id)}
					{@const state = chip[card.health.state]}
					<a
						href={'/connections?provider=' +
							encodeURIComponent(card.account.provider_id) +
							'&tab=accounts'}
						class="flex min-w-0 flex-col gap-3 rounded-panel border border-line bg-surface p-3 transition-[border-color,box-shadow] duration-150 hover:border-accent-border hover:shadow-[var(--shadow-hover)]"
					>
						<div class="flex min-w-0 items-center gap-2.5">
							{#if card.provider}<ProviderLogo
									id={card.provider.id}
									label={card.provider.label}
									size="sm"
								/>{/if}
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-ink">{card.account.label}</p>
								{#if card.provider}<p class="truncate text-xs text-muted-foreground">
										{card.provider.label}
									</p>{/if}
							</div>
							<StatusBadge kind={state.kind} label={$t(state.labelKey)} />
						</div>
						{#if card.health.state === 'needs_signin'}
							<p class="flex items-center gap-1.5 text-xs font-medium text-danger">
								<Icon name="user" size={13} />{$t('ui.pages.dashboard.stateNeedsSignin')}
							</p>
						{:else}
							<div class="rounded-panel bg-sunken p-3">
								<QuotaMeterRows windows={windowsOf(card.account.id)} />
							</div>
							{#if card.health.state === 'limited' && card.health.limitedUntilMs > 0}
								<p class="font-mono text-[0.7rem] text-muted-foreground">
									{$t('ui.pages.dashboard.resumesIn', {
										values: { time: formatCountdown(card.health.limitedUntilMs - Date.now()) }
									})}
								</p>
							{/if}
						{/if}
					</a>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	/* The accounts board changes between one and two cards per row on its own
	   width: at half the page its width depends on the rail beside it, which
	   a viewport breakpoint cannot see. Two cards need room for the quota
	   meter's fixed label, number and time beside a readable bar. */
	.accounts-board {
		container-type: inline-size;
	}

	.accounts-grid {
		display: grid;
		grid-template-columns: minmax(0, 1fr);
		gap: 0.5rem;
	}

	@container (min-width: 32rem) {
		.accounts-grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
		}
	}
</style>
