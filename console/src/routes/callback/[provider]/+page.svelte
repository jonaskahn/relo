<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { flip } from 'svelte/animate';
	import { cubicOut } from 'svelte/easing';
	import { fade, fly } from 'svelte/transition';

	import { page } from '$app/state';

	import Icon from '$lib/components/ui/icon.svelte';
	import LogoMark from '$lib/components/ui/logo-mark.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import { pageMotion } from '$lib/page-motion';
	import { prefersReducedMotion } from '$lib/tab-motion';
	import type { CallbackStatus } from '$lib/types';
	import {
		closeSeconds,
		expiredState,
		initialState,
		isFinal,
		pollCallback,
		statusPath,
		type CallbackState
	} from './login.svelte';

	// The page a browser lands on after a provider redirect. It carries no
	// session, so it polls one login by the ticket the daemon put in the address
	// and shows how far that login got.
	const provider = $derived(page.params.provider ?? '');
	const ticket = $derived(page.url.searchParams.get('ticket') ?? '');

	let login = $state<CallbackState>(initialState());
	// The countdown runs only once the login has an outcome, so the wait before
	// the page goes quiet is legible rather than a sudden change.
	let secondsLeft = $state<number | null>(null);
	let reduced = $state(false);

	// Every moment below is gated on this one read, so a visitor who asked for
	// reduced motion gets a page that changes state without moving.
	const durations = $derived({
		step: reduced ? 0 : 180,
		fade: reduced ? 0 : 180,
		swap: reduced ? 0 : 160,
		leave: reduced ? 0 : 200
	});

	/** The three steps a login walks, and how far this one has got them. */
	const steps = $derived(
		[
			{ key: 'authorized', label: $t('ui.callback.stepAuthorized', { values: { provider } }) },
			{ key: 'exchanging', label: $t('ui.callback.stepExchanging') },
			{ key: 'stored', label: $t('ui.callback.stepStored') }
		].map((step, index) => ({ ...step, state: stepState(index) }))
	);

	// An expired login hides its steps: there was no progress to report, and an
	// empty rail reads as a list that never loaded.
	const showSteps = $derived(login.phase !== 'expired');

	function stepState(index: number): 'done' | 'active' | 'failed' | 'pending' {
		if (login.phase === 'connected') return 'done';
		if (index === 0) return 'done';
		if (login.phase === 'expired') return 'pending';
		if (index === 1) return login.phase === 'failed' ? 'failed' : 'active';
		return 'pending';
	}

	/** The steps that reached an outcome draw the rail behind them in the
	 *  outcome's own colour, so the rail reports progress without a number. */
	const railTone = $derived(
		login.phase === 'connected'
			? 'bg-ok'
			: login.phase === 'failed'
				? 'bg-danger'
				: login.phase === 'working'
					? 'bg-accent'
					: 'bg-line'
	);

	const heading = $derived(
		login.phase === 'connected'
			? $t('ui.callback.connectedTitle')
			: login.phase === 'failed'
				? $t('ui.callback.failedTitle')
				: login.phase === 'expired'
					? $t('ui.callback.expiredTitle')
					: $t('ui.callback.heading')
	);

	const summary = $derived(
		login.phase === 'connected'
			? $t('ui.callback.connectedBody')
			: login.phase === 'failed'
				? $t('ui.callback.failure.' + failureKey())
				: login.phase === 'expired'
					? $t('ui.callback.expiredBody')
					: $t('ui.callback.summary')
	);

	/** The refusal the daemon named, mapped to the copy that explains it. A
	 *  refusal Relo does not name reads as the exchange itself having failed. */
	function failureKey(): string {
		switch (login.error) {
			case 'denied':
				return 'denied';
			case 'state':
				return 'state';
			case 'timeout':
				return 'timeout';
			default:
				return 'exchange';
		}
	}

	const badge = $derived(
		login.phase === 'connected'
			? { kind: 'pass' as const, label: $t('ui.callback.markerConnected') }
			: login.phase === 'working'
				? { kind: 'muted' as const, label: $t('ui.callback.markerWorking') }
				: { kind: 'fail' as const, label: $t('ui.callback.markerFailed') }
	);

	function countdownTick(): void {
		if (secondsLeft === null) return;
		if (secondsLeft <= 0) {
			secondsLeft = null;
			return;
		}
		secondsLeft -= 1;
	}

	onMount(() => {
		reduced = prefersReducedMotion();
		// No ticket means the redirect never reached the login that started it,
		// or one this daemon does not track. Either way there is nothing to
		// poll, and the page says so rather than waiting forever.
		if (!ticket) {
			login = expiredState();
			return;
		}
		pollCallback({
			ticket,
			fetchStatus: readStatus,
			onChange: (next) => {
				login = next;
				if (isFinal(next)) secondsLeft = closeSeconds;
			},
			schedule: (run, ms) => setTimeout(run, ms)
		});
		// The countdown runs on its own clock, so the poll and the sentence that
		// says how long is left do not share a schedule: a slow poll must not
		// hold the figure still.
		const countdown = setInterval(countdownTick, 1000);
		return () => clearInterval(countdown);
	});

	async function readStatus(current: string): Promise<CallbackStatus | null> {
		const response = await fetch(statusPath(provider, current), {
			headers: { Accept: 'application/json' },
			cache: 'no-store'
		});
		// The status endpoint answers a ticket it no longer tracks with a 404,
		// which is an expiry rather than an outage to retry through.
		if (response.status === 404) return null;
		if (!response.ok) throw new Error(String(response.status));
		return (await response.json()) as CallbackStatus;
	}
