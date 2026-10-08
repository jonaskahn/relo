<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';
	import { api, ApiError } from '$lib/api';
	import {
		capabilityWarningKeys,
		eligibleMembers,
		exposedName,
		formatContext,
		formatPrice,
		groupMembersOf,
		modelPrice,
		providerLabel
	} from '$lib/catalog';
	import type { Group, GroupMember, Model, Provider, RoutePreview } from '$lib/types';
	import CenteredModal from '$lib/components/ui/centered-modal.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import Button from '$lib/components/ui/button.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import GroupMemberRow from './group-member-row.svelte';

	// GroupDetail is the read view of one group: the name a request asks for,
	// the members it resolves through, and the order the daemon resolves them
	// in right now. Editing stays the editor's job, so the footer hands the
	// group over rather than becoming one.
	interface Props {
		open?: boolean;
		group: Group | null;
		providers: Provider[];
		meta: Record<string, Model>;
		onedit: (group: Group) => void;
		ondelete: (group: Group) => void;
	}

	let { open = $bindable(false), group, providers, meta, onedit, ondelete }: Props = $props();

	let preview = $state<RoutePreview | null>(null);
	let previewLoading = $state(false);
	// A name that resolves to nothing is answered with 404, and the typed
	// client hands back no body for a failed call, so this records the
	// difference: a resolution with no candidates is not a failed read. The
	// member list below carries why each member cannot serve.
	let previewNone = $state(false);
	let previewFailure = $state('');

	const members = $derived(group === null ? [] : groupMembersOf(group));
	const candidates = $derived(preview?.candidates ?? []);
	const skipped = $derived(preview?.skipped ?? []);
	const warnings = $derived(preview?.warnings ?? []);

	$effect(() => {
		if (open && group !== null) void readPreview(group);
	});

	// The preview is resolved the way an inference request would: by the name
	// a client asks for, with no requirement declared, so the order it returns
	// is the order a plain request reads.
	async function readPreview(target: Group) {
		previewLoading = true;
		previewNone = false;
		previewFailure = '';
		try {
			preview = await api<RoutePreview>('/routes/preview?model=' + encodeURIComponent(target.id));
		} catch (error) {
			preview = null;
			if (error instanceof ApiError && error.status === 404) {
				previewNone = true;
			} else {
				previewFailure = error instanceof Error ? error.message : String(error);
			}
		} finally {
			previewLoading = false;
		}
	}

	function metaOf(member: GroupMember): Model | undefined {
		return meta[member.provider_id + '/' + member.model_id];
	}

	function coverageOf(member: GroupMember): string {
		if (!member.eligible || (member.active_accounts ?? 0) === 0) return '';
		return $t('ui.pages.groupsPage.coverage', {
			values: { serving: member.serving_accounts ?? 0, active: member.active_accounts ?? 0 }
		});
	}

	function capabilityWarningChip(warning: string): string {
		return capabilityWarningKeys(warning).chip;
	}

	function capabilityWarningText(warning: string): string {
		return capabilityWarningKeys(warning).tooltip;
	}

	/** Reads one failover switch as the word the page already uses for on and off. */
	function switchWord(on: boolean): string {
		return on ? $t('ui.pages.groupsPage.memberEnabled') : $t('ui.pages.groupsPage.chipDisabled');
	}

	async function copyExposed() {
		if (group === null) return;
		try {
			await navigator.clipboard.writeText(exposedName(group));
			toast.success($t('ui.pages.groupsPage.copied', { values: { id: exposedName(group) } }));
		} catch {
			toast.error($t('ui.pages.groupsPage.copyFailed'));
		}
	}
</script>

