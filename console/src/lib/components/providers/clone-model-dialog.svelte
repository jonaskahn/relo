<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';

	import { api } from '$lib/api';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Field from '$lib/components/ui/field.svelte';
	import type { Model } from '$lib/types';
	import { contextLabel, parseContextInput } from '$lib/context-window';
	import { clonePayload, formatPrice, suggestedCloneId, validateCloneId } from '$lib/model-clone';

	// CloneModelDialog copies one model under a new identifier. Relo sends the
	// upstream id to the provider, so a clone reaches a model the provider has
	// not listed yet.
	interface Props {
		open?: boolean;
		onOpenChange?: (open: boolean) => void;
		source: Model;
		providerId: string;
		existingIds?: string[];
		oncloned: (model: Model) => void;
	}

	let {
		open = $bindable(false),
		onOpenChange,
		source,
		providerId,
		existingIds = [],
		oncloned
	}: Props = $props();

	// The dialog is mounted for the one row it copies, so it starts from that
	// row: a second clone never inherits the first attempt.
	let newId = $state(untrack(() => suggestedCloneId(source.model_id, existingIds)));
	let upstreamId = $state(untrack(() => source.upstream_model_id || source.model_id));
	let name = $state(untrack(() => source.name || source.model_id));
	let contextWindow = $state(
		untrack(() => (source.context_window ? String(source.context_window) : ''))
	);
	let maxOutput = $state(untrack(() => (source.max_output ? String(source.max_output) : '')));
	let inputPrice = $state(untrack(() => formatPrice(source.prices?.effective?.input ?? null)));
	let outputPrice = $state(untrack(() => formatPrice(source.prices?.effective?.output ?? null)));
	let busy = $state(false);
	let failure = $state('');

	const idProblem = $derived(validateCloneId(newId, existingIds));
	const contextProblem = $derived(
		contextWindow.trim() === '' ? null : parseContextInput(contextWindow)
	);

	async function submit() {
		if (idProblem !== null) return;
		if (contextProblem && !contextProblem.ok) return;
		busy = true;
		failure = '';
		try {
			const payload = clonePayload(
				{ upstreamId, name, contextWindow, maxOutput, inputPrice, outputPrice },
				{
					upstreamId: source.upstream_model_id || source.model_id,
					name: source.name || source.model_id,
					contextWindow: source.context_window,
					maxOutput: source.max_output,
					inputPrice: source.prices?.effective?.input ?? null,
					outputPrice: source.prices?.effective?.output ?? null
				}
			);
			const created = await api<Model>(
				'/connections/' + encodeURIComponent(providerId) + '/models/clone',
				{
					method: 'POST',
					body: JSON.stringify({
						source_model_id: source.model_id,
						model_id: newId.trim(),
						...payload
					})
				}
			);
			open = false;
			oncloned(created);
		} catch (error) {
			failure = error instanceof Error ? error.message : String(error);
		} finally {
			busy = false;
		}
	}
</script>

<CenteredModal
	bind:open
	{onOpenChange}
	title={$t('ui.pages.providersPage.clone.title')}
	description={$t('ui.pages.providersPage.clone.subtitle', {
		values: { id: source.model_id, context: contextLabel(source.context_window) || '—' }
	})}
>
	<div class="flex flex-col gap-4">
		<Field
			id="clone-id"
			label={$t('ui.pages.providersPage.clone.newId')}
			error={idProblem ? $t('ui.pages.providersPage.clone.idError.' + idProblem) : ''}
		>
			<Input id="clone-id" bind:value={newId} autocomplete="off" spellcheck="false" />
		</Field>

		<Field
			id="clone-upstream"
			label={$t('ui.pages.providersPage.clone.upstreamId')}
			hint={$t('ui.pages.providersPage.clone.upstreamHint')}
		>
			<Input id="clone-upstream" bind:value={upstreamId} autocomplete="off" spellcheck="false" />
		</Field>

		<Field id="clone-name" label={$t('ui.pages.providersPage.clone.displayName')}>
			<Input id="clone-name" bind:value={name} />
		</Field>

		<div class="field-grid">
			<Field
				id="clone-context"
				label={$t('ui.pages.providersPage.clone.context')}
				error={contextProblem && !contextProblem.ok
					? $t('ui.pages.providersPage.context.error.' + contextProblem.reason)
					: ''}
			>
				<Input
					id="clone-context"
					bind:value={contextWindow}
					placeholder="200k"
					autocomplete="off"
				/>
			</Field>
			<Field id="clone-max-output" label={$t('ui.pages.providersPage.clone.maxOutput')}>
				<Input
					id="clone-max-output"
					bind:value={maxOutput}
					placeholder="64000"
					autocomplete="off"
				/>
			</Field>
			<Field id="clone-input" label={$t('ui.pages.providersPage.clone.inputPrice')}>
				<Input id="clone-input" bind:value={inputPrice} placeholder="0.28" autocomplete="off" />
			</Field>
			<Field id="clone-output" label={$t('ui.pages.providersPage.clone.outputPrice')}>
				<Input id="clone-output" bind:value={outputPrice} placeholder="0.42" autocomplete="off" />
			</Field>
		</div>

		{#if failure}
			<p class="flex items-center gap-1.5 text-sm text-destructive" role="alert">
				<Icon name="alert-triangle" size={14} />
				{failure}
			</p>
		{/if}
	</div>

	{#snippet footer()}
		<div class="flex justify-end gap-2">
			<Button variant="outline" onclick={() => (open = false)}>
				<Icon name="x" size={14} />
				{$t('ui.common.cancel')}
			</Button>
			<Button disabled={busy || idProblem !== null} onclick={submit}>
				{#if busy}
					<Icon name="loader" size={14} spin />
				{:else}
					<Icon name="copy-plus" size={14} />
				{/if}
				{$t('ui.pages.providersPage.clone.submit')}
			</Button>
		</div>
	{/snippet}
</CenteredModal>
