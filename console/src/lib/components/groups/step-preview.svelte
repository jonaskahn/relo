<script lang="ts">
	import { t } from 'svelte-i18n';
	import { modelPrice, providerLabel } from '$lib/catalog';
	import {
		displayMembers,
		memberKind,
		memberStanding,
		resolutionReason,
		weightShare,
		type MemberCapabilities,
		type RouteDraft
	} from '$lib/route-stepper';
	import type { GroupMember, Model, Provider } from '$lib/types';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	interface Props {
		draft: RouteDraft;
		providers: Provider[];
		meta: Record<string, Model>;
		capabilities: MemberCapabilities;
	}

	let { draft, providers, meta, capabilities }: Props = $props();

	const listed = $derived(displayMembers(draft, meta));
	const candidates = $derived(listed.filter((member) => memberStanding(member) === 'serving'));
	const skipped = $derived(listed.filter((member) => memberStanding(member) !== 'serving'));

	function metaOf(member: GroupMember): Model | undefined {
		return meta[member.provider_id + '/' + member.model_id];
	}

	function whyOf(member: GroupMember, position: number): string {
		if (draft.strategy === 'weighted') {
			return $t('ui.pages.groupsPage.when.weighted', {
				values: { share: weightShare(member, draft.members) }
			});
		}
		return $t(resolutionReason(draft.strategy, position));
	}

	function skipReason(member: GroupMember): string {
		if (memberStanding(member) === 'paused') return $t('ui.pages.groupsPage.skipPaused');
		return member.reason || $t('ui.pages.groupsPage.coverageNone');
	}
</script>

