<script lang="ts">
	import { t } from 'svelte-i18n';
	import { Tooltip } from 'bits-ui';

	import Card from '$lib/components/ui/card.svelte';
	import CardContent from '$lib/components/ui/card-content.svelte';
	import CardDescription from '$lib/components/ui/card-description.svelte';
	import CardHeader from '$lib/components/ui/card-header.svelte';
	import CardTitle from '$lib/components/ui/card-title.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import { cn } from '$lib/utils';
	import { isCardOpenClick } from '$lib/card-target';
	import { cardLayoutCard } from '$lib/theme.svelte';
	import { consoleState } from '$lib/console-state.svelte';
	import {
		contextOn,
		isTested,
		labelOf,
		logoIdOf,
		needsRepair,
		restartsClient,
		stateKey,
		stateTone,
		warningKeys
	} from '$lib/agent-state';
	import { blocked, runningOn, type Running } from '$lib/agent-flow';
	import type { IntegrationView } from '$lib/types';

	// One agent card. The whole card is the hit area and the name button is the
	// accessible target, so it carries the Open label; Setup, Manage and the
	// action buttons keep their own jobs, which is what the shared click guard
	// decides. Every action reports whether it is the one in flight for this
	// agent, so one agent's work never freezes the page.
	interface Props {
		agent: IntegrationView;
		running: Running;
		onopen: () => void;
		onsetup: () => void;
		onrestart: () => void;
		onrepair: () => void;
		onremove: () => void;
		ontogglecontext: () => void;
	}

	let { agent, running, onopen, onsetup, onrestart, onrepair, onremove, ontogglecontext }: Props =
		$props();

	// The sentences behind the warnings the card reports, so it renders one
	// vocabulary of facts rather than a branch per condition.
	const warnings = $derived(warningKeys(agent).map((key) => $t(key)));
</script>

