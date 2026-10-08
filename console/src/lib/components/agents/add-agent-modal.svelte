<script lang="ts">
	import { t } from 'svelte-i18n';

	import { ADD_STEPS, addStepBack, resultCodeFor, type AddStep } from '$lib/agent-flow';
	import { labelOf, logoIdOf, refusalDetailKey, refusalOf } from '$lib/agent-state';
	import { api } from '$lib/api';
	import type { DataPlaneURLs } from '$lib/client-setup';
	import { integrationPath, setupBlocked } from '$lib/integration-actions';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import WizardStepper from '$lib/components/ui/wizard-stepper.svelte';
	import AgentResultStep from '$lib/components/agents/agent-result-step.svelte';
	import AgentVerifyChecks from '$lib/components/agents/agent-verify-checks.svelte';
	import AgentWiringSection from '$lib/components/agents/agent-wiring-section.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import type {
		IntegrationResult,
		IntegrationVerify,
		IntegrationVerifyCheck,
		IntegrationView
	} from '$lib/types';

	// AddAgentModal wires one more agent: pick it, set it up, watch verification
	// prove the wiring, read the result. Verification runs itself for the agent
	// that was just set up; there is no batch verify. The page only reloads its
	// roster when the flow closes after doing work.
	interface Props {
		open?: boolean;
		agents: IntegrationView[];
		urls: DataPlaneURLs;
		onadded: () => void;
	}

	let { open = $bindable(false), agents, urls, onadded }: Props = $props();

	let step = $state<AddStep>('choose');
	let selected = $state<IntegrationView | null>(null);
	let working = $state(false);
	let failure = $state('');
	let checks = $state<IntegrationVerifyCheck[]>([]);
	let verified = $state(false);
	let verifyCopy = $state('');
	let result = $state<{ state: 'success' | 'failed'; copy: string; token: string } | null>(null);
	let revealCopied = $state(false);
	let didWork = $state(false);

	const stepLabels: Record<AddStep, string> = {
		choose: 'ui.pages.integrationsPage.addStepChoose',
		setup: 'ui.pages.integrationsPage.addStepSetup',
		verify: 'ui.pages.integrationsPage.addStepVerify',
		result: 'ui.pages.integrationsPage.addStepResult'
	};

	// Candidates are the agents with nothing wired yet, in page order. An
	// agent already on the grid is managed from its card instead.
	const candidates = $derived(agents.filter((agent) => !agent.enabled));
	const refusal = $derived(selected ? refusalOf(selected) : '');
	const blockedSetup = $derived(selected ? setupBlocked(refusalOf(selected)) : true);
	const refusalKey = $derived(selected && refusal ? refusalDetailKey(refusal) : '');

	function reset() {
		step = 'choose';
		selected = null;
		working = false;
		failure = '';
		checks = [];
		verified = false;
		verifyCopy = '';
		result = null;
		revealCopied = false;
		didWork = false;
	}

	// Opening the flow starts it from the choice, so a second visit never
	// shows what the previous one left behind.
	$effect(() => {
		if (open) reset();
	});

	function message(error: unknown): string {
		return error instanceof Error ? error.message : String(error);
	}

	// MIN_WORK_MS keeps setup and verification visible for at least two
	// seconds, so a fast daemon still reads as work instead of a flicker.
	const MIN_WORK_MS = 2000;

	function withMinDelay<T>(work: Promise<T>): Promise<T> {
		return Promise.all([work, new Promise((resolve) => setTimeout(resolve, MIN_WORK_MS))]).then(
			([answer]) => answer
		);
	}

	function requestClose() {
		open = false;
		if (didWork) {
			didWork = false;
			onadded();
		}
	}

	function pick(agent: IntegrationView) {
		selected = agent;
		failure = '';
	}

	function forward() {
		if (step === 'choose' && !selected) return;
		if (step === 'choose') {
			step = 'setup';
			return;
		}
		if (step === 'setup' && selected?.enabled) {
			step = 'verify';
			void runVerify();
			return;
		}
		if (step === 'setup') {
			void runSetup(false);
			return;
		}
		if (step === 'verify') step = 'result';
	}

	function back() {
		const previous = addStepBack(step);
		if (previous === null) {
			requestClose();
			return;
		}
		failure = '';
		step = previous;
	}

	// A foreign block is replaced only from the explicit replace control, which
	// names what restore puts back.
	async function runSetup(overwrite: boolean) {
		if (!selected || working || selected.enabled) return;
		working = true;
		failure = '';
		try {
			const answer = await withMinDelay(
				api<IntegrationResult>(integrationPath(selected.id, 'setup'), {
					method: 'POST',
					body: overwrite ? JSON.stringify({ overwrite: true }) : undefined
				})
			);
			selected = answer.integration;
			didWork = true;
			result = {
				state: 'success',
				copy: $t('ui.pages.integrationsPage.' + resultCodeFor('setup', answer.integration)),
				token: answer.token ?? ''
			};
			revealCopied = false;
			step = 'verify';
			// Verification runs with a free control: the setup above still
			// holds it until this assignment releases it.
			working = false;
			await runVerify();
		} catch (error) {
			failure = message(error);
		} finally {
			working = false;
		}
	}

	// The result step follows the outcome, so a failed proof reads as failed
	// even though the setup itself stored the key.
	async function runVerify() {
		if (!selected || working) return;
		working = true;
		try {
			const check = await withMinDelay(
				api<IntegrationVerify>(integrationPath(selected.id, 'verify'), { method: 'POST' })
			);
			checks = check.checks ?? [];
			verified = check.ok;
			verifyCopy = check.ok
				? $t(
						check.config
							? 'ui.pages.integrationsPage.verifiedConfig'
							: 'ui.pages.integrationsPage.verified',
						{
							values: { status: check.status }
						}
					)
				: $t('ui.pages.integrationsPage.verifyFailed', { values: { detail: check.message } });
			if (result) {
				result = {
					state: check.ok ? 'success' : 'failed',
					copy: check.ok ? result.copy : verifyCopy,
					token: result.token
				};
			}
		} catch (error) {
			checks = [];
			verified = false;
			verifyCopy = $t('ui.pages.integrationsPage.verifyFailed', {
				values: { detail: message(error) }
			});
			if (result) result = { ...result, state: 'failed', copy: verifyCopy };
		} finally {
			working = false;
		}
	}

	async function copyReveal() {
		if (!result?.token) return;
		try {
			await navigator.clipboard.writeText(result.token);
			revealCopied = true;
		} catch {
			revealCopied = false;
		}
	}
