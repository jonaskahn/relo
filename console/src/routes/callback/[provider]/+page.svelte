<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { flip } from 'svelte/animate';
	import { cubicOut } from 'svelte/easing';
	import { fade, fly } from 'svelte/transition';

	import { page } from '$app/state';

	import CallbackEnjoy from '$lib/components/providers/callback-enjoy.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import LogoMark from '$lib/components/ui/logo-mark.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import { pageMotion } from '$lib/page-motion';
	import { prefersReducedMotion } from '$lib/tab-motion';
	import type { CallbackStatus } from '$lib/types';
	import {
		readMs,
		expiredState,
		initialState,
		isFinal,
		pollCallback,
		statusPath,
		windMs,
		type CallbackState
	} from './login.svelte';

	// The page a browser lands on after a provider redirect. It carries no
	// session, so it polls one login by the ticket the daemon put in the address
	// and shows how far that login got.
	const provider = $derived(page.params.provider ?? '');
	const ticket = $derived(page.url.searchParams.get('ticket') ?? '');

	let login = $state<CallbackState>(initialState());
	// The page's own arc, in order: it reports the login while that runs, then
	// the result arrives, then the page comes apart around it. Separate states
	// rather than one flag because each has to be true on its own — the wind is
	// a class on the same <main> the result is drawn in, so the two cannot
	// share a boolean.
	type Leave = 'working' | 'result' | 'dissolve';
	let leave = $state<Leave>('working');
	let reduced = $state(false);

	// Every moment below is gated on this one read, so a visitor who asked for
	// reduced motion gets a page that changes state without moving.
	const durations = $derived({
		step: reduced ? 0 : 180,
		fade: reduced ? 0 : 180,
		swap: reduced ? 0 : 160
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

	/** Closes the window the sign-in opened, and does nothing else. A tab the
	 *  operator navigated to by hand is not one a script may close, and the
	 *  browser refuses the call quietly rather than failing, so the page has
	 *  simply finished there. */
	function closeTab() {
		// The window a provider redirected through could still reach this page's
		// opener, so the reference goes before anything else.
		window.opener = null;
		window.close();
	}

	/** Arms the page's leave. The result arrives as soon as the login settles —
	 *  there is no wait before it, because the result is the only thing this tab
	 *  has left to say. It then sits still long enough to be read, and only then
	 *  does the page come apart.
	 *
	 *  The waits are their own timers rather than part of the poll's schedule, so
	 *  a slow poll cannot stretch them. */
	function beginLeave(): void {
		// Reduced motion never comes apart, so the close must not wait for an
		// erosion that will not happen.
		const read = reduced ? 0 : readMs;
		const wind = reduced ? 0 : windMs;
		leave = 'result';
		setTimeout(() => {
			leave = 'dissolve';
			setTimeout(closeTab, wind);
		}, read);
	}

	onMount(() => {
		reduced = prefersReducedMotion();

		// No ticket means the redirect never reached the login that started it,
		// or one this daemon does not track. Either way there is nothing to
		// poll, and the page says so rather than waiting forever.
		if (!ticket) {
			settle(expiredState());
			return;
		}
		pollCallback({
			ticket,
			fetchStatus: readStatus,
			onChange: settle,
			schedule: (run, ms) => setTimeout(run, ms)
		});
	});

	/** Records a login's outcome. A login that has one begins the page's leave;
	 *  one still running stays put. */
	function settle(next: CallbackState): void {
		login = next;
		if (isFinal(next)) beginLeave();
	}

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

<!-- The page is taken apart by uncovering this. It has to be a colour the
     erosion can be seen against: <main> paints the same canvas the body does, so
     masking away a pixel would otherwise uncover the exact colour that was
     already there. The card on canvas carries that in the light theme, where
     white on near-white still reads as eroding, but not in the dark one, where
     the surface and canvas are nearly the same value and the erosion would run
     unseen. Mixing ink into canvas darkens one theme and lightens the other, so
     the edge reads in both. -->
<div aria-hidden="true" class="callback-bed"></div>

{#if leave === 'working'}
	<main class="flex min-h-dvh items-center justify-center bg-canvas px-4 py-8">
		<div
			in:pageMotion
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

			<div class="mt-6 flex min-w-0 flex-col gap-0.5">
				<span class="text-xs text-muted-foreground">{$t('ui.callback.closeHint')}</span>
			</div>
		</div>
	</main>
{:else}
	<!-- The result, and then its end. Both live on one <main>, because the
	     erosion has to take the whole page and not just the panel in it: masking
	     away a card-sized region would leave the canvas standing around a hole,
	     which reads as a broken render rather than as a page that has finished. -->
	<main
		class="flex min-h-dvh items-center justify-center bg-canvas px-4 py-8"
		class:callback-wind={leave === 'dissolve'}
		style:--wind-ms={`${windMs}ms`}
	>
		<CallbackEnjoy ok={login.phase === 'connected'} account={login.account} />
	</main>
{/if}

<style>
	/* What the wind uncovers. It sits under <main>, which paints the canvas
	   itself, so without it a masked-away pixel would reveal the colour that
	   was already there. Two tokens and a mix, so the tone inverts with the
	   theme: a shade darker than the canvas in light, a shade lighter in dark. */
	.callback-bed {
		position: fixed;
		inset: 0;
		z-index: -1;
		background: color-mix(in srgb, var(--ink) 6%, var(--canvas));
	}

	.callback-wind {
		/* One cell of the grid. Three device-independent pixels reads as pixels
		   at 1x and stays finer than a hairline at 2x; any larger and the page
		   reads as blocks going missing rather than as pixels. */
		--wind-px: 3px;
		/* How soft the edge is. Tight on purpose: a wide band reads as a wipe
		   with a texture on it, a narrow one as the page coming apart. */
		--wind-edge: 8%;
	}

	@keyframes callback-wind {
		from {
			mask-position: 0 100%;
		}
		to {
			mask-position: 0 0;
		}
	}

	/* Two masks multiplied, and both read the one animated position, so the
	   whole stack travels as one. A mask is read only as alpha, so none of the
	   colours below reaches a pixel: white is what "opaque" means to a mask,
	   transparent is what "taken" means. */
	@media (prefers-reduced-motion: no-preference) {
		.callback-wind {
			-webkit-mask-image:
				/* The travelling edge, on the diagonal. `to top right` is the
				   wind's direction, and a hard stop keeps steps() stepping a
				   boundary rather than a gradient. */
				linear-gradient(to top right, transparent 0, white var(--wind-edge)),
				/* The dither. Full-element, so it travels with the edge instead of
				   sitting in a cell-sized window where it could not move, and
				   repeating at the cell size, so what it multiplies breaks into
				   pixels as the edge passes over it. */
					repeating-conic-gradient(
						white 0% 25%,
						transparent 25% 50%,
						white 50% 75%,
						transparent 75% 100%
					);
			-webkit-mask-size:
				100% 100%,
				var(--wind-px) var(--wind-px);
			-webkit-mask-repeat: no-repeat, repeat;
			-webkit-mask-composite: source-in;
			mask-image:
				linear-gradient(to top right, transparent 0, white var(--wind-edge)),
				repeating-conic-gradient(
					white 0% 25%,
					transparent 25% 50%,
					white 50% 75%,
					transparent 75% 100%
				);
			mask-size:
				100% 100%,
				var(--wind-px) var(--wind-px);
			mask-repeat: no-repeat, repeat;
			mask-composite: intersect;
			/* steps() is what makes this read as pixels rather than as a wipe:
			   the edge advances in discrete bands instead of gliding. The count is
			   a plain integer because steps() takes one — deriving it from the
			   duration with calc() would resolve to a time, which is invalid here
			   and would drop the whole declaration, leaving no motion at all. 100
			   bands over the 3s is 30ms a band: coarse enough to see, fine enough
			   not to strobe. */
			animation: callback-wind var(--wind-ms) steps(100, end) forwards;
		}
	}
</style>
