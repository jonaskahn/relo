<script lang="ts">
	import { t } from 'svelte-i18n';
	import { acceptLanguage, api, csrfToken } from '$lib/api';
	import { ApiError } from '$lib/api';
	import type { ModelsDevState, ProbeResult, Provider, ProviderTemplate } from '$lib/types';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import ConfirmDialog from '$lib/components/ui/confirm-dialog.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import WizardStepper from '$lib/components/ui/wizard-stepper.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import StepProvider from './step-provider.svelte';
	import StepConnect from './step-connect.svelte';
	import StepVerify from './step-verify.svelte';
	import StepReview from './step-review.svelte';
	import {
		connectKindOf,
		buildProbeBody,
		commitBody,
		defaultVariables,
		emptyStepperState,
		isDirty,
		presetStepperState,
		stepAfter,
		stepBefore,
		stepsFor,
		typedModels,
		validateConnect,
		type ConnectKind,
		type StepperState,
		type StepperStep
	} from '$lib/provider-stepper';
	import {
		isProviderId,
		suggestLabel,
		suggestProviderId,
		type TemplateRow
	} from '$lib/provider-sections';

	interface Props {
		open?: boolean;
		templates: ProviderTemplate[];
		// presetProviderId names a connection the operator is already inside, so
		// the flow opens on the connect step with that provider chosen.
		presetProviderId?: string;
		modelsdev?: ModelsDevState | null;
		providers: Provider[];
		onadded: (providerId: string) => void;
		// ondiscarded tells the page the flow closed without adding anything,
		// so the connection list resyncs and any connection a sign-in left
		// half-finished is not left on screen.
		ondiscarded: () => void;
		onrefreshdev: () => Promise<void>;
	}

	let {
		open = $bindable(false),
		templates,
		presetProviderId = '',
		modelsdev = null,
		providers,
		onadded,
		ondiscarded,
		onrefreshdev
	}: Props = $props();

	let step = $state<StepperStep>('provider');
	let draft = $state<StepperState>(emptyStepperState());
	let query = $state('');
	let selectedRow = $state<TemplateRow | null>(null);
	let probe = $state<ProbeResult | null>(null);
	let probing = $state(false);
	let probeFailure = $state('');
	let problems = $state<Record<string, string>>({});
	let signedInAs = $state('');
	let refreshNote = $state('');
	let reviewId = $state('');
	let reviewLabel = $state('');
	let disabled = $state<string[]>([]);
	let working = $state(false);
	let discardOpen = $state(false);
	// addCompleted marks a close that a save already answered for, and notified
	// keeps one close from telling the page twice.
	let addCompleted = $state(false);
	let notified = false;

	const template = $derived(
		draft.templateId === ''
			? null
			: (templates.find((entry) => entry.id === draft.templateId) ?? null)
	);
	const kind = $derived<ConnectKind>(draft.path === 'custom' ? 'custom' : connectKindOf(template));
	const stepLabels: Record<StepperStep, string> = {
		provider: 'ui.pages.providersPage.add.stepProvider',
		connect: 'ui.pages.providersPage.add.stepConnect',
		verify: 'ui.pages.providersPage.add.stepVerify',
		review: 'ui.pages.providersPage.add.stepReview'
	};
	const added = $derived(
		new Set(providers.flatMap((provider) => [provider.template_id, provider.id]).filter(Boolean))
	);
	const takenIds = $derived(providers.map((provider) => provider.id));
	const takenLabels = $derived(providers.map((provider) => provider.label));

	// The same template may back more than one provider, which is what the
	// "add a key" choice has to name exactly.
	const targets = $derived(
		providers.filter(
			(provider) => provider.template_id === draft.templateId || provider.id === draft.templateId
		)
	);
	const formats = $derived(template?.available_formats ?? []);

	function reset() {
		const preset = providers.find((provider) => provider.id === presetProviderId);
		const presetTemplate =
			preset === undefined
				? null
				: (templates.find((entry) => entry.id === (preset.template_id || preset.id)) ?? null);
		draft =
			preset === undefined
				? emptyStepperState()
				: presetStepperState(
						{
							id: preset.id,
							templateId: preset.template_id || preset.id,
							label: preset.label,
							baseURL: preset.base_url,
							variables: preset.variables ?? {}
						},
						presetTemplate
					);
		step = preset === undefined || presetTemplate === null ? 'provider' : 'connect';
		reviewId =
			preset === undefined ? '' : suggestProviderId(preset.template_id || preset.id, takenIds);
		reviewLabel = preset === undefined ? '' : suggestLabel(preset.label, takenLabels);
		query = '';
		selectedRow = null;
		probe = null;
		probing = false;
		probeFailure = '';
		problems = {};
		signedInAs = '';
		refreshNote = '';
		disabled = [];
		addCompleted = false;
		notified = false;
	}

	// Opening the flow starts it clean. A connection the operator is already
	// inside opens on its connect step; every other visit starts at the first.
	$effect(() => {
		if (open) reset();
	});

	// The footer's Next opens the form, so a misread row never advances on its
	// own.
	function select(row: TemplateRow) {
		selectedRow = row;
	}

	function confirmPick() {
		const row = selectedRow;
		if (row === null || row.disabledReason !== '') return;
		if (row.custom) return startCustom();
		const target = providers.filter(
			(provider) => provider.template_id === row.id || provider.id === row.id
		);
		const isAdded = target.length > 0;
		const signIn =
			(row.template?.login_methods ?? []).length > 0 || row.template?.kind === 'signin';
		// A provider that already exists keeps the settings it was added with, so
		// the connection fields open from what it holds rather than empty.
		const variables = { ...defaultVariables(row.template), ...(target[0]?.variables ?? {}) };
		draft = {
			...emptyStepperState(),
			path: signIn ? 'signin' : isAdded ? 'addKey' : 'new',
			templateId: row.id,
			targetProviderId: isAdded && !signIn ? target[0].id : '',
			label: row.label,
			loginFlow: row.template?.login_methods?.[0]?.flow ?? '',
			apiFormat: row.template?.default_format ?? '',
			baseURL: target[0]?.base_url || row.template?.default_base_url || '',
			variables
		};
		reviewId = suggestProviderId(row.id, takenIds);
		reviewLabel = suggestLabel(row.label, takenLabels);
		probe = null;
		problems = {};
		step = 'connect';
	}

	function startCustom() {
		draft = { ...emptyStepperState(), path: 'custom' };
		reviewId = '';
		reviewLabel = '';
		probe = null;
		problems = {};
		step = 'connect';
	}

	function patch(next: Partial<StepperState>) {
		draft = { ...draft, ...next };
		if (next.templateId !== undefined) {
			const chosen = templates.find((entry) => entry.id === next.templateId);
			draft.loginFlow = chosen?.login_methods?.[0]?.flow ?? '';
			draft.apiFormat = chosen?.default_format ?? '';
			draft.baseURL = chosen?.default_base_url ?? '';
			draft.variables = defaultVariables(chosen ?? null);
		}
	}

	async function advance() {
		problems = validateConnect(kind, draft, template);
		if (Object.keys(problems).length > 0) return;
		if (kind === 'signin') {
			step = 'connect';
			return;
		}
		await runProbe();
	}

	// A refused credential answers 422 and a failed listing 502, and both carry
	// the checklist that explains why, so the body is read either way.
	async function runProbe() {
		probing = true;
		probeFailure = '';
		step = 'verify';
		try {
			const response = await fetch('/api/v1/connections/probe', {
				method: 'POST',
				credentials: 'same-origin',
				headers: {
					'Content-Type': 'application/json',
					Accept: 'application/json',
					'Accept-Language': acceptLanguage(),
					'X-CSRF-Token': csrfToken()
				},
				body: JSON.stringify(buildProbeBody(draft, template, kind))
			});
			const payload: ProbeResult | null = await response.json().catch(() => null);
			if (payload) probe = payload;
			if (!response.ok) {
				if (payload && payload.checks.some((check) => check.status === 'fail')) {
					probe = payload;
				}
				probeFailure =
					response.status === 422
						? ''
						: $t('ui.pages.providersPage.verify.listingFailed', {
								values: {
									detail: payload?.checks.find((check) => check.name === 'listing')?.detail ?? ''
								}
							});
				return;
			}
			if (payload) {
				disabled = payload.models.filter((model) => !model.enabled).map((model) => model.id);
				if (draft.path !== 'addKey') {
					reviewId =
						reviewId.trim() || suggestProviderId(draft.templateId || draft.custom.id, takenIds);
					reviewLabel =
						reviewLabel.trim() || suggestLabel(draft.label || draft.custom.label, takenLabels);
				}
			}
		} catch (error) {
			probeFailure = error instanceof Error ? error.message : String(error);
		} finally {
			probing = false;
		}
	}

	// A sign-in finishes at Connect, so Verify reports the account and the
	// model list the refresh that follows it produced.
	async function signedIn(providerId: string, label: string) {
		signedInAs = label;
		working = true;
		refreshNote = '';
		try {
			const result = await api<{ listed: number; added: number }>(
				'/connections/' + encodeURIComponent(providerId) + '/models/refresh',
				{ method: 'POST' }
			);
			refreshNote = $t('ui.pages.providersPage.verify.listingLoaded', {
				values: { count: result.listed }
			});
		} catch (error) {
			refreshNote = $t('ui.pages.providersPage.verify.refreshFailed', {
				values: { detail: error instanceof Error ? error.message : String(error) }
			});
		} finally {
			working = false;
			step = 'verify';
		}
	}

	async function commit() {
		working = true;
		try {
			if (probe) {
				await api('/connections/probes/' + encodeURIComponent(probe.probe_id) + '/commit', {
					method: 'POST',
					body: JSON.stringify(
						commitBody(draft, {
							providerId: draft.path === 'addKey' ? draft.targetProviderId : reviewId,
							label: reviewLabel,
							disabled
						})
					)
				});
			}
			const providerId = probe?.target_provider_id || draft.targetProviderId || reviewId;
			addCompleted = true;
			open = false;
			reset();
			onadded(providerId);
		} catch (error) {
			probeFailure = $t('ui.pages.providersPage.review.commitFailed', {
				values: { detail: error instanceof Error ? error.message : String(error) }
			});
		} finally {
			working = false;
		}
	}

	function discard() {
		if (probe)
			void api('/connections/probes/' + encodeURIComponent(probe.probe_id), { method: 'DELETE' });
		discardOpen = false;
		open = false;
		reset();
		notifyClosed();
	}

	function notifyClosed() {
		if (notified) return;
		notified = true;
		ondiscarded();
	}

	function requestClose() {
		// A finished sign-in already stored its connection and its account, so
		// closing the flow discards nothing an operator would miss.
		if (addCompleted) {
			open = false;
			reset();
			return;
		}
		if (signedInAs === '' && isDirty(draft)) {
			discardOpen = true;
			open = true;
			return;
		}
		open = false;
		reset();
		notifyClosed();
	}

	// The login wrote the connection and the account, so the only thing left to
	// store is the model ids a connection that publishes no list of its own
	// takes by hand; then the operator is handed the connection the account
	// belongs to.
	async function finishSignIn() {
		const providerId = draft.targetProviderId || draft.templateId || reviewId;
		working = true;
		probeFailure = '';
		const refused: string[] = [];
		try {
			for (const model of typedModels(draft)) {
				try {
					await api('/connections/' + encodeURIComponent(providerId) + '/models', {
						method: 'POST',
						body: JSON.stringify(model)
					});
				} catch (error) {
					// A model the connection already has is not a failure to report.
					if (error instanceof ApiError && error.status === 409) continue;
					refused.push(
						$t('ui.pages.providersPage.verify.modelRefused', {
							values: {
								id: model.model_id,
								detail: error instanceof Error ? error.message : String(error)
							}
						})
					);
				}
			}
			if (refused.length > 0) {
				probeFailure = refused.join(' ');
				return;
			}
			addCompleted = true;
			open = false;
			reset();
			onadded(providerId);
		} finally {
			working = false;
		}
	}

	function back() {
		// A flow that opened inside one connection has no provider step to go
		// back to, so Back leaves it.
		if (step === 'connect' && presetProviderId !== '') {
			requestClose();
			return;
		}
		const previous = stepBefore(draft.path, step);
		if (previous === '') {
			requestClose();
			return;
		}
		step = previous;
	}

	function forward() {
		const next = stepAfter(draft.path, step);
		if (next !== '') step = next;
	}

	const idProblem = $derived(
		reviewId.trim() === ''
			? $t('ui.pages.providersPage.verify.fieldRequired')
			: !isProviderId(reviewId)
				? $t('ui.pages.providersPage.add.idInvalid')
				: ''
	);

	// A key path continues once its probe verified the connection, and a
	// sign-in once the login finished the account it stores.
	const canContinue = $derived(
		!probing && !working && (draft.path === 'signin' ? signedInAs !== '' : probe !== null)
	);
	const verifyLabel = $derived(
		draft.path === 'addKey'
			? 'ui.pages.providersPage.verify.addKey'
			: draft.path === 'signin'
				? 'ui.common.done'
				: 'ui.pages.providersPage.verify.continue'
	);
