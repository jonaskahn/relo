<script lang="ts">
	import { t } from 'svelte-i18n';
	import { Tooltip } from 'bits-ui';

	import Icon from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';
	import { refusalCode } from '$lib/integration-actions';
	import type { IntegrationFile, IntegrationPlan, IntegrationView } from '$lib/types';

	// What Relo wrote, or what an operator writes by hand. One agent has exactly
	// one of the two, and before a set-up it has neither: it has the plan Relo
	// would write, so the operator sees the change before making it.
	interface Props {
		agent: IntegrationView;
	}

	let { agent }: Props = $props();

	// A missing file and a drifted one are both facts about the machine, never
	// presented as healthy.
	function fileState(file: IntegrationFile): {
		key: string;
		tone: string;
		icon: 'check' | 'alert-triangle' | 'circle-x';
	} {
		if (!file.present) {
			return { key: 'fileMissing', tone: 'text-danger', icon: 'circle-x' };
		}
		if (file.drifted) {
			return { key: 'fileDrifted', tone: 'text-warn', icon: 'alert-triangle' };
		}
		return { key: 'filePresent', tone: 'text-muted-foreground', icon: 'check' };
	}

	// A code the daemon names wins over its free-text reason, so the sentence is
	// the one the daemon localizes.
	function planLabel(plan: IntegrationPlan): string {
		const detail = refusalCode([plan]);
		if (!detail) return plan.reason ?? '';
		switch (detail) {
			case 'not_installed':
				return $t('ui.pages.integrationsPage.notInstalledDetail');
			case 'relative_path':
				return $t('ui.pages.integrationsPage.relativePath');
			case 'foreign_key':
				return $t('ui.pages.integrationsPage.foreignKey');
			default:
				return $t('ui.pages.integrationsPage.unrecognised');
		}
	}

	// steps are what an operator does by hand for a client Relo does not
	// configure. A client that publishes none falls back to the two facts that
	// decide whether it works: the address and the key.
	const steps = $derived.by(() => {
		const listed = (agent.steps ?? []).filter((step) => step !== '');
		if (listed.length > 0) return listed;
		if (!agent.base_url && !agent.key) return [];
		return [`${agent.base_url ?? '—'}/v1`, agent.key?.token_hint ?? '—'];
	});

	const preview = $derived(agent.preview ?? []);
</script>

{#if agent.manages_files}
	<div class="space-y-2">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.writesTitle')}</h3>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.writesHint')}</p>
		{#if agent.files.length === 0}
			<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.noFilesYet')}</p>
		{:else}
			<ul class="flex flex-col gap-1.5">
				{#each agent.files as file (file.kind)}
					{@const state = fileState(file)}
					<li
						class="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border px-2.5 py-1.5"
					>
						<Tooltip.Provider delayDuration={250} disableHoverableContent>
							<Tooltip.Root>
								<Tooltip.Trigger>
									{#snippet child({ props: { type: _type, tabindex: _tabindex, ...rest } })}
										<span {...rest} class="mono-data min-w-0 truncate text-xs">{file.path}</span>
									{/snippet}
								</Tooltip.Trigger>
								<Tooltip.Portal>
									<Tooltip.Content
										class="z-50 surface-pop max-w-sm break-all px-2.5 py-1.5 text-xs"
									>
										{file.path}
									</Tooltip.Content>
								</Tooltip.Portal>
							</Tooltip.Root>
						</Tooltip.Provider>
						<span class={cn('flex shrink-0 items-center gap-1 text-xs', state.tone)}>
							<Icon name={state.icon} size={12} class="shrink-0" />
							{$t('ui.pages.integrationsPage.' + state.key)}
						</span>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{:else}
	<div class="space-y-2">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.setupSteps')}</h3>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.stepsHint')}</p>
		{#if steps.length > 0}
			<ol class="list-decimal space-y-1 pl-5 text-xs text-muted-foreground">
				{#each steps as step (step)}
					<li class="mono-data whitespace-pre-wrap break-words">{step}</li>
				{/each}
			</ol>
		{:else}
			<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.stepsUnknown')}</p>
		{/if}
	</div>
{/if}

<!-- The plan Relo would write, shown before anything changes so the operator can
     read the edit and the reason a write would be refused. -->
{#if !agent.enabled && preview.length > 0}
	<div class="space-y-2">
		<h3 class="text-sm font-semibold">{$t('ui.pages.integrationsPage.previewTitle')}</h3>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.integrationsPage.previewHint')}</p>
		<ul class="flex flex-col gap-2">
			{#each preview as plan (plan.kind)}
				<li class="rounded-lg border border-border px-2.5 py-2">
					<div class="flex flex-wrap items-center justify-between gap-2">
						<span class="mono-data min-w-0 truncate text-xs">{plan.path}</span>
						{#if plan.refused}
							<span class="flex shrink-0 items-center gap-1 text-xs text-warn">
								<Icon name="alert-triangle" size={12} class="shrink-0" />
								{planLabel(plan)}
							</span>
						{/if}
					</div>
					{#if plan.fragment}
						<pre
							class="mono-data mt-1.5 overflow-x-auto text-xs text-muted-foreground">{plan.fragment}</pre>
					{/if}
				</li>
			{/each}
		</ul>
	</div>
{/if}
