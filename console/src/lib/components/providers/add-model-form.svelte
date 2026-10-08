<script lang="ts">
	import { t } from 'svelte-i18n';
	import { api } from '$lib/api';
	import type { Model } from '$lib/types';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Field from '$lib/components/ui/field.svelte';

	interface Props {
		providerId: string;
		onadded: (model: Model) => void;
		oncancel: () => void;
	}

	let { providerId, onadded, oncancel }: Props = $props();

	let modelId = $state('');
	let pricedAs = $state('');
	let working = $state(false);
	let failed = $state('');

	async function submit() {
		working = true;
		failed = '';
		try {
			const created = await api<Model>(
				'/connections/' + encodeURIComponent(providerId) + '/models',
				{
					method: 'POST',
					body: JSON.stringify({ model_id: modelId.trim(), priced_as: pricedAs.trim() })
				}
			);
			onadded(created);
		} catch (error) {
			failed = error instanceof Error ? error.message : String(error);
		} finally {
			working = false;
		}
	}
</script>

<form
	class="flex flex-wrap items-end gap-2 rounded-lg border border-border p-3"
	onsubmit={(event) => {
		event.preventDefault();
		void submit();
	}}
>
	<div class="field-grid w-full">
		<Field id="add-model-id" label={$t('ui.pages.providersPage.models.addModelId')}>
			<Input id="add-model-id" class="font-mono text-xs" bind:value={modelId} required />
		</Field>
		<Field id="add-model-priced-as" label={$t('ui.pages.providersPage.models.addModelPricedAs')}>
			<Input
				id="add-model-priced-as"
				class="font-mono text-xs"
				placeholder="openai/gpt-5"
				bind:value={pricedAs}
			/>
		</Field>
	</div>
	<div class="flex gap-2">
		<Button type="button" variant="ghost" size="sm" onclick={oncancel}>
			<Icon name="x" size={14} />
			{$t('ui.common.cancel')}
		</Button>
		<Button type="submit" size="sm" disabled={working || modelId.trim() === ''}>
			<Icon name={working ? 'loader' : 'plus'} size={14} spin={working} />
			{$t('ui.pages.providersPage.models.addModelSubmit')}
		</Button>
	</div>
	{#if failed}
		<p class="w-full text-xs text-destructive">
			{$t('ui.pages.providersPage.models.addModelFailed', { values: { detail: failed } })}
		</p>
	{/if}
</form>