</script>

<CenteredModal
	bind:open
	size="wide"
	title={$t('ui.pages.integrationsPage.addTitle')}
	description={$t('ui.pages.integrationsPage.addSubtitle')}
	onOpenChange={(value) => {
		if (!value) requestClose();
	}}
>
	<div class="flex min-h-0 flex-1 flex-col gap-4">
		<WizardStepper
			steps={ADD_STEPS}
			current={step}
			labelKeys={stepLabels}
			onjump={(target) => {
				if (target === 'choose' || target === 'setup') {
					failure = '';
					step = target;
				}
			}}
			stepOfKey="ui.pages.integrationsPage.addStepOf"
		/>

		{#if step === 'choose'}
			{#if candidates.length === 0}
				<p class="py-8 text-center text-sm text-muted-foreground">
					{$t('ui.pages.integrationsPage.addAllWired')}
				</p>
			{:else}
				<!-- App drawer: one tile per agent, logo over label. Choosing
				     marks the tile; the footer's Next advances. -->
				<ul
					class="grid grid-cols-2 gap-2 sm:grid-cols-3"
					role="radiogroup"
					aria-label={$t('ui.pages.integrationsPage.addTitle')}
				>
					{#each candidates as agent (agent.id)}
						{@const chosen = selected?.id === agent.id}
						<li>
							<button
								type="button"
								data-plain
								role="radio"
								aria-checked={chosen}
								title={agent.summary || labelOf(agent)}
								class="flex min-h-28 w-full min-w-0 flex-col items-center justify-center gap-2 rounded-card border px-3 py-4 text-center transition-colors hover:bg-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {chosen
									? 'border-accent-border bg-accent-soft'
									: 'border-line bg-surface'}"
								onclick={() => pick(agent)}
							>
								<span class="relative shrink-0">
									<ProviderLogo id={logoIdOf(agent)} label={labelOf(agent)} size="sm" />
									{#if chosen}
										<span
											class="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full bg-primary text-primary-foreground"
											aria-hidden="true"
										>
											<Icon name="check" size={12} />
										</span>
									{/if}
								</span>
								<span class="min-w-0">
									<span class="block truncate text-sm font-medium text-ink">{labelOf(agent)}</span>
									{#if !agent.manages_files}
										<span class="block truncate text-xs text-muted-foreground">
											{$t('ui.pages.integrationsPage.addManual')}
										</span>
									{/if}
								</span>
							</button>
						</li>
					{/each}
				</ul>
			{/if}
		{:else if step === 'setup' && selected}
			{#if failure}
				<p class="flex items-start gap-2 text-sm text-danger" role="alert">
					<Icon name="alert-triangle" size={15} class="mt-0.5 shrink-0" />
					{failure}
				</p>
			{/if}
			{#if working}
				<!-- Slot loading: the step keeps its shape and the slot spins
				     for at least MIN_WORK_MS while setup runs. -->
				<div class="flex min-h-16 items-center justify-center py-8" role="status">
					<Icon name="loader" size={20} spin class="text-accent-strong" />
					<span class="sr-only">{$t('ui.common.loading')}</span>
				</div>
			{:else}
				<!-- The preview is the whole step: what Relo would write, with
				     the fragment and any refusal reason, before anything changes. -->
				<AgentWiringSection agent={selected} />
				{#if refusal === 'foreign_key' && !selected.enabled}
					<div class="rounded-panel border border-warn/40 bg-warn/5 px-3 py-2.5">
						<p class="text-sm text-ink">{$t('ui.pages.integrationsPage.overwriteBody')}</p>
					</div>
				{:else if refusalKey && !selected.enabled}
					<p class="text-sm text-muted-foreground">{$t(refusalKey)}</p>
				{/if}
			{/if}
		{:else if step === 'verify' && selected}
			{#if working}
				<div class="flex min-h-16 items-center justify-center py-4" role="status">
					<Icon name="loader" size={18} spin class="text-accent-strong" />
					<span class="sr-only">{$t('ui.common.loading')}</span>
				</div>
			{:else}
				{#if verifyCopy}
					<p
						class="flex items-start gap-2 text-sm {verified ? 'text-ink' : 'text-danger'}"
						role={verified ? undefined : 'alert'}
					>
						<Icon
							name={verified ? 'circle-check' : 'circle-x'}
							size={15}
							class="mt-0.5 shrink-0 {verified ? 'text-ok' : ''}"
						/>
						{verifyCopy}
					</p>
				{/if}
				{#if checks.length > 0}
					<AgentVerifyChecks {checks} />
				{/if}
			{/if}
		{:else if step === 'result' && selected && result}
			<AgentResultStep
				agent={selected}
				state={result.state}
				copy={result.copy}
				token={result.token}
				{urls}
				copied={revealCopied}
				oncopy={() => void copyReveal()}
			/>
		{/if}
	</div>

	{#snippet footer()}
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div class="flex min-w-0 items-center gap-2" role="status">
				{#if selected}
					<span class="min-w-0 truncate text-sm font-semibold">{labelOf(selected)}</span>
				{:else}
					<span class="truncate text-sm text-muted-foreground">
						{$t('ui.pages.integrationsPage.addChooseHint')}
					</span>
				{/if}
			</div>
			<div class="flex shrink-0 items-center gap-2">
				<Button variant="ghost" onclick={back} disabled={working}>
					<Icon name={step === 'choose' ? 'x' : 'arrow-left'} size={14} />
					{#if step === 'choose'}
						{$t('ui.common.cancel')}
					{:else}
						{$t('ui.pages.providersPage.pane.back')}
					{/if}
				</Button>
				{#if step === 'choose'}
					<Button disabled={!selected} onclick={forward}>
						<Icon name="chevron-right" size={14} />
						{$t('ui.pages.providersPage.add.next')}
					</Button>
				{:else if step === 'setup' && selected}
					{#if selected.enabled}
						<Button onclick={forward}>
							<Icon name="chevron-right" size={14} />
							{$t('ui.pages.providersPage.add.next')}
						</Button>
					{:else if refusal === 'foreign_key'}
						<Button disabled={working} onclick={() => void runSetup(true)}>
							<Icon name={working ? 'loader' : 'arrow-back-up'} size={14} spin={working} />
							{$t('ui.pages.integrationsPage.overwriteAction')}
						</Button>
					{:else}
						<Button disabled={working || blockedSetup} onclick={() => void runSetup(false)}>
							<Icon name={working ? 'loader' : 'chevron-right'} size={14} spin={working} />
							{$t('ui.pages.providersPage.add.next')}
						</Button>
					{/if}
				{:else if step === 'verify'}
					<Button variant="outline" disabled={working} onclick={() => void runVerify()}>
						<Icon name="circle-check" size={14} />
						{$t('ui.pages.integrationsPage.verify')}
					</Button>
					<Button disabled={working} onclick={forward}>
						<Icon name="check" size={14} />
						{$t('ui.pages.providersPage.verify.continue')}
					</Button>
				{:else if step === 'result'}
					<Button onclick={requestClose}>
						<Icon name="check" size={14} />
						{$t('ui.common.done')}
					</Button>
				{/if}
			</div>
		</div>
	{/snippet}
</CenteredModal>
