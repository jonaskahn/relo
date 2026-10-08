<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { LoginMethod, ProviderTemplate, TemplateFormatOption } from '$lib/types';
	import type {
		ConnectKind,
		CredentialDraft,
		Deployment,
		StepperState
	} from '$lib/provider-stepper';
	import Input from '$lib/components/ui/input.svelte';
	import SecretInput from '$lib/components/ui/secret-input.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import FormatCards from '../format-cards.svelte';
	import SignInPanel from '../sign-in-panel.svelte';
	import CredentialFields from './credential-fields.svelte';

	interface Props {
		kind: ConnectKind;
		template: ProviderTemplate | null;
		draft: StepperState;
		problems: Record<string, string>;
		methods?: LoginMethod[];
		targetChoices?: { id: string; label: string }[];
		formats?: TemplateFormatOption[];
		ondraft: (patch: Partial<StepperState>) => void;
		onsignedin: () => void;
	}

	let {
		kind,
		template,
		draft,
		problems,
		methods = [],
		targetChoices = [],
		formats = [],
		ondraft,
		onsignedin
	}: Props = $props();

	let advanced = $state(false);

	const addKey = $derived(draft.path === 'addKey');

	function setVariable(name: string, value: string) {
		ondraft({ variables: { ...draft.variables, [name]: value } });
	}
</script>