<!-- The summary reads as one line whatever its length, and the tooltip carries
     the whole sentence for a pointer. Its trigger is the description itself
     rather than a control, because the line sits inside the header's
     click-to-open button and a second button inside it would be invalid. -->
{#snippet summaryLine()}
	<Tooltip.Provider delayDuration={250} disableHoverableContent>
		<Tooltip.Root>
			<Tooltip.Trigger>
				{#snippet child({ props: { type: _type, tabindex: _tabindex, ...rest } })}
					<CardDescription {...rest} class="mt-1 block truncate text-xs">
						{agent.summary}
					</CardDescription>
				{/snippet}
			</Tooltip.Trigger>
			<Tooltip.Portal>
				<Tooltip.Content class="z-50 surface-pop max-w-sm break-words px-2.5 py-1.5 text-xs">
					{agent.summary}
				</Tooltip.Content>
			</Tooltip.Portal>
		</Tooltip.Root>
	</Tooltip.Provider>
{/snippet}

<Card
	class={cn(
		'section-surface card-press cursor-pointer gap-4 py-5 min-w-0',
		cardLayoutCard(consoleState.settings.cardLayout)
	)}
	onclick={(event: MouseEvent) => {
		if (isCardOpenClick(event)) onopen();
	}}
>
	<CardHeader class="shrink-0 gap-3">
		<div class="flex items-start justify-between gap-3">
			<button
				type="button"
				data-plain
				class="flex min-w-0 flex-1 flex-col items-start gap-3 text-left"
				aria-label={$t('ui.common.openCard', { values: { name: labelOf(agent) } })}
				onclick={() => onopen()}
			>
				<ProviderLogo id={logoIdOf(agent)} label={labelOf(agent)} />
				<div class="w-full min-w-0">
					<CardTitle class="truncate text-sm">{labelOf(agent)}</CardTitle>
					{@render summaryLine()}
				</div>
			</button>
			<!-- The coverage tags share the head's right column with the status
			     pill, so a long translated label wraps the row instead of
			     colliding with the title. -->
			<div class="flex shrink-0 flex-col items-end gap-1.5">
				<div class="flex flex-wrap items-center justify-end gap-1.5">
					{#if agent.id === 'codex' && contextOn(agent)}
						<span class="badge border-accent-line bg-accent-soft text-accent-ink">1M</span>
					{/if}
					<span
						class={cn(
							'badge',
							// Coverage is a fact about Relo, so the untested clients read as a
							// neutral tag rather than a warning.
							isTested(agent)
								? 'border-accent-line bg-accent-soft text-accent-ink'
								: 'border-line bg-sunken text-muted-foreground'
						)}
					>
						{$t(
							isTested(agent)
								? 'ui.pages.integrationsPage.tested'
								: 'ui.pages.integrationsPage.notTestedYet'
						)}
					</span>
				</div>
				<StatusBadge
					kind={stateTone(agent)}
					label={$t(stateKey(agent))}
					class="w-fit whitespace-nowrap"
				/>
			</div>
		</div>
	</CardHeader>
	<CardContent class="flex min-h-0 flex-1 flex-col gap-3 text-xs">
		<div class="flex items-center justify-between gap-3">
			<span class="text-muted-foreground">{$t('ui.pages.integrationsPage.keyTitle')}</span>
			<!-- A key that does not exist reads as a dash, never as a word. -->
			<span class="mono-data truncate">{agent.key?.token_hint ?? '—'}</span>
		</div>
		{#if warnings.length > 0}
			<div class="max-h-16 overflow-y-auto">
				{#each warnings as copy (copy)}
					<p class="flex items-start gap-1.5 text-warn">
						<Icon name="alert-triangle" size={13} class="mt-px shrink-0" />
						{copy}
					</p>
				{/each}
			</div>
		{/if}
		<div class="mt-auto flex flex-wrap items-end justify-between border-t border-border pt-3 gap-2">
			<div class="flex flex-wrap items-center gap-2">
				{#if !agent.enabled}
					<Button disabled={blocked(running, agent)} onclick={onsetup}>
						<Icon name="plus" size={14} />
						{$t('ui.pages.integrationsPage.setup')}
					</Button>
				{:else}
					<IconAction
						icon="settings"
						variant="outline"
						label={$t('ui.pages.integrationsPage.manage')}
						disabled={blocked(running, agent)}
						onclick={onopen}
					/>
					{#if agent.id === 'codex'}
						<IconAction
							icon="gauge"
							variant="outline"
							pressed={contextOn(agent)}
							tone={contextOn(agent) ? 'accent' : 'default'}
							label={$t(
								contextOn(agent)
									? 'ui.pages.integrationsPage.context1MTurnOff'
									: 'ui.pages.integrationsPage.context1MTurnOn'
							)}
							disabled={blocked(running, agent)}
							spin={runningOn(running, agent, 'context')}
							onclick={ontogglecontext}
						/>
					{/if}
					{#if restartsClient(agent)}
						<IconAction
							icon={runningOn(running, agent, 'restart') ? 'loader' : 'refresh'}
							variant="outline"
							label={$t(
								runningOn(running, agent, 'restart')
									? 'ui.pages.integrationsPage.restarting'
									: 'ui.pages.integrationsPage.restart'
							)}
							disabled={blocked(running, agent)}
							spin={runningOn(running, agent, 'restart')}
							onclick={onrestart}
						/>
					{/if}
					{#if needsRepair(agent)}
						<IconAction
							icon={runningOn(running, agent, 'repair') ? 'loader' : 'wrench'}
							variant="outline"
							tone="accent"
							label={$t(
								runningOn(running, agent, 'repair')
									? 'ui.pages.integrationsPage.repairing'
									: 'ui.pages.integrationsPage.repair'
							)}
							disabled={blocked(running, agent)}
							spin={runningOn(running, agent, 'repair')}
							onclick={onrepair}
						/>
					{/if}
				{/if}
			</div>
			<div class="flex flex-wrap items-center gap-2">
				{#if agent.enabled}
					<IconAction
						icon="trash"
						variant="outline"
						label={$t('ui.pages.integrationsPage.remove')}
						tone="destructive"
						disabled={blocked(running, agent)}
						onclick={onremove}
					/>
				{/if}
			</div>
		</div>
	</CardContent>
</Card>
