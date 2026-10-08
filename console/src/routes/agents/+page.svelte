<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';

	import { agentAttention } from '$lib/agent-attention.svelte';
	import {
		blocked,
		overwrittenFiles,
		resultCodeFor,
		runningOn,
		stepAfter,
		type AgentAction,
		type AgentStep,
		type ConfirmAction,
		type Running,
		type Work
	} from '$lib/agent-flow';
	import {
		contextOn,
		isTested,
		labelOf,
		patchAgent,
		refusalOf,
		splitAgents,
		stateKey,
		stateTone
	} from '$lib/agent-state';
	import { api } from '$lib/api';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import PageHeader from '$lib/components/ui/page-header.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import AgentCard from '$lib/components/agents/agent-card.svelte';
	import AgentConfirm from '$lib/components/agents/agent-confirm.svelte';
	import AddAgentModal from '$lib/components/agents/add-agent-modal.svelte';
	import AgentKeySection from '$lib/components/agents/agent-key-section.svelte';
	import AgentResultStep from '$lib/components/agents/agent-result-step.svelte';
	import AgentTestPane from '$lib/components/agents/agent-test-pane.svelte';
	import AgentWiringSection from '$lib/components/agents/agent-wiring-section.svelte';
	import CardLayoutToggle from '$lib/components/providers/card-layout-toggle.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import { consoleState } from '$lib/console-state.svelte';
	import { cardLayoutGrid } from '$lib/theme.svelte';
	import { integrationPath, needsRepair, setupBlocked } from '$lib/integration-actions';
	import type { DataPlaneURLs } from '$lib/client-setup';
	import { toast } from 'svelte-sonner';
	import type {
		DataPlaneAddress,
		IntegrationChat,
		IntegrationModel,
		IntegrationModels,
		IntegrationResult,
		IntegrationVerify,
		IntegrationView,
		StatusResponse
	} from '$lib/types';

	let agents = $state.raw<IntegrationView[]>([]);
	let loading = $state(true);
	let failure = $state('');

	// running names the one action in flight, so its own control carries the
	// spinner while every other control stays live: one agent's work never
	// freezes the page.
	let running = $state<Running>(null);
	// cardFailure is the one failure an action on a card reports inline, since the
	// card is the surface the operator is looking at.
	let cardFailure = $state<{ id: string; text: string } | null>(null);

	// The detail modal is the whole surface for one agent. Its body and footer swap
	// per step, so setup, its result and the one-time key are steps of this one
	// modal rather than modals stacked on it.
	let detailOpen = $state(false);
	let detail = $state.raw<IntegrationView | null>(null);
	let step = $state<AgentStep>('view');
	let result = $state<{ state: 'success' | 'failed'; copy: string; token: string } | null>(null);
	let confirmFailure = $state('');

	// The key section reads the stored secret on demand, so the eye control shows
	// what the agent actually runs with rather than only the hint. It is cleared
	// when the modal closes.
	let keyVisible = $state(false);
	let keyToken = $state('');
	let keyLoading = $state(false);
	let keyCopied = $state(false);
	let keyFailure = $state('');
	// A minted token exists once, in memory, for the step that produced it.
	let revealCopied = $state(false);

	// The confirmation an irreversible action opens once the modal has closed.
	let confirming = $state<ConfirmAction | null>(null);
	let pendingConfirm = $state<ConfirmAction | null>(null);
	let confirmOpen = $state(false);
	let confirmBusy = $state(false);

	// The test side: the models the agent's own key reaches and one turn sent
	// through it. Both travel the data plane the agent is pointed at.
	let models = $state<IntegrationModel[]>([]);
	let modelsState = $state<'idle' | 'loading' | 'ready' | 'failed'>('idle');
	let modelsMessage = $state('');
	let modelQuery = $state('');
	let modelProvider = $state('');
	let testModel = $state('');
	let testPrompt = $state('');
	let testing = $state(false);
	let testAnswer = $state<IntegrationChat | null>(null);
	let verifying = $state(false);
	let verifyRead = $state<{ ok: boolean; copy: string } | null>(null);

	// The address a client points at, read from the daemon rather than written
	// here, so a snippet never names a port nothing serves.
	let dataPlane = $state<DataPlaneAddress[]>([]);
	const urls = $derived<DataPlaneURLs>({
		openai:
			(dataPlane.find((entry) => entry.protocol === 'openai') ?? dataPlane[0])?.base_url ??
			'http://127.0.0.1:10201',
		anthropic:
			(dataPlane.find((entry) => entry.protocol === 'anthropic') ?? dataPlane[1])?.base_url ??
			'http://127.0.0.1:10202'
	});

	const { configured } = $derived(splitAgents(agents));

	// addOpen is the add-agent flow: choose, set up, verify, done. The roster
	// underneath reloads when the flow closes after doing work.
	let addOpen = $state(false);

	onMount(() => void load());

	// A silent re-sync replaces entries in place without showing a skeleton, so
	// the cards an operator is reading keep their order and their scroll.
	async function load(silent = false) {
		if (!silent) failure = '';
		// The roster and the data plane are separate reads, so they go out
		// together rather than one after the other.
		const [roster, status] = await Promise.allSettled([
			api<{ items: IntegrationView[] }>('/integrations'),
			api<StatusResponse>('/status')
		]);
		loading = false;
		if (roster.status === 'fulfilled') {
			agents = roster.value.items ?? [];
			agentAttention.set(agents);
			if (detail) detail = agents.find((agent) => agent.id === detail?.id) ?? null;
		} else if (!silent) {
			failure = message(roster.reason);
		}
		dataPlane = status.status === 'fulfilled' ? (status.value.data_plane ?? []) : [];
	}

	function message(error: unknown): string {
		return error instanceof Error ? error.message : String(error);
	}

	function openDetail(agent: IntegrationView) {
		detail = agent;
		step = 'view';
		result = null;
		confirmFailure = '';
		revealCopied = false;
		verifyRead = null;
		resetKey();
		resetTest();
		detailOpen = true;
		if (agent.enabled) void loadModels();
	}

	function closeDetail() {
		detailOpen = false;
		// A token exists once. Closing the modal drops it from memory, so it cannot
		// be read back later.
		resetKey();
	}

	// A second agent never shows the first agent's key.
	function resetKey() {
		keyVisible = false;
		keyToken = '';
		keyCopied = false;
		keyFailure = '';
	}

	// A second agent never shows the first agent's models or answer.
	function resetTest() {
		models = [];
		modelsState = 'idle';
		modelsMessage = '';
		testModel = '';
		testPrompt = '';
		testAnswer = null;
		modelQuery = '';
		modelProvider = '';
	}

	// The list keeps its order and the open step survives, so the page never
	// reloads underneath the operator.
	async function patch(answer: IntegrationResult) {
		agents = patchAgent(agents, answer.integration);
		agentAttention.set(agents);
		detail = agents.find((agent) => agent.id === answer.integration.id) ?? answer.integration;
		if (detail?.enabled && modelsState === 'idle') void loadModels();
		void load(true);
	}

	// The outcome lands in the modal the work happened in, so the operator reads
	// it before leaving. The step the flow lands on is the flow's own decision: a
	// key the action minted is revealed here, once.
	function reportResult(answer: IntegrationResult, action: AgentAction) {
		result = {
			state: 'success',
			copy: $t('ui.pages.integrationsPage.' + resultCodeFor(action, answer.integration)),
			token: answer.token ?? ''
		};
		revealCopied = false;
		step = stepAfter(answer, action);
	}

	async function run(action: AgentAction, agent: IntegrationView) {
		running = { id: agent.id, action };
		cardFailure = null;
		try {
			const answer = await api<IntegrationResult>(integrationPath(agent.id, action), {
				method: 'POST'
			});
			await patch(answer);
			// Set up, repair and rotation report their result in the modal before the
			// operator leaves; the rest report in a toast carrying the action's verb.
			if (action === 'setup' || action === 'repair' || action === 'rotate') {
				openDetail(answer.integration);
				reportResult(answer, action);
			} else {
				toast.success($t('ui.pages.integrationsPage.' + resultCodeFor(action, answer.integration)));
			}
		} catch (error) {
			// A failure stays next to the control that failed, with the reason and
			// the fix, rather than in a toast the operator did not ask for. The modal
			// only reports for the agent it is showing.
			if (detailOpen && detail?.id === agent.id) {
				result = {
					state: 'failed',
					copy: $t('ui.pages.integrationsPage.actionFailedWith', {
						values: { action, detail: message(error) }
					}),
					token: ''
				};
				step = 'result';
			} else {
				cardFailure = { id: agent.id, text: message(error) };
			}
		} finally {
			running = null;
		}
	}

	async function toggleContext(agent: IntegrationView) {
		running = { id: agent.id, action: 'context' };
		cardFailure = null;
		const wasOn = contextOn(agent);
		try {
			const answer = await api<IntegrationView>(integrationPath(agent.id, 'context'), {
				method: 'POST',
				body: JSON.stringify({ enabled: !wasOn })
			});
			agents = patchAgent(agents, answer);
			agentAttention.set(agents);
			detail = agents.find((entry) => entry.id === agent.id) ?? detail;
			void load(true);
			toast.success(
				$t(
					wasOn ? 'ui.pages.integrationsPage.context1MOff' : 'ui.pages.integrationsPage.context1MOn'
				)
			);
		} catch (error) {
			cardFailure = { id: agent.id, text: message(error) };
		} finally {
			running = null;
		}
	}

	// The proof is reported beside the wiring it proves: a line in the test pane,
	// never a toast. A passing proof reloads the model list, so the pane stays
	// fresh without its own control.
	async function verifyNow() {
		if (!detail?.enabled || verifying) return;
		verifying = true;
		try {
			const check = await api<IntegrationVerify>(integrationPath(detail.id, 'verify'), {
				method: 'POST'
			});
			verifyRead = {
				ok: check.ok,
				copy: check.ok
					? $t(
							check.config
								? 'ui.pages.integrationsPage.verifiedConfig'
								: 'ui.pages.integrationsPage.verified',
							{
								values: { status: check.status }
							}
						)
					: $t('ui.pages.integrationsPage.verifyFailed', { values: { detail: check.message } })
			};
			if (check.ok) void loadModels();
		} catch (error) {
			verifyRead = {
				ok: false,
				copy: $t('ui.pages.integrationsPage.verifyFailed', { values: { detail: message(error) } })
			};
		} finally {
			verifying = false;
		}
	}

	// A refusal is reported in the section itself: the operator asked to see one
	// value, and the reason it is not there belongs beside the hint.
	async function toggleKey() {
		if (!detail) return;
		if (keyVisible) {
			keyVisible = false;
			return;
		}
		if (keyToken) {
			keyVisible = true;
			return;
		}
		keyLoading = true;
		keyFailure = '';
		try {
			keyToken = (await api<{ token: string }>(integrationPath(detail.id, 'token'))).token;
			keyVisible = true;
		} catch (error) {
			keyFailure = message(error);
		} finally {
			keyLoading = false;
		}
	}

	async function copyKey() {
		try {
			await navigator.clipboard.writeText(keyToken);
			keyCopied = true;
		} catch {
			keyCopied = false;
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

	// A refused key is reported beside the picker rather than raised: the operator
	// is looking at the wiring either way.
	async function loadModels() {
		if (!detail || modelsState === 'loading') return;
		modelsState = 'loading';
		modelsMessage = '';
		try {
			const read = await api<IntegrationModels>(integrationPath(detail.id, 'models'));
			models = read.models ?? [];
			modelsMessage = read.message;
			modelsState = read.ok ? 'ready' : 'failed';
			if (!models.some((model) => model.id === testModel)) testModel = models[0]?.id ?? '';
		} catch (error) {
			models = [];
			modelsMessage = message(error);
			modelsState = 'failed';
		}
	}

	async function sendTest() {
		if (!detail || testing || !testModel || testPrompt.trim() === '') return;
		testing = true;
		testAnswer = null;
		try {
			testAnswer = await api<IntegrationChat>(integrationPath(detail.id, 'chat'), {
				method: 'POST',
				body: JSON.stringify({ model: testModel, prompt: testPrompt.trim() })
			});
		} catch (error) {
			testAnswer = {
				ok: false,
				status: 0,
				model: testModel,
				text: '',
				error: message(error),
				duration_ms: 0
			};
		} finally {
			testing = false;
		}
	}

	// A destructive step is never stacked on the surface that asked for it.
	function ask(action: ConfirmAction, agent: IntegrationView) {
		detail = agent;
		confirmFailure = '';
		pendingConfirm = action;
		detailOpen = false;
	}

	// Runs after the modal's closing transition finishes, so the two overlays are
	// never on screen together.
	function openPendingConfirm() {
		if (pendingConfirm === null) return;
		confirming = pendingConfirm;
		pendingConfirm = null;
		confirmOpen = true;
	}

	async function confirm() {
		if (!detail || !confirming) return;
		const agent = detail;
		const action = confirming;
		confirmBusy = true;
		// Rotation reports in the modal with the key it minted, so the confirmation
		// closes first and the modal returns underneath it rather than the operator
		// being dropped on the grid. Never two overlays at once.
		if (action === 'rotate') {
			confirmOpen = false;
			confirming = null;
			confirmBusy = false;
			detailOpen = true;
			await run('rotate', agent);
			return;
		}
		try {
			if (action === 'overwrite') {
				const answer = await api<IntegrationResult>(integrationPath(agent.id, 'setup'), {
					method: 'POST',
					body: JSON.stringify({ overwrite: true })
				});
				await patch(answer);
				openDetail(answer.integration);
				reportResult(answer, 'setup');
			} else {
				// The operator's "remove" is the daemon's "disable": Relo takes its
				// configuration back out, restores what it snapshotted, and retires the
				// key it minted.
				await api(integrationPath(agent.id, action), { method: 'POST' });
				agents = patchAgent(agents, {
					...agent,
					enabled: false,
					state: 'off',
					key: undefined,
					files: []
				});
				agentAttention.set(agents);
				detail = agents.find((entry) => entry.id === agent.id) ?? null;
				resetKey();
				resetTest();
				void load(true);
				toast.success(
					$t(
						action === 'remove'
							? 'ui.pages.integrationsPage.removed'
							: 'ui.pages.integrationsPage.restored'
					)
				);
			}
		} catch (error) {
			// The confirmation itself reports the failure, so the operator is not left
			// wondering whether anything happened.
			confirmFailure = message(error);
		} finally {
			confirmBusy = false;
			confirmOpen = false;
			confirming = null;
		}
	}

	const overwritten = $derived(confirming === 'remove' && detail ? overwrittenFiles(detail) : []);

	const confirmTitle = $derived(
		confirming === 'remove'
			? $t('ui.pages.integrationsPage.removeTitle', {
					values: { label: detail ? labelOf(detail) : '' }
				})
			: confirming === 'restore'
				? $t('ui.pages.integrationsPage.restoreTitle')
				: confirming === 'overwrite'
					? $t('ui.pages.integrationsPage.overwriteTitle')
					: $t('ui.pages.integrationsPage.rotateTitle')
	);

	const confirmBody = $derived(
		confirming === 'remove'
			? overwritten.length > 0
				? $t('ui.pages.integrationsPage.removeBodyOverwrite', {
						values: { count: overwritten.length, files: overwritten.join(', ') }
					})
				: $t('ui.pages.integrationsPage.removeBody')
			: confirming === 'restore'
				? $t('ui.pages.integrationsPage.restoreBody')
				: confirming === 'overwrite'
					? $t('ui.pages.integrationsPage.overwriteBody')
					: $t('ui.pages.integrationsPage.rotateBody')
	);

	// primary is the modal's one primary for the view step: what the operator most
	// likely came to do about this agent right now. A drifted agent is offered its
	// repair, a healthy one its verification, an unwired one its set-up.
	const primary = $derived.by<{ key: Work; icon: IconName; act: () => void } | null>(() => {
		const agent = detail;
		if (!agent) return null;
		if (!agent.enabled) {
			return { key: 'setup', icon: 'plus', act: () => startSetup(agent) };
		}
		if (needsRepair(agent)) {
			return { key: 'repair', icon: 'wrench', act: () => void run('repair', agent) };
		}
		return { key: 'verify', icon: 'circle-check', act: () => void verifyNow() };
	});

	function startSetup(agent: IntegrationView) {
		if (refusalOf(agent) === 'foreign_key') {
			ask('overwrite', agent);
			return;
		}
		void run('setup', agent);
	}
</script>

<svelte:head><title>{$t('ui.pages.integrationsPage.title')} · Relo</title></svelte:head>

<PageHeader
	title="ui.pages.integrationsPage.title"
	description="ui.pages.integrationsPage.subtitle"
>
	{#snippet actions()}
		<Button size="sm" onclick={() => (addOpen = true)}>
			<Icon name="plus" size={14} />
			{$t('ui.pages.integrationsPage.addTitle')}
		</Button>
	{/snippet}
</PageHeader>

<div class="page-frame page-stack pt-6">
	{#if failure}
		<p class="text-sm text-danger" role="alert">{failure}</p>
	{/if}

	{#if loading}
		<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
			{#each [0, 1, 2, 3] as placeholder (placeholder)}
				<div class="section-surface flat-surface h-56 animate-pulse bg-sunken"></div>
			{/each}
		</div>
	{:else}
		<div class="hidden justify-end sm:flex">
			<CardLayoutToggle />
		</div>
		{#if cardFailure}
			<div
				class="flex items-start gap-2 rounded-xl border border-danger/30 bg-danger/5 px-4 py-3 text-sm text-danger"
				role="alert"
			>
				<Icon name="circle-x" size={15} class="mt-0.5 shrink-0" />
				<span class="min-w-0 flex-1">{cardFailure.text}</span>
				<IconAction icon="x" label={$t('ui.common.dismiss')} onclick={() => (cardFailure = null)} />
			</div>
		{/if}
		{#snippet agentGrid(roster: IntegrationView[])}
			<div class={cardLayoutGrid(consoleState.settings.cardLayout)}>
				{#each roster as agent (agent.id)}
					<AgentCard
						{agent}
						{running}
						onopen={() => openDetail(agent)}
						onsetup={() => startSetup(agent)}
						onrestart={() => void run('restart', agent)}
						onrepair={() => void run('repair', agent)}
						onremove={() => ask('remove', agent)}
						ontogglecontext={() => void toggleContext(agent)}
					/>
				{/each}
			</div>
		{/snippet}
		{#if configured.length === 0}
			<div
				class="empty-panel rounded-card border-[1.5px] border-dashed border-accent-border bg-accent-soft"
			>
				<p class="section-heading text-ink">{$t('ui.pages.integrationsPage.empty')}</p>
				<Button size="sm" onclick={() => (addOpen = true)}>
					<Icon name="plus" size={14} />
					{$t('ui.pages.integrationsPage.addTitle')}
				</Button>
			</div>
		{:else}
			{@render agentGrid(configured)}
		{/if}
	{/if}
</div>

<!-- One modal for one agent. Its body and footer swap per step: the wiring and the
     test pane, the result of an action, and the one-time key an action minted. -->
<CenteredModal
	bind:open={detailOpen}
	onOpenChange={(value) => {
		if (!value) closeDetail();
	}}
	onClosed={openPendingConfirm}
	size="wide"
	title={detail ? labelOf(detail) : ''}
	description={detail?.summary ?? ''}
>
	{#if detail}
		{#key step}
			{#if step === 'view'}
				{#if detail.last_error}
					<p class="mb-4 flex items-start gap-2 text-sm text-danger" role="alert">
						<Icon name="alert-triangle" size={15} class="mt-0.5 shrink-0" />
						{detail.last_error}
					</p>
				{/if}
				<!-- Two sides: what Relo wired, and what the key actually does. The
				     columns stack below the dock breakpoint with the wiring first. -->
				<div class="grid gap-6 lg:grid-cols-2">
					<div class="flex min-w-0 flex-col gap-6">
						<AgentKeySection
							agent={detail}
							visible={keyVisible}
							token={keyToken}
							loading={keyLoading}
							copied={keyCopied}
							failure={keyFailure}
							busy={blocked(running, detail)}
							ontoggle={() => void toggleKey()}
							oncopy={() => void copyKey()}
						/>
						<AgentWiringSection agent={detail} />
					</div>
					<AgentTestPane
						agent={detail}
						{models}
						state={modelsState}
						message={modelsMessage}
						query={modelQuery}
						provider={modelProvider}
						selected={testModel}
						prompt={testPrompt}
						{testing}
						answer={testAnswer}
						verify={verifying || verifyRead
							? { running: verifying, ok: verifyRead?.ok ?? true, copy: verifyRead?.copy ?? '' }
							: null}
						onquery={(value) => (modelQuery = value)}
						onprovider={(value) => (modelProvider = value)}
						onselect={(value) => (testModel = value)}
						onprompt={(value) => (testPrompt = value)}
						onsend={() => void sendTest()}
					/>
				</div>
			{:else if result}
				<AgentResultStep
					agent={detail}
					state={result.state}
					copy={result.copy}
					token={result.token}
					{urls}
					copied={revealCopied}
					oncopy={() => void copyReveal()}
				/>
			{/if}
		{/key}
	{/if}

	{#snippet footer()}
		{#if step !== 'view'}
			<!-- A result step keeps one way out, and Done returns to the wiring
			     rather than closing the surface the work happened on. -->
			<div class="modal-footer justify-end">
				<Button
					variant={result?.state === 'failed' ? 'outline' : 'ghost'}
					onclick={() => (step = 'view')}
				>
					<Icon name="check" size={14} />
					{$t('ui.common.done')}
				</Button>
			</div>
		{:else if detail && primary}
			<!-- Footer summary and actions share one row: what this modal is
			     about and how it stands on the left, the action group on the
			     right. The row wraps on narrow sheets while the action group
			     keeps its own stacking. -->
			<div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
				<div class="flex min-w-0 flex-wrap items-center gap-2">
					<span class="min-w-0 truncate text-sm font-semibold">{labelOf(detail)}</span>
					<StatusBadge kind={stateTone(detail)} label={$t(stateKey(detail))} />
					<StatusBadge
						kind="muted"
						label={$t(
							isTested(detail)
								? 'ui.pages.integrationsPage.tested'
								: 'ui.pages.integrationsPage.notTestedYet'
						)}
					/>
				</div>
				<div class="modal-footer max-sm:w-full flex-wrap">
					<!-- The destructive action leads the footer group: named, not a bare
					     icon, and apart from the primary rather than beside it. -->
					{#if detail.enabled}
						<Button
							variant="destructive"
							disabled={blocked(running, detail)}
							onclick={() => detail && ask('remove', detail)}
						>
							<Icon name="trash" size={14} />
							{$t('ui.pages.integrationsPage.remove')}
						</Button>
					{/if}
					{#if detail.enabled && detail.id === 'codex'}
						<IconAction
							icon="gauge"
							variant="outline"
							pressed={contextOn(detail)}
							tone={contextOn(detail) ? 'accent' : 'default'}
							label={$t(
								contextOn(detail)
									? 'ui.pages.integrationsPage.context1MTurnOff'
									: 'ui.pages.integrationsPage.context1MTurnOn'
							)}
							disabled={blocked(running, detail)}
							onclick={() => detail && toggleContext(detail)}
						/>
					{/if}
					{#if detail.enabled}
						<Button
							variant="outline"
							disabled={blocked(running, detail)}
							onclick={() => detail && ask('rotate', detail)}
						>
							<Icon name="refresh" size={14} />
							{$t('ui.pages.integrationsPage.rotate')}
						</Button>
					{/if}
					{#if detail.manages_files && detail.files.length > 0}
						<Button
							variant="outline"
							disabled={blocked(running, detail)}
							onclick={() => detail && ask('restore', detail)}
						>
							<Icon name="arrow-back-up" size={14} />
							{$t('ui.pages.integrationsPage.restore')}
						</Button>
					{/if}
					<Button
						disabled={primary.key === 'setup' && setupBlocked(refusalOf(detail))}
						onclick={primary.act}
					>
						{#if runningOn(running, detail, primary.key)}
							<Icon name="loader" size={14} spin />
						{:else}
							<Icon name={primary.icon} size={14} />
						{/if}
						{$t('ui.pages.integrationsPage.' + primary.key)}
					</Button>
				</div>
			</div>
		{/if}
	{/snippet}
</CenteredModal>

<AgentConfirm
	bind:open={confirmOpen}
	title={confirmTitle}
	body={confirmBody}
	error={confirmFailure}
	confirmLabel={confirming === 'remove'
		? $t('ui.pages.integrationsPage.remove')
		: $t('ui.common.confirm')}
	tone={confirming === 'remove' ? 'destructive' : 'default'}
	busy={confirmBusy}
	icon={confirming === 'remove' ? 'trash' : 'alert-triangle'}
	onconfirm={confirm}
	onClose={() => (confirming = null)}
/>

<AddAgentModal bind:open={addOpen} {agents} {urls} onadded={() => void load()} />