</script>

<svelte:head><title>{$t('ui.callback.documentTitle')}</title></svelte:head>

<main class="flex min-h-dvh items-center justify-center bg-canvas px-4 py-8">
	<div
		in:pageMotion
		out:fade={{ duration: durations.leave }}
		class="w-full max-w-md rounded-card border border-line bg-surface p-6 shadow-1"
	>
		<div class="flex items-center gap-2.5">
			<LogoMark class="size-6 text-accent-strong" />
			<ProviderLogo id={provider} label={provider} size="xs" />
			<span class="truncate text-xs font-medium text-muted-foreground" title={provider}>
				{provider}
			</span>
		</div>

		<h1 class="mt-4 text-base font-semibold text-ink">{heading}</h1>
		<p class="mt-2 max-w-[72ch] text-sm leading-relaxed text-muted-foreground">{summary}</p>

		<div class="mt-4" in:fade={{ duration: durations.swap }} out:fade={{ duration: 80 }}>
			<StatusBadge kind={badge.kind} label={badge.label} />
		</div>

		{#if showSteps}
			<ol class="mt-6 flex flex-col gap-0" aria-label={$t('ui.callback.stepsLabel')}>
				{#each steps as step, index (step.key)}
					{@const settled = step.state === 'done' || step.state === 'failed'}
					<li
						class="relative flex items-center gap-3 pb-4 last:pb-0"
						in:fly={{ y: 6, duration: durations.step, easing: cubicOut }}
						animate:flip={{ duration: durations.step }}
					>
						{#if index < steps.length - 1}
							<span
								aria-hidden="true"
								class="absolute top-6 left-[11px] h-[calc(100%-1.25rem)] w-px transition-colors duration-200 {railTone}"
							></span>
						{/if}
						<span
							class="relative z-10 flex size-6 shrink-0 items-center justify-center rounded-full bg-surface {settled
								? step.state === 'failed'
									? 'text-danger'
									: 'text-ok'
								: step.state === 'active'
									? 'text-accent-strong'
									: 'text-muted-foreground'}"
						>
							{#if step.state === 'done'}
								<Icon name="check" size={14} />
							{:else if step.state === 'failed'}
								<Icon name="circle-x" size={14} />
							{:else if step.state === 'active'}
								<Icon name="loader" size={14} spin />
							{:else}
								<Icon name="clock" size={14} />
							{/if}
						</span>
						<span
							class="text-sm {step.state === 'pending'
								? 'text-muted-foreground'
								: step.state === 'failed'
									? 'text-danger'
									: 'text-ink'}"
						>
							{step.label}
						</span>
					</li>
				{/each}
			</ol>
		{/if}

		{#if login.account}
			<p class="mt-5 text-sm text-muted-foreground">
				{$t('ui.callback.accountLabel')}:
				<span class="mono-data ml-1 text-xs text-ink">{login.account}</span>
			</p>
		{/if}

		<div class="mt-6 flex flex-wrap items-center justify-between gap-3">
			<div class="flex min-w-0 flex-col gap-0.5">
				<span class="text-xs text-muted-foreground">{$t('ui.callback.closeHint')}</span>
				{#if secondsLeft !== null}
					<span
						class="mono-data text-xs text-muted-foreground"
						in:fade={{ duration: durations.fade }}
					>
						{$t('ui.callback.closingIn', { values: { seconds: secondsLeft } })}
					</span>
				{/if}
			</div>
			<a
				href="/providers"
				class="inline-flex h-control shrink-0 items-center gap-1.5 rounded-control border border-line px-3 text-sm font-medium text-ink transition-colors duration-150 hover:bg-hover focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
			>
				{$t('ui.callback.consoleLink')}
				<Icon name="arrow-right" size={14} />
			</a>
		</div>
	</div>
</main>