<div class="flex flex-col gap-4">
	<div class="flex flex-wrap items-center gap-2">
		{#if template}
			<ProviderLogo id={template.id} label={template.label} size="sm" />
			<span class="text-sm font-medium">{template.label}</span>
		{:else}
			<span class="text-sm font-medium">{$t('ui.pages.providersPage.add.customTitle')}</span>
		{/if}
		{#if kind === 'signin'}
			<span class="rounded border border-border px-1.5 py-0.5 text-[0.65rem] text-muted-foreground">
				{$t('ui.pages.providersPage.accounts.kindSignin')}
			</span>
		{/if}
	</div>

	{#if addKey && targetChoices.length > 0}
		<div class="flex flex-col gap-2 rounded-lg border border-border p-3">
			<label class="flex items-start gap-2 text-sm">
				<input
					type="radio"
					class="mt-1"
					name="target"
					checked={draft.targetProviderId !== ''}
					onchange={() => ondraft({ targetProviderId: targetChoices[0].id })}
				/>
				<span>
					<span class="block font-medium">
						{$t('ui.pages.providersPage.add.targetTitle', {
							values: { label: template?.label ?? '' }
						})}
					</span>
					<span class="block text-xs text-muted-foreground">
						{$t('ui.pages.providersPage.add.targetHint', {
							values: { label: template?.label ?? '' }
						})}
					</span>
				</span>
			</label>
			{#if targetChoices.length > 1}
				<select
					class="h-9 rounded-md border border-border bg-transparent px-2 text-sm"
					aria-label={$t('ui.pages.providersPage.add.targetProvider')}
					value={draft.targetProviderId}
					onchange={(event) => ondraft({ targetProviderId: event.currentTarget.value })}
				>
					{#each targetChoices as choice (choice.id)}
						<option value={choice.id}>{choice.label} · {choice.id}</option>
					{/each}
				</select>
			{/if}
			<label class="flex items-start gap-2 text-sm">
				<input
					type="radio"
					class="mt-1"
					name="target"
					checked={draft.targetProviderId === ''}
					onchange={() => ondraft({ targetProviderId: '' })}
				/>
				<span>
					<span class="block font-medium">{$t('ui.pages.providersPage.add.separateTitle')}</span>
					<span class="block text-xs text-muted-foreground">
						{$t('ui.pages.providersPage.add.separateHint')}
					</span>
				</span>
			</label>
		</div>
	{/if}

	{#if kind === 'signin'}
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.providersPage.add.signInTargetHint', {
				values: { label: template?.label ?? '' }
			})}
		</p>
		<SignInPanel {methods} label={draft.accountLabel} ondone={onsignedin} />
	{:else}
		{#if formats.length > 1 && !addKey}
			<div class="flex flex-col gap-1">
				<span class="text-sm">{$t('ui.pages.providersPage.add.apiFormat')}</span>
				<FormatCards
					options={formats}
					value={draft.apiFormat}
					onchange={(option) =>
						ondraft({ apiFormat: option.format, baseURL: option.default_base_url })}
				/>
			</div>
		{/if}

		{#if !addKey}
			<CredentialFields
				{kind}
				{template}
				{draft}
				{problems}
				onvariables={(variables) => ondraft({ variables })}
				oncredential={(credential) => ondraft({ credential })}
				ondeployments={(deployments) => ondraft({ deployments })}
			/>
		{:else}
			<CredentialFields
				{kind}
				{template}
				{draft}
				{problems}
				onvariables={() => {}}
				oncredential={(credential: CredentialDraft) => ondraft({ credential })}
				ondeployments={(deployments: Deployment[]) => ondraft({ deployments })}
			/>
		{/if}

		<Field
			id="connect-account-label"
			label={$t('ui.pages.providersPage.add.accountLabel')}
			hint={$t('ui.pages.providersPage.add.accountLabelHint')}
		>
			<Input
				id="connect-account-label"
				value={draft.accountLabel}
				oninput={(event) => ondraft({ accountLabel: event.currentTarget.value })}
			/>
		</Field>

		{#if !addKey}
			<details class="text-xs" bind:open={advanced}>
				<summary class="cursor-pointer text-muted-foreground">
					{$t('ui.pages.providersPage.add.advanced')}
				</summary>
				<div class="mt-2 flex flex-col gap-3">
					<Field id="connect-base-url" label={$t('ui.pages.providersPage.add.baseURL')}>
						<Input
							id="connect-base-url"
							class="font-mono text-xs"
							value={draft.baseURL}
							oninput={(event) => ondraft({ baseURL: event.currentTarget.value })}
						/>
					</Field>
					{#if kind !== 'custom'}
						<div class="flex flex-col gap-2">
							<span class="text-muted-foreground">{$t('ui.pages.providersPage.add.variables')}</span
							>
							{#each Object.keys(draft.variables) as name (name)}
								<div class="flex items-center gap-2">
									<span class="w-28 shrink-0 font-mono text-[0.7rem]">{name}</span>
									<Input
										class="font-mono text-xs"
										value={draft.variables[name]}
										oninput={(event) => setVariable(name, event.currentTarget.value)}
									/>
								</div>
							{/each}
						</div>
					{/if}
				</div>
			</details>
		{/if}
	{/if}

	{#if kind === 'custom'}
		<div class="field-grid">
			<Field
				id="custom-id"
				label={$t('ui.pages.providersPage.add.providerId')}
				error={problems['custom.id'] ?? ''}
			>
				<Input
					id="custom-id"
					class="font-mono text-xs"
					value={draft.custom.id}
					oninput={(event) =>
						ondraft({ custom: { ...draft.custom, id: event.currentTarget.value } })}
				/>
			</Field>
			<Field
				id="custom-label"
				label={$t('ui.pages.providersPage.add.label')}
				error={problems['custom.label'] ?? ''}
			>
				<Input
					id="custom-label"
					value={draft.custom.label}
					oninput={(event) =>
						ondraft({ custom: { ...draft.custom, label: event.currentTarget.value } })}
				/>
			</Field>
			<Field id="custom-models-format" label={$t('ui.pages.providersPage.add.modelsFormat')}>
				<NativeSelect
					id="custom-models-format"
					value={draft.custom.modelsFormat}
					onchange={(event) =>
						ondraft({ custom: { ...draft.custom, modelsFormat: event.currentTarget.value } })}
				>
					{#each ['openai', 'anthropic', 'gemini', 'bedrock', 'kiro', 'none'] as option (option)}
						<option value={option}>{option}</option>
					{/each}
				</NativeSelect>
			</Field>
			<Field id="custom-key-header" label={$t('ui.pages.providersPage.add.keyHeader')}>
				<NativeSelect
					id="custom-key-header"
					value={draft.custom.keyHeader}
					onchange={(event) =>
						ondraft({ custom: { ...draft.custom, keyHeader: event.currentTarget.value } })}
				>
					{#each ['bearer', 'x-api-key', 'x-goog-api-key', 'api-key', 'none'] as option (option)}
						<option value={option}>{option}</option>
					{/each}
				</NativeSelect>
			</Field>
			<Field
				id="custom-base-url"
				class="sm:col-span-2"
				label={$t('ui.pages.providersPage.add.baseURL')}
				error={problems['custom.baseURL'] ?? ''}
			>
				<Input
					id="custom-base-url"
					class="font-mono text-xs"
					placeholder="https://host/v1"
					value={draft.custom.baseURL}
					oninput={(event) =>
						ondraft({ custom: { ...draft.custom, baseURL: event.currentTarget.value } })}
				/>
			</Field>
			{#if draft.custom.keyHeader !== 'none'}
				<Field
					id="custom-secret"
					class="sm:col-span-2"
					label={$t('ui.pages.providersPage.add.apiKey')}
				>
					<SecretInput
						id="custom-secret"
						class="font-mono text-xs"
						value={draft.credential.secret}
						oninput={(event) =>
							ondraft({ credential: { ...draft.credential, secret: event.currentTarget.value } })}
					/>
				</Field>
			{/if}
		</div>
	{/if}

	{#if problems['loginFlow']}
		<p class="text-xs text-destructive" role="alert">{$t(problems['loginFlow'])}</p>
	{/if}
	{#if Object.keys(problems).length > 0}
		<p class="flex items-center gap-1.5 text-xs text-destructive" role="alert">
			<Icon name="alert-triangle" size={14} />
			{$t('ui.pages.providersPage.verify.fixFields')}
		</p>
	{/if}
	{#if addKey}
		<p class="text-xs text-muted-foreground">
			{$t('ui.pages.providersPage.add.targetHint', { values: { label: template?.label ?? '' } })}
		</p>
	{/if}
</div>