{#if group}
	<CenteredModal
		bind:open
		size="wide"
		title={group.label || group.id}
		description={$t('ui.pages.groupsPage.detailHint')}
	>
		<div class="flex flex-col gap-6">
			<dl class="grid gap-3 text-sm sm:grid-cols-2">
				<div class="min-w-0">
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.id')}</dt>
					<dd class="mono-data truncate">{group.id}</dd>
				</div>
				<div class="min-w-0">
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.exposedHint')}</dt>
					<dd class="mt-0.5 flex min-w-0 items-center gap-1.5">
						<span class="mono-data min-w-0 truncate">{exposedName(group)}</span>
						<IconAction
							icon="copy"
							label={$t('ui.pages.groupsPage.copyExposed')}
							onclick={() => void copyExposed()}
						/>
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.strategy')}</dt>
					<dd>
						{$t('ui.pages.groupsPage.strategyName.' + group.strategy)}
						<span class="text-muted-foreground">
							· {$t('ui.pages.groupsPage.strategyHint.' + group.strategy)}
						</span>
					</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.switchOn4xx')}</dt>
					<dd>{switchWord(group.switch_on_4xx)}</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.switchOn5xx')}</dt>
					<dd>{switchWord(group.switch_on_5xx)}</dd>
				</div>
				<div>
					<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.membersLabel')}</dt>
					<dd class="flex flex-wrap items-center gap-1.5">
						{$t('ui.pages.groupsPage.memberCount', {
							values: { eligible: eligibleMembers(group), total: members.length }
						})}
						{#if !group.enabled}
							<span class="badge border border-border text-muted-foreground">
								{$t('ui.pages.groupsPage.chipDisabled')}
							</span>
						{/if}
						{#if !group.listed}
							<span class="badge border border-border text-muted-foreground">
								{$t('ui.pages.groupsPage.chipUnlisted')}
							</span>
						{/if}
						{#each group.capability_warnings ?? [] as warning (warning)}
							<span
								class="badge border border-warn/40 bg-warn/10 text-warn"
								title={$t(capabilityWarningText(warning))}
							>
								<Icon name="alert-triangle" size={11} />
								{$t(capabilityWarningChip(warning))}
							</span>
						{/each}
					</dd>
				</div>
				{#if group.context_window}
					<div>
						<dt class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.budgetContext')}</dt>
						<dd class="mono-data" title={$t('ui.pages.groupsPage.budgetHint')}>
							{formatContext(group.context_window)}{#if group.max_output}
								· {formatContext(group.max_output)} {$t('ui.pages.groupsPage.budgetMaxOutput')}{/if}
						</dd>
					</div>
				{/if}
			</dl>

			<div>
				<h3 class="field-label">{$t('ui.pages.groupsPage.previewLabel')}</h3>
				{#if previewLoading}
					<p class="mt-2 text-sm text-muted-foreground" role="status">
						{$t('ui.pages.groupsPage.previewLoading')}
					</p>
				{:else if previewNone}
					<p
						class="mt-2 rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground"
					>
						{group.enabled
							? $t('ui.pages.groupsPage.previewNone')
							: $t('ui.pages.groupsPage.previewOff')}
					</p>
				{:else if previewFailure}
					<p class="mt-2 flex items-center gap-2 text-sm text-destructive" role="alert">
						<Icon name="alert-triangle" size={15} />
						{$t('ui.pages.groupsPage.previewFailed')}
						<span class="text-xs">{previewFailure}</span>
					</p>
				{:else if preview}
					{#if candidates.length === 0}
						<p
							class="mt-2 rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground"
						>
							{$t('ui.pages.groupsPage.previewNone')}
						</p>
					{:else}
						<ol class="mt-2 flex flex-col gap-2">
							{#each candidates as candidate, index (candidate.provider_id + '/' + candidate.model_id)}
								<li class="flex items-center gap-2 rounded-panel border border-line px-2.5 py-1.5">
									<span class="mono-data w-4 shrink-0 text-muted-foreground">{index + 1}</span>
									<ProviderLogo
										id={candidate.provider_id}
										label={providerLabel(providers, candidate.provider_id)}
										size="xs"
									/>
									<div class="min-w-0 flex-1">
										<span class="block truncate text-sm">
											{meta[candidate.provider_id + '/' + candidate.model_id]?.name ||
												candidate.model_id}
										</span>
										<span class="mono-data block truncate text-muted-foreground">
											{candidate.provider_id}/{candidate.model_id}
										</span>
									</div>
									{#if candidate.prices}
										<span class="mono-data shrink-0 text-muted-foreground">
											{formatPrice(candidate.prices.input)} / {formatPrice(candidate.prices.output)}
										</span>
									{/if}
								</li>
							{/each}
						</ol>
					{/if}
					{#if skipped.length > 0}
						<h4 class="mt-4 text-xs font-medium text-muted-foreground">
							{$t('ui.pages.groupsPage.previewSkipped')}
						</h4>
						<ul class="mt-1 flex flex-col gap-1">
							{#each skipped as entry (entry.provider_id + '/' + entry.model_id)}
								<li
									class="mono-data flex flex-wrap items-baseline gap-2 text-xs text-muted-foreground"
								>
									<span class="truncate">{entry.provider_id}/{entry.model_id}</span>
									{#if entry.reason}<span class="text-warn">{entry.reason}</span>{/if}
								</li>
							{/each}
						</ul>
					{/if}
					{#if warnings.length > 0}
						<h4 class="mt-4 text-xs font-medium text-muted-foreground">
							{$t('ui.pages.groupsPage.previewWarnings')}
						</h4>
						<ul class="mt-1 flex flex-col gap-1">
							{#each warnings as entry (entry.provider_id + '/' + entry.model_id + ':' + (entry.reason ?? ''))}
								{#if entry.reason}
									<li
										class="mono-data flex flex-wrap items-baseline gap-2 text-xs text-muted-foreground"
									>
										<span class="truncate">{entry.provider_id}/{entry.model_id}</span>
										<span class="text-warn">{entry.reason}</span>
									</li>
								{/if}
							{/each}
						</ul>
					{/if}
				{/if}
			</div>

			<div>
				<h3 class="field-label">{$t('ui.pages.groupsPage.membersLabel')}</h3>
				{#if members.length === 0}
					<p
						class="mt-2 rounded-lg border border-dashed border-border p-3 text-sm text-muted-foreground"
					>
						{$t('ui.pages.groupsPage.noMembers')}
					</p>
				{:else}
					<ul class="mt-2 overflow-hidden rounded-lg border border-border">
						{#each members as member, index (member.provider_id + '/' + member.model_id)}
							<GroupMemberRow
								{member}
								position={index + 1}
								title={metaOf(member)?.name}
								label={providerLabel(providers, member.provider_id)}
								price={modelPrice(metaOf(member))}
								weight={group.strategy === 'weighted' ? member.weight : null}
								coverage={coverageOf(member)}
							/>
						{/each}
					</ul>
				{/if}
			</div>
		</div>

		{#snippet footer()}
			<div class="modal-footer items-center">
				<!-- The destructive action leads the footer group, named rather than
				     a bare icon. -->
				<Button variant="destructive" onclick={() => ondelete(group)}>
					<Icon name="trash" size={14} />
					{$t('ui.common.remove')}
				</Button>
				<Button onclick={() => onedit(group)}>
					<Icon name="pencil" size={14} />
					{$t('ui.pages.groupsPage.edit')}
				</Button>
			</div>
		{/snippet}
	</CenteredModal>
{/if}
