<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import Icon from '$lib/components/ui/icon.svelte';
	import { api } from '$lib/api';
	import { followFormatDefaults } from '$lib/provider-stepper';
	import {
		CALL_WAIT_PRESETS,
		callWaitLabel,
		cloneWindows,
		DEFAULT_CALL_WAIT,
		normalizeRetryBackoff,
		retryBackoffLabel,
		retryBackoffValid
	} from '$lib/upstream-settings';
	import type { Provider, Settings, TemplateFormatOption, TemplateSettings } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import FieldSwitch from '$lib/components/ui/field-switch.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import SignInSettingsSection from './sign-in-settings-section.svelte';
	import FormatCards from './format-cards.svelte';
	import HeadersEditor from './headers-editor.svelte';
	import SettingsSection from './settings-section.svelte';
	import {
		baseURLMissing,
		connectionDirty,
		connectionDraft,
		rankInvalid,
		signInDirty,
		signInSection,
		templatePatch,
		type ConnectionDraft,
		type SignInDraft
	} from '$lib/settings-form';

	interface Props {
		provider: Provider;
		allFormats: TemplateFormatOption[];
		onchanged: (updated: Provider) => void;
	}

	let { provider, allFormats, onchanged }: Props = $props();

	// The three attempts in order, which is both the label each row shows and
	// the key that names it.
	const RETRY_LABEL_KEYS = [
		'ui.settingsPage.retryFirst',
		'ui.settingsPage.retrySecond',
		'ui.settingsPage.retryThird'
	];
	const strategies = ['least-loaded', 'round-robin', 'random'] as const;

	// The form holds a draft of the connection, seeded from the connection
	// itself. A re-fetch hands the pane a new row, and the key the pane mounts
	// this tab under throws that draft away with it — which is what tells the
	// form a re-fetch landed and what was being typed should be discarded.
	let draft = $state(untrack(() => connectionDraft(provider)));
	let globalSettings = $state<Settings | null>(null);
	let saving = $state(false);
	let notice = $state('');
	let failed = $state('');
	let saveAttempted = $state(false);
	let rankTouched = $state(false);
	let baseTouched = $state(false);
	let signInSaved = $state<SignInDraft | null>(null);
	let signInDraft = $state<SignInDraft>({ autoRefresh: true });
	let signInState = $state<'loading' | 'ready' | 'error'>('loading');
	let loadSeq = 0;

	const signIn = $derived(signInSection(provider.template_id));
	const formats = $derived(allFormats.length > 0 ? allFormats : fallbackFormats());
	const selected = $derived(formats.find((option) => option.format === draft.apiFormat) ?? null);
	const connectionChanged = $derived(connectionDirty(draft, provider));
	const signInChanged = $derived(signInSaved !== null && signInDirty(signInDraft, signInSaved));
	const dirty = $derived(connectionChanged || signInChanged);
	const rankError = $derived((rankTouched || saveAttempted) && rankInvalid(draft.rank));
	const baseError = $derived((baseTouched || saveAttempted) && baseURLMissing(draft.baseURL));

	$effect(() => {
		const id = provider.id;
		if (signInSection(provider.template_id) === null) return;
		void loadSignIn(id);
	});

	const proxyDescription = $derived(
		draft.useProxy && (globalSettings?.providers.proxy_url ?? '') === ''
			? $t('ui.pages.providersPage.settings.useProxyEmpty')
			: $t('ui.pages.providersPage.settings.useProxyHint')
	);
	const globalCallWait = $derived(globalSettings?.providers.timeout_seconds ?? DEFAULT_CALL_WAIT);
	const globalRetryBackoff = $derived(
		normalizeRetryBackoff(globalSettings?.providers.retry_backoff)
	);
	const retryError = $derived(
		saveAttempted && draft.retryBackoff !== null && !retryBackoffValid(draft.retryBackoff)
	);

	onMount(() => {
		let alive = true;
		void api<Settings>('/settings')
			.then((data) => {
				if (alive) globalSettings = data;
			})
			.catch(() => {
				if (alive) globalSettings = null;
			});
		return () => {
			alive = false;
		};
	});

	function fallbackFormats(): TemplateFormatOption[] {
		return [
			{
				format: provider.api_format,
				default_base_url: provider.base_url,
				key_header: provider.key_header,
				models_format: provider.models_format,
				label: provider.api_format
			}
		];
	}

	function strategyName(strategy: string): string {
		if (strategy === 'least-loaded')
			return $t('ui.pages.providersPage.settings.strategyLeastLoaded');
		if (strategy === 'round-robin') return $t('ui.pages.providersPage.settings.strategyRoundRobin');
		return $t('ui.pages.providersPage.settings.strategyRandom');
	}

	function chooseFormat(option: TemplateFormatOption) {
		const next = followFormatDefaults(selected, option, {
			baseURL: draft.baseURL,
			keyHeader: draft.keyHeader
		});
		draft.apiFormat = option.format;
		draft.baseURL = next.baseURL;
		draft.keyHeader = next.keyHeader;
	}

	function reset() {
		draft = connectionDraft(provider);
		if (signInSaved) signInDraft = { ...signInSaved };
		rankTouched = false;
		baseTouched = false;
		saveAttempted = false;
		notice = '';
		failed = '';
	}

	async function loadSignIn(id: string) {
		const seq = ++loadSeq;
		signInState = 'loading';
		try {
			const body = await api<TemplateSettings>(
				'/connections/' + encodeURIComponent(id) + '/template-settings'
			);
			if (seq !== loadSeq) return;
			signInSaved = { autoRefresh: body.auto_refresh };
			signInDraft = { ...signInSaved };
			signInState = 'ready';
		} catch {
			if (seq !== loadSeq) return;
			signInState = 'error';
		}
	}

	async function save() {
		saveAttempted = true;
		if (
			rankInvalid(draft.rank) ||
			baseURLMissing(draft.baseURL) ||
			(draft.retryBackoff !== null && !retryBackoffValid(draft.retryBackoff))
		)
			return;
		const connection = connectionChanged;
		const patch = signInSaved ? templatePatch(signInDraft, signInSaved) : null;
		if (!connection && patch === null) return;
		saving = true;
		notice = '';
		failed = '';
		try {
			if (connection) {
				const updated = await api<Provider>('/connections/' + encodeURIComponent(provider.id), {
					method: 'PATCH',
					body: JSON.stringify(connectionBody(draft))
				});
				onchanged(updated);
			}
			if (patch !== null) {
				const saved = await api<TemplateSettings>(
					'/connections/' + encodeURIComponent(provider.id) + '/template-settings',
					{ method: 'PATCH', body: JSON.stringify(patch) }
				);
				signInSaved = { autoRefresh: saved.auto_refresh };
				signInDraft = { ...signInSaved };
			}
			notice = $t('ui.pages.providersPage.settings.saved');
			saveAttempted = false;
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		} finally {
			saving = false;
		}
	}

	function connectionBody(current: ConnectionDraft) {
		return {
			label: current.label,
			enabled: current.enabled,
			rank: Number(current.rank),
			pool_strategy: current.poolStrategy,
			use_proxy: current.useProxy,
			timeout_seconds: current.timeoutSeconds,
			retry_backoff: current.retryBackoff,
			switch_on_4xx: current.switchOn4xx,
			switch_on_5xx: current.switchOn5xx,
			api_format: current.apiFormat,
			base_url: current.baseURL,
			variables: current.variables,
			headers: current.headers,
			key_header: current.keyHeader
		};
	}
