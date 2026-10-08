<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { ProviderTemplate } from '$lib/types';
	import type {
		ConnectKind,
		CredentialDraft,
		Deployment,
		StepperState
	} from '$lib/provider-stepper';
	import { declaresNoList } from '$lib/provider-stepper';
	import Input from '$lib/components/ui/input.svelte';
	import SecretInput from '$lib/components/ui/secret-input.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import DeploymentsEditor from './deployments-editor.svelte';

	interface Props {
		kind: ConnectKind;
		template: ProviderTemplate | null;
		draft: StepperState;
		problems: Record<string, string>;
		onvariables: (variables: Record<string, string>) => void;
		oncredential: (credential: CredentialDraft) => void;
		ondeployments: (deployments: Deployment[]) => void;
	}

	let { kind, template, draft, problems, onvariables, oncredential, ondeployments }: Props =
		$props();

	function patch(change: Partial<CredentialDraft>) {
		oncredential({ ...draft.credential, ...change });
	}

	// A connection that publishes no model list of its own gets every id from
	// the operator, so the flow asks for them here. Adding a key to a provider
	// that already exists keeps the roster it holds.
	const manualModels = $derived(
		kind !== 'azure' && draft.path !== 'addKey' && declaresNoList(draft, template)
	);

	function setVariable(name: string, value: string) {
		onvariables({ ...draft.variables, [name]: value });
	}

	async function readFile(event: Event) {
		if (!(event.currentTarget instanceof HTMLInputElement)) return;
		const file = event.currentTarget.files?.[0];
		if (!file) return;
		patch({ serviceAccount: await file.text() });
	}
</script>

<div class="flex flex-col gap-3">
	{#each template?.variables ?? [] as variable (variable.name)}
		<Field
			id={'connect-var-' + variable.name}
			label={variable.label || variable.name}
			error={problems['variables.' + variable.name] ?? ''}
		>
			{#if (variable.options ?? []).length > 0}
				<NativeSelect
					id={'connect-var-' + variable.name}
					value={draft.variables[variable.name] ?? ''}
					onchange={(event) => setVariable(variable.name, event.currentTarget.value)}
				>
					{#each variable.options ?? [] as option (option)}
						<option value={option}>{option}</option>
					{/each}
				</NativeSelect>
			{:else}
				<Input
					id={'connect-var-' + variable.name}
					placeholder={variable.placeholder}
					value={draft.variables[variable.name] ?? ''}
					oninput={(event) => setVariable(variable.name, event.currentTarget.value)}
				/>
			{/if}
		</Field>
	{/each}

	{#if kind === 'aws' || kind === 'gcp'}
		<div class="flex flex-col gap-1">
			<span class="text-sm">{$t('ui.pages.providersPage.add.credentialKind')}</span>
			<div class="flex flex-wrap gap-2">
				{#if kind === 'aws'}
					<Button
						type="button"
						variant="chip"
						size="sm"
						aria-pressed={!draft.credential.useAccessKeys}
						onclick={() => patch({ useAccessKeys: false })}
					>
						{$t('ui.pages.providersPage.add.useBedrockKey')}
					</Button>
					<Button
						type="button"
						variant="chip"
						size="sm"
						aria-pressed={draft.credential.useAccessKeys}
						onclick={() => patch({ useAccessKeys: true })}
					>
						{$t('ui.pages.providersPage.add.useAccessKeys')}
					</Button>
				{:else}
					<Button
						type="button"
						variant="chip"
						size="sm"
						aria-pressed={!draft.credential.useServiceAccount}
						onclick={() => patch({ useServiceAccount: false })}
					>
						{$t('ui.pages.providersPage.add.useVertexKey')}
					</Button>
					<Button
						type="button"
						variant="chip"
						size="sm"
						aria-pressed={draft.credential.useServiceAccount}
						onclick={() => patch({ useServiceAccount: true })}
					>
						{$t('ui.pages.providersPage.add.useServiceAccount')}
					</Button>
				{/if}
			</div>
		</div>
	{/if}

	{#if kind === 'aws' && draft.credential.useAccessKeys}
		<div class="field-grid">
			<Field
				id="connect-access-key-id"
				label={$t('ui.pages.providersPage.add.accessKeyId')}
				error={problems['credential.accessKeyId'] ?? ''}
			>
				<Input
					id="connect-access-key-id"
					class="font-mono text-xs"
					value={draft.credential.accessKeyId}
					oninput={(event) => patch({ accessKeyId: event.currentTarget.value })}
				/>
			</Field>
			<Field
				id="connect-secret-access-key"
				label={$t('ui.pages.providersPage.add.secretAccessKey')}
				error={problems['credential.secretAccessKey'] ?? ''}
			>
				<SecretInput
					id="connect-secret-access-key"
					class="font-mono text-xs"
					value={draft.credential.secretAccessKey}
					oninput={(event) => patch({ secretAccessKey: event.currentTarget.value })}
				/>
			</Field>
			<Field
				id="connect-session-token"
				label={$t('ui.pages.providersPage.add.sessionToken')}
				hint={$t('ui.pages.providersPage.add.sessionTokenHint')}
			>
				<SecretInput
					id="connect-session-token"
					class="font-mono text-xs"
					value={draft.credential.sessionToken}
					oninput={(event) => patch({ sessionToken: event.currentTarget.value })}
				/>
			</Field>
		</div>
	{:else if kind === 'gcp' && draft.credential.useServiceAccount}
		<Field
			id="connect-service-account"
			label={$t('ui.pages.providersPage.add.serviceAccount')}
			error={problems['credential.serviceAccount'] ?? ''}
		>
			<textarea
				id="connect-service-account"
				class="min-h-32 rounded-md border border-border bg-transparent p-2 font-mono text-xs"
				placeholder={$t('ui.pages.providersPage.add.serviceAccountPaste')}
				value={draft.credential.serviceAccount}
				oninput={(event) => patch({ serviceAccount: event.currentTarget.value })}></textarea>
			<label
				class="inline-flex h-control w-fit cursor-pointer items-center gap-1.5 rounded-md border border-border px-3 text-xs font-medium hover:border-border-strong"
			>
				<Icon name="external-link" size={14} />
				{$t('ui.pages.providersPage.add.serviceAccountFile')}
				<input type="file" accept="application/json,.json" class="sr-only" onchange={readFile} />
			</label>
		</Field>
	{:else if kind === 'azure'}
		<Field
			id="connect-api-key"
			label={$t('ui.pages.providersPage.add.apiKey')}
			error={problems['credential.secret'] ?? ''}
		>
			<SecretInput
				id="connect-api-key"
				class="font-mono text-xs"
				value={draft.credential.secret}
				oninput={(event) => patch({ secret: event.currentTarget.value })}
			/>
		</Field>
		<DeploymentsEditor
			deployments={draft.deployments}
			problem={problems.deployments ?? ''}
			onchange={ondeployments}
		/>
	{:else if kind !== 'signin' && kind !== 'none'}
		<Field
			id="connect-api-key"
			label={$t('ui.pages.providersPage.add.apiKey')}
			error={problems['credential.secret'] ?? ''}
		>
			<SecretInput
				id="connect-api-key"
				class="font-mono text-xs"
				value={draft.credential.secret}
				oninput={(event) => patch({ secret: event.currentTarget.value })}
			/>
		</Field>
	{/if}

	{#if manualModels}
		<DeploymentsEditor
			scope="model"
			deployments={draft.deployments}
			problem={problems.deployments ?? ''}
			onchange={ondeployments}
		/>
	{/if}
</div>
