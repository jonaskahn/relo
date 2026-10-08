<script lang="ts">
	import { t } from 'svelte-i18n';

	import {
		isChatFormat,
		type ChatMode,
		type ChatSettings,
		type ChatTarget
	} from '$lib/chat-threads';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import NativeSelect from '$lib/components/ui/native-select.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import type { Group, Model, Provider } from '$lib/types';

	// ChatSettingsPanel is the target a test points at, one press away from the
	// conversation rather than over the top of it. Every choice applies the
	// moment it is made on a conversation that has not sent anything; a
	// conversation that has keeps the target it was tested with, so its fields
	// are frozen and the way out is a new chat.
	interface Props {
		settings: ChatSettings;
		providers: Provider[];
		models: Model[];
		routes: Group[];
		loading: boolean;
		locked: boolean;
		closeIcon: IconName;
		closeLabel: string;
		onclose: () => void;
		onchange: (next: ChatSettings) => void;
	}

	let {
		settings,
		providers,
		models,
		routes,
		loading,
		locked,
		closeIcon,
		closeLabel,
		onclose,
		onchange
	}: Props = $props();

	let closeButton = $state<HTMLButtonElement | null>(null);
	let targetGroup = $state<HTMLDivElement | null>(null);

	// A format test may follow a route; a direct test always names a model.
	const usesRoute = $derived(settings.mode === 'format' && settings.target === 'route');
	const enabledModels = $derived(models.filter((model) => model.enabled));

	const modeOptions: { value: ChatMode; label: string }[] = $derived([
		{ value: 'direct', label: $t('ui.pages.chatPage.modeDirect') },
		{ value: 'format', label: $t('ui.pages.chatPage.modeFormat') }
	]);
	const targetOptions: { value: ChatTarget; label: string }[] = $derived([
		{ value: 'model', label: $t('ui.pages.chatPage.targetModel') },
		{ value: 'route', label: $t('ui.pages.chatPage.targetRoute') }
	]);
	// The reply the tester asks for: frame by frame, which is what a client that
	// streams reads, or as one body, which is what it reads otherwise.
	const streamOptions = $derived([
		{ value: 'stream', label: $t('ui.pages.chatPage.streamOn') },
		{ value: 'complete', label: $t('ui.pages.chatPage.streamOff') }
	]);

	export function focusClose() {
		closeButton?.focus();
	}

	// focusTarget puts the caret on the choice the header named: the model a
	// direct test calls, or the route a format test follows. The model comes
	// after the connection it belongs to, so the last select is the one meant.
	// A pane that is still reading its catalog, a locked conversation, or a
	// model list with nothing enabled has no choice to put a caret in, so the
	// control that closes the pane takes it instead.
	export function focusTarget() {
		const selects = targetGroup?.querySelectorAll<HTMLSelectElement>('select');
		const field =
			selects === undefined || selects.length === 0 ? undefined : selects[selects.length - 1];
		if (field === undefined || field.disabled) {
			closeButton?.focus();
			return;
		}
		field.focus();
	}

	function change(patch: Partial<ChatSettings>) {
		onchange({ ...settings, ...patch });
	}
</script>

