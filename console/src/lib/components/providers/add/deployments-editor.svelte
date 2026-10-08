<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Deployment } from '$lib/provider-stepper';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	interface Props {
		scope?: 'deployment' | 'model';
		deployments: Deployment[];
		problem?: string;
		onchange: (next: Deployment[]) => void;
	}

	let { scope = 'deployment', deployments, problem = '', onchange }: Props = $props();

	// The same editor types the deployment names Azure serves and the model ids
	// of a connection that publishes no list of its own, so its copy follows
	// what it holds.
	const KEYS = {
		deployment: {
			title: 'ui.pages.providersPage.add.deployments',
			hint: 'ui.pages.providersPage.add.deploymentsHint',
			name: 'ui.pages.providersPage.add.deploymentName',
			pricedAs: 'ui.pages.providersPage.add.deploymentPricedAs',
			add: 'ui.pages.providersPage.add.addDeployment',
			remove: 'ui.pages.providersPage.add.removeDeployment'
		},
		model: {
			title: 'ui.pages.providersPage.add.manualModels',
			hint: 'ui.pages.providersPage.add.manualModelsHint',
			name: 'ui.pages.providersPage.add.manualModelName',
			pricedAs: 'ui.pages.providersPage.add.manualModelPricedAs',
			add: 'ui.pages.providersPage.add.addManualModel',
			remove: 'ui.pages.providersPage.add.removeManualModel'
		}
	} as const;

	const keys = $derived(KEYS[scope]);

	let nextRowId = 0;
	const takeRowId = () => `deployment-${nextRowId++}`;

	function update(id: string, patch: Partial<Deployment>) {
		onchange(deployments.map((row) => (row.id === id ? { ...row, ...patch } : row)));
	}
</script>

<div class="flex flex-col gap-2">
	<span class="field-label">{$t(keys.title)}</span>
	<p class="field-hint">{$t(keys.hint)}</p>
	{#each deployments as row (row.id)}
		<div class="flex flex-wrap items-center gap-2">
			<Input
				class="min-w-40 flex-1 font-mono text-xs"
				aria-label={$t(keys.name)}
				placeholder={$t(keys.name)}
				value={row.modelId}
				oninput={(event) => update(row.id, { modelId: event.currentTarget.value })}
			/>
			<Input
				class="min-w-40 flex-1 font-mono text-xs"
				aria-label={$t(keys.pricedAs)}
				placeholder={$t(keys.pricedAs)}
				value={row.pricedAs}
				oninput={(event) => update(row.id, { pricedAs: event.currentTarget.value })}
			/>
			<Button
				variant="ghost"
				size="icon"
				aria-label={$t(keys.remove)}
				disabled={deployments.length <= 1}
				onclick={() => onchange(deployments.filter((other) => other.id !== row.id))}
			>
				<Icon name="trash" size={14} />
			</Button>
		</div>
	{/each}
	<Button
		variant="outline"
		size="sm"
		class="self-start"
		onclick={() => onchange([...deployments, { id: takeRowId(), modelId: '', pricedAs: '' }])}
	>
		<Icon name="plus" size={14} />
		{$t(keys.add)}
	</Button>
	{#if problem}
		<p class="text-xs text-destructive" role="alert">{problem}</p>
	{/if}
</div>