</script>

<div class="provider-settings flex flex-col py-5">
	<div class="settings-rows">
		<SettingsSection
			section="general"
			title={$t('ui.pages.providersPage.settings.general')}
			description={$t('ui.pages.providersPage.settings.generalDescription')}
		>
			<div class="flex flex-col gap-4">
				<Field id="settings-label" label={$t('ui.pages.providersPage.settings.label')}>
					<Input id="settings-label" class="max-sm:h-11" bind:value={draft.label} />
				</Field>
				<FieldSwitch
					id="settings-enabled"
					bind:checked={draft.enabled}
					label={$t('ui.pages.providersPage.settings.enabled')}
				/>
			</div>
		</SettingsSection>

		<SettingsSection
			section="routing"
			title={$t('ui.pages.providersPage.settings.routing')}
			description={$t('ui.pages.providersPage.settings.routingDescription')}
		>
			<div class="flex flex-col gap-4">
				<Field
					id="settings-rank"
					label={$t('ui.pages.providersPage.settings.rank')}
					hint={$t('ui.pages.providersPage.settings.rankHint')}
					error={rankError ? $t('ui.pages.providersPage.settings.rankInvalid') : ''}
				>
					<Input
						id="settings-rank"
						class="max-sm:h-11"
						inputmode="numeric"
						bind:value={draft.rank}
						aria-invalid={rankError ? true : undefined}
						aria-describedby={rankError ? 'settings-rank-error' : 'settings-rank-hint'}
						onblur={() => (rankTouched = true)}
					/>
				</Field>
				<FieldSwitch
					id="settings-use-proxy"
					bind:checked={draft.useProxy}
					label={$t('ui.pages.providersPage.settings.useProxy')}
					description={proxyDescription}
				/>
				<Field id="settings-strategy" label={$t('ui.pages.providersPage.settings.poolStrategy')}>
					<NativeSelect id="settings-strategy" class="max-sm:h-11" bind:value={draft.poolStrategy}>
						{#each strategies as strategy (strategy)}
							<option value={strategy}>{strategyName(strategy)}</option>
						{/each}
					</NativeSelect>
				</Field>
				<FieldSwitch
					id="settings-switch-on-4xx"
					bind:checked={draft.switchOn4xx}
					label={$t('ui.pages.providersPage.settings.switchOn4xx')}
					description={$t('ui.pages.providersPage.settings.switchOn4xxHint')}
				/>
				<FieldSwitch
					id="settings-switch-on-5xx"
					bind:checked={draft.switchOn5xx}
					label={$t('ui.pages.providersPage.settings.switchOn5xx')}
					description={$t('ui.pages.providersPage.settings.switchOn5xxHint')}
				/>
			</div>
		</SettingsSection>

		<SettingsSection
			section="upstream"
			title={$t('ui.pages.providersPage.settings.waitTitle')}
			description={$t('ui.pages.providersPage.settings.waitDescription')}
		>
			<div class="flex flex-col gap-5">
				<div class="flex flex-col gap-2">
					<span class="text-sm">{$t('ui.pages.providersPage.settings.callWait')}</span>
					<div class="flex flex-wrap gap-2">
						<Button
							type="button"
							variant="chip"
							size="sm"
							aria-pressed={draft.timeoutSeconds === null}
							onclick={() => (draft.timeoutSeconds = null)}
						>
							{$t('ui.pages.providersPage.settings.callWaitUseGlobal', {
								values: { value: callWaitLabel(globalCallWait) }
							})}
						</Button>
						{#each CALL_WAIT_PRESETS as seconds (seconds)}
							<Button
								type="button"
								variant="chip"
								size="sm"
								aria-pressed={draft.timeoutSeconds === seconds}
								onclick={() => (draft.timeoutSeconds = seconds)}
							>
								{callWaitLabel(seconds)}
							</Button>
						{/each}
					</div>
					<p class="text-sm text-muted-foreground">
						{$t('ui.pages.providersPage.settings.callWaitHint')}
					</p>
				</div>

				<div class="flex flex-col gap-2">
					<span class="text-sm">{$t('ui.pages.providersPage.settings.retryWaits')}</span>
					<div class="flex flex-wrap gap-2">
						<Button
							type="button"
							variant="chip"
							size="sm"
							aria-pressed={draft.retryBackoff === null}
							onclick={() => (draft.retryBackoff = null)}
						>
							{$t('ui.pages.providersPage.settings.retryUseGlobal', {
								values: { value: retryBackoffLabel(globalRetryBackoff) }
							})}
						</Button>
						<Button
							type="button"
							variant="chip"
							size="sm"
							aria-pressed={draft.retryBackoff !== null}
							onclick={() => {
								if (draft.retryBackoff === null)
									draft.retryBackoff = cloneWindows(globalRetryBackoff);
							}}
						>
							{$t('ui.pages.providersPage.settings.retryCustom')}
						</Button>
					</div>
					{#if draft.retryBackoff}
						<div class="flex flex-col gap-2">
							{#each draft.retryBackoff as window, index (RETRY_LABEL_KEYS[index])}
								{@const label = RETRY_LABEL_KEYS[index]}
								<div class="flex flex-wrap items-center gap-2">
									<span class="w-full text-sm text-muted-foreground sm:w-28">{$t(label)}</span>
									<Input
										type="number"
										min="0"
										max="600"
										class="w-20 max-sm:h-11"
										bind:value={window[0]}
										aria-label={$t('ui.settingsPage.retryLow') + ' · ' + $t(label)}
									/>
									<span class="text-sm text-muted-foreground">–</span>
									<Input
										type="number"
										min="0"
										max="600"
										class="w-20 max-sm:h-11"
										bind:value={window[1]}
										aria-label={$t('ui.settingsPage.retryHigh') + ' · ' + $t(label)}
									/>
									<span class="text-sm text-muted-foreground">s</span>
								</div>
							{/each}
						</div>
						{#if retryError}
							<p class="text-sm text-destructive" role="alert">
								{$t('ui.pages.providersPage.settings.retryInvalid')}
							</p>
						{/if}
					{/if}
					<p class="text-sm text-muted-foreground">
						{$t('ui.pages.providersPage.settings.retryWaitsHint')}
					</p>
				</div>
			</div>
		</SettingsSection>

		<SettingsSection
			section="advanced"
			title={$t('ui.pages.providersPage.settings.advanced')}
			description={$t('ui.pages.providersPage.settings.advancedDescription')}
		>
			<div class="flex min-h-0 flex-col gap-4 pt-1">
				{#if formats.length > 1}
					<div class="flex flex-col gap-1">
						<span class="text-sm">{$t('ui.pages.providersPage.settings.apiFormat')}</span>
						<FormatCards options={formats} value={draft.apiFormat} onchange={chooseFormat} />
					</div>
				{/if}
				<Field
					id="settings-base-url"
					label={$t('ui.pages.providersPage.settings.baseURL')}
					error={baseError ? $t('ui.pages.providersPage.settings.baseURLRequired') : ''}
				>
					<div class="flex items-center gap-2">
						<Input
							id="settings-base-url"
							class="mono-data max-sm:h-11"
							bind:value={draft.baseURL}
							aria-invalid={baseError ? true : undefined}
							aria-describedby={baseError ? 'settings-base-url-error' : undefined}
							onblur={() => (baseTouched = true)}
						/>
						{#if selected && draft.baseURL !== selected.default_base_url}
							<Button
								variant="outline"
								size="sm"
								class="max-sm:min-h-11"
								onclick={() => (draft.baseURL = selected?.default_base_url ?? draft.baseURL)}
							>
								<Icon name="refresh" size={14} />
								{$t('ui.pages.providersPage.settings.baseURLReset')}
							</Button>
						{/if}
					</div>
				</Field>
				{#if (provider.variable_defs ?? []).length > 0}
					<div class="flex flex-col gap-2">
						<span class="text-sm">{$t('ui.pages.providersPage.settings.variables')}</span>
						{#each provider.variable_defs ?? [] as variable (variable.name)}
							<Field id={'settings-var-' + variable.name} label={variable.label || variable.name}>
								{#if (variable.options ?? []).length > 0}
									<NativeSelect
										id={'settings-var-' + variable.name}
										class="max-sm:h-11"
										bind:value={draft.variables[variable.name]}
									>
										{#each variable.options ?? [] as option (option)}
											<option value={option}>{option}</option>
										{/each}
									</NativeSelect>
								{:else}
									<Input
										id={'settings-var-' + variable.name}
										class="max-sm:h-11"
										placeholder={variable.placeholder}
										bind:value={draft.variables[variable.name]}
									/>
								{/if}
							</Field>
						{/each}
					</div>
				{/if}
				{#if provider.origin === 'custom'}
					<Field
						id="settings-key-header"
						label={$t('ui.pages.providersPage.settings.keyHeader')}
						hint={$t('ui.pages.providersPage.settings.keyHeaderHint')}
					>
						<NativeSelect id="settings-key-header" class="max-sm:h-11" bind:value={draft.keyHeader}>
							<option value="bearer">bearer</option>
							<option value="x-api-key">x-api-key</option>
							<option value="x-goog-api-key">x-goog-api-key</option>
							<option value="api-key">api-key</option>
							<option value="none">none</option>
						</NativeSelect>
					</Field>
				{/if}
				<div class="flex flex-col gap-1">
					<span class="text-sm">{$t('ui.pages.providersPage.settings.headers')}</span>
					<HeadersEditor headers={draft.headers} onchange={(next) => (draft.headers = next)} />
				</div>
			</div>
		</SettingsSection>

		{#if signIn}
			<SettingsSection
				section="signin"
				title={$t(signIn.title)}
				description={$t(signIn.description)}
			>
				<SignInSettingsSection
					status={signInState}
					autoRefresh={signInDraft.autoRefresh}
					disabled={saving}
					onretry={() => void loadSignIn(provider.id)}
					onchange={(next) => (signInDraft = next)}
				/>
			</SettingsSection>
		{/if}
	</div>

	{#if failed}
		<p class="mt-3 text-sm text-destructive" role="alert">
			{$t('ui.pages.providersPage.settings.saveFailed', { values: { detail: failed } })}
		</p>
	{/if}
	{#if notice}
		<p class="mt-3 text-sm text-muted-foreground" role="status">{notice}</p>
	{/if}

	{#if dirty}
		<div class="settings-savebar" data-savebar>
			<p class="text-sm">{$t('ui.pages.providersPage.settings.unsaved')}</p>
			<div class="flex items-center gap-2">
				<Button variant="ghost" size="sm" class="max-sm:min-h-11" onclick={reset}>
					<Icon name="refresh" size={14} />
					{$t('ui.pages.providersPage.settings.reset')}
				</Button>
				<Button size="sm" class="max-sm:min-h-11" disabled={saving} onclick={save}>
					<Icon name={saving ? 'loader' : 'check'} size={14} spin={saving} />
					{$t(
						saving
							? 'ui.pages.providersPage.settings.saving'
							: 'ui.pages.providersPage.settings.save'
					)}
				</Button>
			</div>
		</div>
	{/if}
</div>
