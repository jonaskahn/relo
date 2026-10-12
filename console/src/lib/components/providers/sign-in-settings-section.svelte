<script lang="ts">
	import { t } from 'svelte-i18n';
	import Button from '$lib/components/ui/button.svelte';
	import FieldSwitch from '$lib/components/ui/field-switch.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import { formatPrefixes, parsePrefixes } from '$lib/settings-form';

	interface Props {
		// status is one value rather than a loading flag and a failed one: a
		// read is either in flight, answered, or refused.
		status: 'loading' | 'ready' | 'error';
		autoRefresh: boolean;
		modelPrefixes: string[];
		showPrefixes: boolean;
		disabled: boolean;
		onretry: () => void;
		onchange: (next: { autoRefresh: boolean; modelPrefixes: string[] }) => void;
	}

	let { status, autoRefresh, modelPrefixes, showPrefixes, disabled, onretry, onchange }: Props =
		$props();
</script>

<div class="flex flex-col gap-4" data-section="signin">
	{#if status === 'loading'}
		<p class="text-sm text-muted-foreground" role="status">
			{$t('ui.pages.providersPage.settings.signInLoading')}
		</p>
	{/if}
	<div class="signin-switches">
		<FieldSwitch
			id="settings-auto-refresh"
			checked={autoRefresh}
			disabled={disabled || status !== 'ready'}
			label={$t('ui.pages.providersPage.settings.autoRefreshLabel')}
			description={$t('ui.pages.providersPage.settings.autoRefreshHint')}
			onCheckedChange={(value) => onchange({ autoRefresh: value, modelPrefixes })}
		/>
	</div>
	{#if showPrefixes}
		<Field
			id="settings-model-prefixes"
			label={$t('ui.pages.providersPage.settings.modelPrefixesLabel')}
			hint={$t('ui.pages.providersPage.settings.modelPrefixesHint')}
		>
			<Input
				id="settings-model-prefixes"
				class="mono-data max-sm:h-11"
				value={formatPrefixes(modelPrefixes)}
				disabled={disabled || status !== 'ready'}
				oninput={(event) =>
					onchange({
						autoRefresh,
						modelPrefixes: parsePrefixes(event.currentTarget.value)
					})}
			/>
		</Field>
	{/if}
	{#if status === 'error'}
		<div class="flex items-center justify-between gap-3">
			<p class="text-sm text-destructive" role="alert">
				{$t('ui.pages.providersPage.settings.signInLoadFailed')}
			</p>
			<Button variant="outline" size="sm" onclick={onretry}>
				{$t('ui.pages.providersPage.settings.templateRetry')}
			</Button>
		</div>
	{/if}
</div>