<div class="flex flex-col gap-5">
	<p class="text-sm text-muted-foreground">{$t('ui.pages.groupsPage.reviewHint')}</p>
	{#if capabilities.toolsConflict}
		<p
			class="flex items-center gap-2 rounded-lg border border-destructive/50 bg-destructive/5 p-3 text-sm text-destructive"
			role="alert"
		>
			<Icon name="alert-triangle" size={14} />{$t('ui.pages.groupsPage.toolsConflict')}
		</p>
	{/if}
	{#if capabilities.reasoningMixed}
		<p
			class="flex items-center gap-2 rounded-lg border border-warn/40 bg-warn/10 p-3 text-sm text-warn"
			role="status"
		>
			<Icon name="alert-triangle" size={14} />{$t('ui.pages.groupsPage.warnReasoning')}
		</p>
	{/if}
	{#if capabilities.visionMixed}
		<p
			class="flex items-center gap-2 rounded-lg border border-warn/40 bg-warn/10 p-3 text-sm text-warn"
			role="status"
		>
			<Icon name="alert-triangle" size={14} />{$t('ui.pages.groupsPage.warnVision')}
		</p>
	{/if}

	<div class="grid gap-6 lg:grid-cols-[minmax(0,5fr)_minmax(0,6fr)]">
		<div class="self-start rounded-panel border border-border p-4.5">
			<dl class="grid gap-3.5 text-sm">
				<div class="min-w-0">
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.id')}</dt>
					<dd class="mono-data truncate">{draft.id.trim()}</dd>
				</div>
				<div class="min-w-0">
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.labelLabel')}</dt>
					<dd class="truncate">{draft.label.trim() || draft.id.trim()}</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.strategy')}</dt>
					<dd>
						{$t('ui.pages.groupsPage.strategyName.' + draft.strategy)}
						<span class="text-muted-foreground">
							· {$t('ui.pages.groupsPage.strategyHint.' + draft.strategy)}
						</span>
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">{$t('ui.common.status')}</dt>
					<dd class="flex flex-wrap items-center gap-1.5">
						<span
							class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs {draft.enabled
								? 'bg-accent-soft text-accent-ink'
								: 'bg-muted text-muted-foreground'}"
						>
							{$t('ui.pages.groupsPage.enabled')}
						</span>
						<span
							class="inline-flex items-center rounded-full px-2.5 py-0.5 text-xs {draft.listed
								? 'bg-accent-soft text-accent-ink'
								: 'bg-muted text-muted-foreground'}"
						>
							{$t('ui.pages.groupsPage.listed')}
						</span>
					</dd>
				</div>
			</dl>
		</div>

		<div class="flex min-w-0 flex-col">
			{#if candidates.length === 0}
				<p
					class="rounded-panel border-[1.5px] border-dashed border-accent-line bg-background p-5 text-center text-sm text-muted-foreground"
				>
					{$t('ui.pages.groupsPage.emptyResolution')}
				</p>
			{:else}
				<span
					class="mono-data self-start rounded-control bg-primary px-2.5 py-1.5 text-primary-foreground"
					>{draft.id.trim()}</span
				>
				<div class="mt-3.5 flex flex-col">
					{#each candidates as member, position (member.provider_id + '/' + member.model_id)}
						<div class="flex gap-3">
							<div class="flex w-6.5 shrink-0 flex-col items-center">
								<span
									class="flex size-6.5 items-center justify-center rounded-full text-xs font-semibold {position ===
									0
										? 'bg-primary text-primary-foreground'
										: 'bg-accent-soft text-accent-ink'}"
								>
									{#if draft.strategy === 'round-robin'}
										<Icon name="refresh" size={13} />
									{:else}
										{position + 1}
									{/if}
								</span>
								{#if position < candidates.length - 1}
									<span
										class="w-0.5 flex-1 bg-[repeating-linear-gradient(to_bottom,var(--accent-100)_0_4px,transparent_4px_8px)]"
										aria-hidden="true"
									></span>
								{/if}
							</div>
							<div
								class="mb-2.5 flex min-w-0 flex-1 items-center gap-3 rounded-panel border p-3 {position ===
								0
									? 'border-primary bg-accent-soft'
									: 'border-border'}"
							>
								{#if memberKind(member) === 'auto'}
									<span
										class="flex size-6 shrink-0 items-center justify-center rounded-lg border border-accent-line bg-accent-soft text-[0.6rem] font-semibold text-accent-ink"
										title={$t('ui.pages.groupsPage.autoHint')}
									>
										{$t('ui.pages.groupsPage.autoBadge')}
									</span>
								{:else}
									<ProviderLogo
										id={member.provider_id}
										label={providerLabel(providers, member.provider_id)}
										size="xs"
									/>
								{/if}
								<div class="min-w-0 flex-1">
									<span class="block truncate text-sm"
										>{metaOf(member)?.name || member.model_id}</span
									>
									<span class="block truncate text-xs text-muted-foreground"
										>{whyOf(member, position)}</span
									>
								</div>
								{#if modelPrice(metaOf(member))}
									<span class="mono-data shrink-0 text-muted-foreground"
										>{modelPrice(metaOf(member))}</span
									>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}

			{#if skipped.length > 0}
				<div class="mt-1 flex flex-col gap-2 opacity-[0.55]">
					{#each skipped as member (member.provider_id + '/' + member.model_id)}
						<div class="flex min-w-0 items-center gap-3 rounded-panel border border-border p-3">
							{#if memberKind(member) === 'auto'}
								<span
									class="flex size-6 shrink-0 items-center justify-center rounded-lg border border-accent-line bg-accent-soft text-[0.6rem] font-semibold text-accent-ink"
									title={$t('ui.pages.groupsPage.autoHint')}
								>
									{$t('ui.pages.groupsPage.autoBadge')}
								</span>
							{:else}
								<ProviderLogo
									id={member.provider_id}
									label={providerLabel(providers, member.provider_id)}
									size="xs"
								/>
							{/if}
							<div class="min-w-0 flex-1">
								<span class="block truncate text-sm">{metaOf(member)?.name || member.model_id}</span
								>
								<span class="mono-data block truncate text-muted-foreground">
									{memberKind(member) === 'auto'
										? member.model_id
										: member.provider_id + '/' + member.model_id}
								</span>
							</div>
							<span class="badge shrink-0 border border-border text-muted-foreground"
								>{skipReason(member)}</span
							>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	</div>
</div>
