<script lang="ts">
	import { t } from 'svelte-i18n';
	import { slide } from 'svelte/transition';
	import type { Account, Provider, QuotaWindow } from '$lib/types';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import Card from '$lib/components/ui/card.svelte';
	import CardDescription from '$lib/components/ui/card-description.svelte';
	import CardTitle from '$lib/components/ui/card-title.svelte';
	import { selectedAccount } from '$lib/provider-quota';
	import { isFreeConnection } from '$lib/provider-free';
import { isPaidPlanConnection, isPayAsYouGoConnection } from '$lib/provider-billing';
	import { isExperimentalConnection } from '$lib/provider-experimental';
	import { providerStatus, type ProviderStatusName } from '$lib/provider-status';
	import { ATTENTION_PERCENT } from '$lib/provider-attention';
	import { isCardOpenClick } from '$lib/card-target';
	import { cardLayoutCard, type CardLayout } from '$lib/theme.svelte';
	import ProviderQuotaFooter from './provider-quota-footer.svelte';

	// The card leads with what the connection is and the account on show, and
	// closes with what the whole connection will do: the models it can route
	// and the rows that still have no price.
	interface Props {
		provider: Provider;
		accounts?: Account[];
		windows?: QuotaWindow[];
		layout?: CardLayout;
		onopen: (id: string) => void;
		onunpriced?: (id: string) => void;
		onreload?: () => void;
	}

	let {
		provider,
		accounts = [],
		windows = [],
		layout = 'vertical',
		onopen,
		onunpriced,
		onreload
	}: Props = $props();

	let picked = $state(0);
	let hoverFree = $state(false);
	let hoverPayg = $state(false);
	let hoverPaid = $state(false);
	let statusHovered = $state(false);
	const current = $derived(selectedAccount(accounts, picked));
	const status = $derived(providerStatus(provider));

	// The quota the account reported is the card's own reading, so the pill
	// overrides the connection state when a window is near or at its end.
	const worst = $derived(
		windows.reduce((max, window) => {
			if (window.amount !== undefined) return max;
			return Math.max(max, window.used_percent);
		}, 0)
	);

	// Status is never carried by color alone: every state keeps its icon.
	const icons: Record<ProviderStatusName, IconName> = {
		ready: 'circle-check',
		paused: 'player-pause',
		needsSetup: 'alert-triangle',
		noAccount: 'user',
		signInAgain: 'lock',
		accountsPaused: 'player-pause',
		modelListFailed: 'circle-alert',
		noModelsOn: 'ban'
	};

	const pill = $derived.by((): { tone: string; icon: IconName; label: string } => {
		if (worst >= 100) {
			return {
				tone: 'bg-danger/10 text-danger',
				icon: 'alert-triangle',
				label: $t('ui.pages.providersPage.status.limitReached')
			};
		}
		if (worst >= ATTENTION_PERCENT) {
			return {
				tone: 'bg-warn/10 text-warn',
				icon: 'alert-triangle',
				label: $t('ui.pages.providersPage.status.nearLimit')
			};
		}
		return {
			tone:
				status.tone === 'pass'
					? 'bg-ok/10 text-ok'
					: status.tone === 'warn'
						? 'bg-warn/10 text-warn'
						: 'bg-muted text-muted-foreground',
			icon: icons[status.name],
			label: $t(status.labelKey)
		};
	});

	const modelsOn = $derived(
		$t('ui.pages.providersPage.list.modelsOn', {
			values: { on: provider.counts.enabled_models, total: provider.counts.models }
		})
	);
	const modelsPercent = $derived(
		provider.counts.models === 0
			? 0
			: Math.round((provider.counts.enabled_models / provider.counts.models) * 100)
	);
	const atLimit = $derived(worst >= 100);
	const pillName = $derived(
		worst >= 100 ? 'limitReached' : worst >= ATTENTION_PERCENT ? 'nearLimit' : status.name
	);
	const free = $derived(isFreeConnection(provider));
	const paidPlan = $derived(isPaidPlanConnection(provider));
	const payg = $derived(isPayAsYouGoConnection(provider));
	// The lane logs what it is sent, so the card says so beside what it costs.
	const experimental = $derived(isExperimentalConnection(provider.template_id || provider.id));
</script>

<Card
	class="section-surface {cardLayoutCard(layout)} gap-0 py-3 transition-colors card-press {atLimit
		? 'card-limit'
		: ''}"
	onclick={(event) => {
		if (isCardOpenClick(event)) onopen(provider.id);
	}}