<div class="flex h-full min-h-0 flex-col">
	<div class="flex shrink-0 items-center gap-1 px-4 pt-4">
		<h2 class="min-w-0 flex-1 truncate text-[15px] font-semibold text-ink">
			{$t('ui.pages.chatPage.testSettings')}
		</h2>
		<Button
			bind:ref={closeButton}
			variant="ghost"
			size="icon"
			aria-label={closeLabel}
			title={closeLabel}
			onclick={onclose}
		>
			<Icon name={closeIcon} size={16} />
		</Button>
	</div>

	<div class="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 pt-4 pb-4">
		{#if loading}
			<p class="text-xs text-muted-foreground">{$t('ui.common.loading')}</p>
		{:else if providers.length === 0}
			<div class="space-y-3">
				<p class="text-xs leading-relaxed text-muted-foreground">
					{$t('ui.pages.chatPage.noProviders')}
				</p>
				<a
					class="inline-flex items-center gap-1.5 text-sm font-medium text-accent-deep underline-offset-4 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
					href="/connections"
				>
					<Icon name="plug-connected" size={14} />
					{$t('ui.sidebar.connections')}
				</a>
			</div>
		{:else}
			{#if locked}
				<p
					class="flex items-start gap-2 rounded-panel border border-line bg-surface px-3 py-2.5 text-xs leading-relaxed text-muted-foreground"
				>
					<Icon name="lock" size={13} class="mt-0.5 shrink-0" />
					{$t('ui.pages.chatPage.lockedHint')}
				</p>
			{/if}

			<Field id="chat-mode" label={$t('ui.pages.chatPage.mode')}>
				<Segmented
					class="w-full [&>button]:flex-1 [&>button]:justify-center"
					value={settings.mode}
					options={modeOptions}
					ariaLabel={$t('ui.pages.chatPage.mode')}
					disabled={locked}
					onchange={(mode) => change({ mode })}
				/>
			</Field>

			{#if settings.mode === 'format'}
				<Field id="chat-target" label={$t('ui.pages.chatPage.target')}>
					<Segmented
						class="w-full [&>button]:flex-1 [&>button]:justify-center"
						value={settings.target}
						options={targetOptions}
						ariaLabel={$t('ui.pages.chatPage.target')}
						disabled={locked}
						onchange={(target) => change({ target })}
					/>
				</Field>
				<Field id="chat-format" label={$t('ui.pages.chatPage.format')}>
					<NativeSelect
						id="chat-format"
						value={settings.format}
						disabled={locked}
						onchange={(event) => {
							const { value } = event.currentTarget;
							if (isChatFormat(value)) change({ format: value });
						}}
					>
						<option value="openai-chat">{$t('ui.pages.chatPage.formatChat')}</option>
						<option value="openai-responses">{$t('ui.pages.chatPage.formatResponses')}</option>
						<option value="anthropic">{$t('ui.pages.chatPage.formatMessages')}</option>
					</NativeSelect>
				</Field>
			{/if}

			<div class="space-y-4" bind:this={targetGroup}>
				{#if usesRoute}
					<Field id="chat-route" label={$t('ui.pages.chatPage.route')}>
						<NativeSelect
							id="chat-route"
							value={settings.routeId}
							disabled={locked || routes.length === 0}
							onchange={(event) => change({ routeId: event.currentTarget.value })}
						>
							{#if routes.length === 0}
								<option value="">{$t('ui.pages.chatPage.noRoutes')}</option>
							{/if}
							{#each routes as route (route.id)}
								<option value={route.id}>{route.label || route.id}</option>
							{/each}
						</NativeSelect>
					</Field>
				{:else}
					<Field id="chat-provider" label={$t('ui.pages.chatPage.provider')}>
						<NativeSelect
							id="chat-provider"
							value={settings.providerId}
							disabled={locked}
							onchange={(event) => change({ providerId: event.currentTarget.value })}
						>
							{#each providers as provider (provider.id)}
								<option value={provider.id}>{provider.label}</option>
							{/each}
						</NativeSelect>
					</Field>
					<Field id="chat-model" label={$t('ui.pages.chatPage.model')}>
						<NativeSelect
							id="chat-model"
							value={settings.modelId}
							disabled={locked || enabledModels.length === 0}
							onchange={(event) => change({ modelId: event.currentTarget.value })}
						>
							{#if enabledModels.length === 0}
								<option value="">{$t('ui.pages.chatPage.noModels')}</option>
							{/if}
							{#each enabledModels as model (model.model_id)}
								<option value={model.model_id}>{model.name || model.model_id}</option>
							{/each}
						</NativeSelect>
					</Field>
				{/if}
			</div>

			<Field id="chat-system" label={$t('ui.pages.chatPage.system')} optional>
				<textarea
					id="chat-system"
					rows="5"
					disabled={locked}
					class="min-h-24 w-full resize-y rounded-control border border-line bg-surface px-3 py-2 text-sm text-ink outline-none disabled:cursor-not-allowed disabled:opacity-60"
					value={settings.system}
					oninput={(event) => change({ system: event.currentTarget.value })}></textarea>
			</Field>

			<Field id="chat-max-tokens" label={$t('ui.pages.chatPage.maxTokens')}>
				<Input
					id="chat-max-tokens"
					type="number"
					min="1"
					step="64"
					disabled={locked}
					value={settings.maxTokens}
					oninput={(event) => change({ maxTokens: Number(event.currentTarget.value) || 0 })}
				/>
			</Field>

			<Field id="chat-stream" label={$t('ui.pages.chatPage.stream')}>
				<Segmented
					class="w-full [&>button]:flex-1 [&>button]:justify-center"
					value={settings.stream ? 'stream' : 'complete'}
					options={streamOptions}
					ariaLabel={$t('ui.pages.chatPage.stream')}
					disabled={locked}
					onchange={(choice) => change({ stream: choice === 'stream' })}
				/>
			</Field>
		{/if}
	</div>
</div>