</script>

<CenteredModal
	bind:open
	size="xl"
	title={$t('ui.pages.providersPage.add.title')}
	description={$t('ui.pages.providersPage.add.subtitle')}
	class="h-[min(780px,100%)] max-sm:h-[92dvh]"
	chrome={{ header: 'px-7 pt-5 pb-3', footer: 'bg-background px-7 py-3.5' }}
	onOpenChange={(value: boolean) => {
		if (!value) requestClose();
	}}
>
	<div class="flex min-h-0 flex-1 flex-col gap-4">
		<WizardStepper
			steps={stepsFor(draft.path)}
			current={step}
			labelKeys={stepLabels}
			onjump={(target) => (step = target)}
			stepOfKey="ui.pages.providersPage.add.stepOf"
		/>

		{#if step === 'provider'}
			<StepProvider
				{templates}
				{modelsdev}
				{added}
				selected={selectedRow?.id ?? ''}
				{query}
				onquery={(value) => (query = value)}
				onpick={select}
				oncustom={startCustom}
				onretry={() => void onrefreshdev()}
			/>
		{:else if step === 'connect'}
			<StepConnect
				{kind}
				{template}
				{draft}
				{problems}
				methods={template?.login_methods ?? []}
				targetChoices={targets.map((provider) => ({ id: provider.id, label: provider.label }))}
				{formats}
				ondraft={patch}
				onsignedin={() => void signedIn(draft.targetProviderId || draft.templateId, reviewLabel)}
			/>
		{:else if step === 'verify'}
			<StepVerify
				running={probing || working}
				{probe}
				failure={probeFailure}
				{signedInAs}
				onretry={() => void runProbe()}
			/>
			{#if refreshNote}
				<p class="text-xs text-muted-foreground">{refreshNote}</p>
			{/if}
		{:else}
			<StepReview
				{probe}
				providerId={reviewId}
				label={reviewLabel}
				{disabled}
				{idProblem}
				isAddKey={draft.path === 'addKey'}
				targetLabel={template?.label ?? ''}
				templateId={template?.id ?? ''}
				onchange={(change) => {
					if (change.providerId !== undefined) reviewId = change.providerId;
					if (change.label !== undefined) reviewLabel = change.label;
					if (change.disabled !== undefined) disabled = change.disabled;
				}}
			/>
		{/if}
	</div>

	{#snippet footer()}
		<div class="flex flex-wrap items-center justify-between gap-3">
			<div class="flex min-w-0 items-center gap-2" role="status">
				{#if selectedRow}
					{#if selectedRow.custom}
						<span
							class="flex size-6 shrink-0 items-center justify-center rounded-lg border border-border text-muted-foreground"
						>
							<Icon name="settings" size={14} />
						</span>
					{:else}
						<ProviderLogo id={selectedRow.id} label={selectedRow.label} size="xs" />
					{/if}
					<span class="min-w-0 truncate text-sm font-semibold">{selectedRow.label}</span>
				{:else}
					<span class="truncate text-sm text-muted-foreground">
						{$t('ui.pages.providersPage.add.chooseHint')}
					</span>
				{/if}
				{#if step !== 'provider' && draft.apiFormat}
					<span
						class="hidden shrink-0 items-center rounded-full bg-accent-soft px-2.5 py-0.5 text-xs text-accent-ink sm:inline-flex"
					>
						{draft.apiFormat}
					</span>
				{/if}
				{#if probe}
					<span
						class="hidden shrink-0 items-center rounded-full bg-muted px-2.5 py-0.5 text-xs text-muted-foreground sm:inline-flex"
					>
						{$t('ui.pages.providersPage.list.modelCount', {
							values: { count: probe.models.length }
						})}
					</span>
				{/if}
			</div>
			<div class="flex shrink-0 items-center gap-2">
				<Button variant="ghost" onclick={back}>
					<Icon name={step === 'provider' ? 'x' : 'arrow-left'} size={14} />
					{#if step === 'provider'}
						{$t('ui.common.cancel')}
					{:else}
						{$t('ui.pages.providersPage.pane.back')}
					{/if}
				</Button>
				{#if step === 'provider'}
					<Button
						disabled={selectedRow === null || selectedRow.disabledReason !== ''}
						onclick={confirmPick}
					>
						<Icon name="chevron-right" size={14} />
						{$t('ui.pages.providersPage.add.next')}
					</Button>
				{:else if step === 'connect' && kind === 'signin'}
					<span class="text-xs text-muted-foreground">
						{$t('ui.pages.providersPage.add.signInHint')}
					</span>
				{:else if step === 'connect'}
					<Button disabled={probing} onclick={advance}>
						<Icon name={probing ? 'loader' : 'plug-connected'} size={14} spin={probing} />
						{$t('ui.pages.providersPage.add.verify')}
					</Button>
				{:else if step === 'verify'}
					<Button
						disabled={!canContinue}
						onclick={draft.path === 'addKey'
							? commit
							: draft.path === 'signin'
								? finishSignIn
								: forward}
					>
						<Icon name="check" size={14} />
						{$t(verifyLabel)}
					</Button>
				{:else if step === 'review'}
					<Button disabled={working || idProblem !== ''} onclick={commit}>
						<Icon name={working ? 'loader' : 'plus'} size={14} spin={working} />
						{$t('ui.pages.providersPage.review.addProvider')}
					</Button>
				{/if}
			</div>
		</div>
	{/snippet}
</CenteredModal>

<ConfirmDialog
	bind:open={discardOpen}
	title={$t('ui.pages.providersPage.add.discardTitle')}
	body={$t('ui.pages.providersPage.add.discardBody')}
	confirmLabel={$t('ui.pages.providersPage.add.discard')}
	tone="destructive"
	icon="alert-triangle"
	onconfirm={discard}
/>