>
	<div class="flex min-w-0 flex-col">
		<div class="flex items-start gap-2 px-4">
			<span class={provider.enabled ? '' : 'opacity-50'}>
				<ProviderLogo id={provider.template_id || provider.id} label={provider.label} />
			</span>
			<button
				type="button"
				data-provider-card={provider.id}
				data-plain
				class="min-w-0 flex-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
				aria-label={$t('ui.common.openCard', { values: { name: provider.label } })}
				onclick={() => onopen(provider.id)}
			>
				<CardTitle class="flex min-w-0 items-center gap-1.5 truncate text-sm">
					<span class="truncate">{provider.label}</span>
					{#if free}
						<button
							type="button"
							data-free-chip
							data-plain
							class="badge shrink-0 border border-ok/40 bg-ok/10 text-ok"
							aria-label={$t('ui.pages.providersPage.list.tagFree')}
							onmouseenter={() => (hoverFree = true)}
												onmouseleave={() => (hoverFree = false)}
						>
							<Icon name="sparkles" size={11} />
							{#if hoverFree}
								<span transition:slide={{ duration: 150, axis: "x" }} class="ms-1">
									{$t('ui.pages.providersPage.list.tagFree')}
								</span>
							{/if}
						</button>
					{/if}
					{#if payg}
						<span
							data-payg-chip
							class="badge shrink-0 border border-border bg-muted text-muted-foreground"
							aria-label={$t('ui.pages.providersPage.list.tagPayAsYouGo')}
							role="img"
							onmouseenter={() => (hoverPayg = true)}
												onmouseleave={() => (hoverPayg = false)}
						>
							<Icon name="currency-dollar" size={11} />
							{#if hoverPayg}
								<span transition:slide={{ duration: 150, axis: "x" }} class="ms-1">
									{$t('ui.pages.providersPage.list.tagPayAsYouGo')}
								</span>
							{/if}
						</span>
					{/if}
					{#if paidPlan}
						<span
							data-paid-chip
							class="badge shrink-0 border border-primary/40 bg-primary/10 text-primary"
							aria-label={$t('ui.pages.providersPage.list.tagPaidPlan')}
							role="img"
							onmouseenter={() => (hoverPaid = true)}
												onmouseleave={() => (hoverPaid = false)}
						>
							<Icon name="layers" size={11} />
							{#if hoverPaid}
								<span transition:slide={{ duration: 150, axis: "x" }} class="ms-1">
									{$t('ui.pages.providersPage.list.tagPaidPlan')}
								</span>
							{/if}
						</span>
					{/if}
					{#if experimental}
						<span
							data-experimental-chip
							class="badge shrink-0 border border-border text-muted-foreground"
						>
							{$t('ui.pages.providersPage.list.tagExperimental')}
						</span>
					{/if}
				</CardTitle>
				<CardDescription class="truncate font-mono text-xs">{provider.id}</CardDescription>
			</button>
			<span
				data-status-pill={pillName}
				class="group inline-flex shrink-0 items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium {pill.tone}"
				aria-label={pill.label}
				role="img"
				onmouseenter={() => (statusHovered = true)}
				onmouseleave={() => (statusHovered = false)}
			>
				<Icon name={pill.icon} size={12} />
				{#if statusHovered}
					<span transition:slide={{ duration: 150, axis: "x" }} class="ms-1">{pill.label}</span>
				{/if}
			</span>
		</div>
		<!-- The account line stays when a provider has none, so every header
		     in the row is the same height. -->
		<span
			data-account-line
			data-selected-account={current?.label}
			class="mt-1.5 flex h-5 min-w-0 items-center gap-1.5 px-4 text-sm font-medium"
		>
			<Icon name="user" size={13} class="shrink-0 text-faint" />
			<span class="truncate">{current?.label || '\u00a0'}</span>
		</span>
		<ProviderQuotaFooter
			{provider}
			{accounts}
			{windows}
			{layout}
			needsCredentials={provider.auth !== 'none'}
			{picked}
			onpick={(next) => (picked = next)}
			{onreload}
		/>
		<div class="mt-2 flex min-w-0 items-center gap-2 px-4 pt-3 text-xs">
			<span
				class="h-1.5 w-14 shrink-0 overflow-hidden rounded-full bg-accent-line"
				aria-hidden="true"
			>
				<span class="block h-full rounded-full bg-primary" style="width: {modelsPercent}%"></span>
			</span>
			<span class="min-w-0 truncate text-muted-foreground">{modelsOn}</span>
			{#if provider.counts.unpriced_models > 0}
				<button
					type="button"
					data-plain
					class="badge ms-auto shrink-0 border border-warn/40 text-warn hover:bg-warn/10"
					onclick={() => onunpriced?.(provider.id)}
				>
					<Icon name="tag" size={11} />
					{$t('ui.pages.providersPage.list.unpriced', {
						values: { count: provider.counts.unpriced_models }
					})}
				</button>
			{/if}
		</div>
	</div>
</Card>
