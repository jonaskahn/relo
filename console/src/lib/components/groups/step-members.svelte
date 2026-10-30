<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { modelPrice, moveMember, providerLabel } from '$lib/catalog';
	import { similarModels, groupSuggestions } from '$lib/group-suggestions';
	import BillingChip from '$lib/components/providers/billing-chip.svelte';
	import FreeAvailableChip from '$lib/components/providers/free-available-chip.svelte';
	import {
		displayMembers,
		memberKind,
		memberStanding,
		weightShare,
		type MemberCapabilities
	} from '$lib/route-stepper';
	import { sortable, type SortableOptions } from '$lib/sortable';
	import type { GroupMember, Model, Provider } from '$lib/types';
	import Button from '$lib/components/ui/button.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Segmented from '$lib/components/ui/segmented.svelte';
	import { Switch } from 'bits-ui';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import MemberPicker from './member-picker.svelte';
	import type { RouteDraft } from '$lib/route-stepper';

	interface Props {
		draft: RouteDraft;
		configured: Provider[];
		providers: Provider[];
		meta: Record<string, Model>;
		problems?: Record<string, string>;
		capabilities: MemberCapabilities;
		creating?: boolean;
		ondraft: (patch: Partial<RouteDraft>) => void;
		onmembers: (members: GroupMember[]) => void;
		onmove: (from: number, to: number) => void;
		onlearn: (models: Model[]) => void;
	}

	let {
		draft,
		configured,
		providers,
		meta,
		problems = {},
		capabilities,
		creating = false,
		ondraft,
		onmembers,
		onmove,
		onlearn
	}: Props = $props();

	let pane = $state<'members' | 'add'>(
		untrack(() => (draft.members.length > 0 ? 'members' : 'add'))
	);

	// Priority is the one ordering the operator sets by hand; the daemon
	// turns the others automatically, so those rows offer no move controls.
	const manual = $derived(draft.strategy === 'priority');

	// The reorder attachment reads its options when a gesture starts and is
	// attached once, so a strategy change never tears down a list mid-drag.
	const sortableOptions: SortableOptions = {
		handle: '[data-sortable-handle]',
		onMove: (from, to) => onmove(from, to),
		disabled: () => !manual
	};
	function toggleLane(lane: 'payg' | 'plan' | 'free') {
		const next = new Set(draft.lanes.filter((entry) => entry !== 'manual'));
		if (next.has(lane)) next.delete(lane);
		else next.add(lane);
		ondraft({ lanes: [...next] });
	}

	const shown = $derived(displayMembers(draft, meta));

	const suggestionSeeds = $derived(
		creating
			? draft.members
					.map((member) => meta[member.provider_id + '/' + member.model_id])
					.filter((model): model is Model => Boolean(model))
			: []
	);
	const suggestionReport = $derived.by(() => {
		if (!creating) return { models: [] as Model[], conflicts: 0 };
		const models: Model[] = [];
		const seen = new Set<string>();
		let conflicts = 0;
		for (const seed of suggestionSeeds) {
			const report = similarModels(
				Object.values(meta),
				seed,
				providers,
				draft.lanes,
				draft.members
			);
			conflicts += report.conflicts;
			for (const model of report.models) {
				const key = model.provider_id + '/' + model.model_id;
				if (seen.has(key)) continue;
				seen.add(key);
				models.push(model);
			}
		}
		return { models, conflicts };
	});
	const suggestions = $derived(suggestionReport.models);
	const suggestionGroups = $derived(groupSuggestions(suggestions));
	const conflictCount = $derived(suggestionReport.conflicts);

	function addOne(model: Model) {
		if (
			draft.members.some(
				(m) => m.provider_id === model.provider_id && m.model_id === model.model_id
			)
		)
			return;
		onmembers([
			...draft.members,
			{
				provider_id: model.provider_id,
				model_id: model.model_id,
				kind: 'model',
				weight: 1,
				enabled: true,
				eligible: model.routable,
				serving_accounts: model.serving_accounts ?? 0,
				active_accounts: model.active_accounts ?? 0
			}
		]);
	}

	function addAll(models: Model[]) {
		const existing = new Set(draft.members.map((m) => m.provider_id + '/' + m.model_id));
		const toAdd = models.filter((m) => !existing.has(m.provider_id + '/' + m.model_id));
		if (toAdd.length === 0) return;
		onmembers([
			...draft.members,
			...toAdd.map((model) => ({
				provider_id: model.provider_id,
				model_id: model.model_id,
				kind: 'model' as const,
				weight: 1,
				enabled: true,
				eligible: model.routable,
				serving_accounts: model.serving_accounts ?? 0,
				active_accounts: model.active_accounts ?? 0
			}))
		]);
	}
	const noteKey = $derived(
		draft.strategy === 'cheapest'
			? 'ui.pages.groupsPage.autoNote.cheapest'
			: draft.strategy === 'fastest'
				? 'ui.pages.groupsPage.autoNote.fastest'
				: draft.strategy === 'round-robin'
					? 'ui.pages.groupsPage.autoNote.round-robin'
					: ''
	);

	const panes = $derived([
		{ value: 'add' as const, label: $t('ui.pages.groupsPage.add') },
		{ value: 'members' as const, label: $t('ui.pages.groupsPage.membersLabel') }
	]);

	function memberKey(providerID: string, modelID: string): string {
		return providerID + '/' + modelID;
	}

	function metaOf(member: GroupMember): Model | undefined {
		return meta[memberKey(member.provider_id, member.model_id)];
	}

	function memberCode(member: GroupMember): string {
		return memberKind(member) === 'auto'
			? member.model_id
			: memberKey(member.provider_id, member.model_id);
	}

	function coverageOf(member: GroupMember): string {
		if (!member.eligible || (member.active_accounts ?? 0) === 0) return '';
		return $t('ui.pages.groupsPage.coverage', {
			values: {
				serving: member.serving_accounts ?? 0,
				active: member.active_accounts ?? 0
			}
		});
	}

	function replace(index: number, patch: Partial<GroupMember>) {
		onmembers(
			draft.members.map((member, position) =>
				position === index ? { ...member, ...patch } : member
			)
		);
	}

	function removeAt(index: number) {
		onmembers(draft.members.filter((_, position) => position !== index));
	}

	function moveRow(index: number, step: number) {
		onmembers(moveMember(draft.members, index, step));
	}
</script>

<div class="flex min-h-0 flex-1 flex-col gap-3">
	<div class="md:hidden">
		<Segmented
			bind:value={pane}
			options={panes}
			ariaLabel={$t('ui.pages.groupsPage.step.members')}
		/>
	</div>
	<div class="grid min-h-0 flex-1 grid-cols-1 grid-rows-[minmax(0,1fr)] gap-4 md:grid-cols-2">
		<div
			class="{pane === 'add'
				? 'flex'
				: 'hidden'} min-h-0 min-w-0 flex-1 flex-col gap-2.5 rounded-panel bg-background p-4 md:flex"
		>
			<MemberPicker
				{configured}
				{providers}
				members={draft.members}
				lanes={draft.lanes}
				onadd={(member) => onmembers([...draft.members, member])}
				{onlearn}
			/>
		</div>

		<div
			class="{pane === 'members'
				? 'flex'
				: 'hidden'} min-h-0 flex-1 flex-col gap-2.5 rounded-panel bg-background p-4 md:flex"
		>
			{#if creating}
				<div class="flex shrink-0 flex-wrap items-center gap-2">
					<span class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.billingScope')}</span
					>
					<button
						type="button"
						aria-pressed={draft.lanes.includes('payg')}
						class="rounded-control border px-2 py-0.5 text-xs {draft.lanes.includes('payg')
							? 'bg-accent-soft ring-1 ring-accent-border'
							: 'hover:bg-control-hover'}"
						onclick={() => toggleLane('payg')}
						>{$t('ui.pages.providersPage.list.tagPayAsYouGo')}</button
					>
					<button
						type="button"
						aria-pressed={draft.lanes.includes('plan')}
						class="rounded-control border px-2 py-0.5 text-xs {draft.lanes.includes('plan')
							? 'bg-accent-soft ring-1 ring-accent-border'
							: 'hover:bg-control-hover'}"
						onclick={() => toggleLane('plan')}
						>{$t('ui.pages.providersPage.list.tagPaidPlan')}</button
					>
					<button
						type="button"
						aria-pressed={draft.lanes.includes('free')}
						class="rounded-control border px-2 py-0.5 text-xs {draft.lanes.includes('free')
							? 'bg-accent-soft ring-1 ring-accent-border'
							: 'hover:bg-control-hover'}"
						onclick={() => toggleLane('free')}>{$t('ui.pages.providersPage.list.tagFree')}</button
					>
					<button
						type="button"
						aria-pressed={draft.lanes.length === 0}
						class="rounded-control border px-2 py-0.5 text-xs {draft.lanes.length === 0
							? 'bg-accent-soft ring-1 ring-accent-border'
							: 'hover:bg-control-hover'}"
						onclick={() => ondraft({ lanes: [] })}>{$t('ui.pages.groupsPage.laneManual')}</button
					>
				</div>
			{/if}
			<div class="flex shrink-0 items-center justify-between gap-2">
				<span class="field-label">{$t('ui.pages.groupsPage.membersLabel')}</span>
				<span class="mono-data text-xs text-muted-foreground">{draft.members.length}</span>
			</div>
			{#if draft.members.length === 0}
				<p
					class="rounded-panel border-[1.5px] border-dashed border-accent-line bg-card p-6 text-center text-sm text-muted-foreground"
				>
					{$t('ui.pages.groupsPage.noMembers')}
				</p>
			{:else}
				{#if creating && suggestions.length > 0}
					<div class="shrink-0 rounded-panel bg-accent-soft ring-1 ring-accent-border p-3">
						<div class="mb-2 flex items-center justify-between gap-2">
							<span class="text-xs font-semibold text-accent-ink"
								>{$t('ui.pages.groupsPage.suggestTitle')}</span
							>
							<span class="text-xs text-muted-foreground"
								>{$t('ui.pages.groupsPage.suggestHint')}</span
							>
						</div>
						{#each suggestionGroups as group (group.family)}
							<div class="mb-2">
								<div class="mb-1 flex items-center justify-between gap-2">
									<span class="text-xs font-semibold">{group.family}</span>
									<Button variant="ghost" size="sm" onclick={() => addAll(group.models)}>
										{$t('ui.pages.groupsPage.suggestAddAll', {
											values: { count: group.models.length }
										})}
									</Button>
								</div>
								<div class="flex flex-wrap gap-1.5">
									{#each group.models as model (model.provider_id + '/' + model.model_id)}
										<button
											type="button"
											aria-pressed="false"
											class="flex items-center gap-1.5 rounded-control bg-card shadow-float-1 px-2 py-1 text-xs"
											onclick={() => addOne(model)}
										>
											<ProviderLogo
												id={model.provider_id}
												label={providerLabel(providers, model.provider_id)}
												size="xs"
											/>
											<span class="truncate">{model.name || model.model_id}</span>
											<BillingChip provider={providers.find((p) => p.id === model.provider_id)!} />
											<FreeAvailableChip
												provider={providers.find((p) => p.id === model.provider_id)!}
											/>
										</button>
									{/each}
								</div>
							</div>
						{/each}
						{#if conflictCount > 0}
							<p class="text-xs text-muted-foreground">
								{$t('ui.pages.groupsPage.suggestConflict', { values: { count: conflictCount } })}
							</p>
						{/if}
					</div>
				{/if}
				<div class="min-h-0 flex-1 overflow-y-auto">
					{#if noteKey}
						<p class="mb-2.5 rounded-lg bg-card shadow-float-1 p-2.5 text-xs text-muted-foreground">
							{$t(noteKey)}
						</p>
					{/if}
					<ul class="flex flex-col gap-2.5" {@attach sortable(sortableOptions)} role="list">
						{#each shown as member, position (member.provider_id + '/' + member.model_id)}
							{@const index = draft.members.indexOf(member)}
							{@const standing = memberStanding(member)}
							<li
								data-sortable-row
								class="flex flex-col gap-2.5 rounded-panel bg-card shadow-float-1 p-3 {member.enabled
									? ''
									: 'opacity-[0.55]'}"
							>
								<div class="flex items-center gap-2.5">
									{#if manual}
										<span
											data-sortable-handle
											aria-hidden="true"
											class="cursor-grab touch-none text-muted-foreground/60 hover:text-muted-foreground"
										>
											<Icon name="grip-vertical" size={16} />
										</span>
									{/if}
									{#if manual || draft.strategy === 'cheapest'}
										<span
											class="flex size-5.5 shrink-0 items-center justify-center rounded-full bg-accent-soft text-[0.7rem] font-semibold text-accent-ink"
										>
											{position + 1}
										</span>
									{/if}
									{#if memberKind(member) === 'auto'}
										<span
											class="flex size-6 shrink-0 items-center justify-center rounded-lg bg-accent-soft ring-1 ring-accent-border text-[0.6rem] font-semibold text-accent-ink"
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
										<span class="mono-data block truncate text-muted-foreground"
											>{memberCode(member)}</span
										>
									</div>
									<Switch.Root
										id={'member-' + index}
										checked={member.enabled}
										onCheckedChange={(checked) => replace(index, { enabled: checked })}
										aria-label={$t('ui.pages.groupsPage.memberEnabled')}
									/>
								</div>
								<div class="flex flex-wrap items-center gap-2">
									{#if modelPrice(metaOf(member))}
										<span class="mono-data rounded-sm bg-muted px-2 py-0.5 text-muted-foreground">
											{modelPrice(metaOf(member))}
										</span>
									{/if}
									{#if standing === 'ineligible'}
										<span class="badge bg-warn/10 text-warn">
											<Icon name="alert-triangle" size={11} />
											{member.reason || $t('ui.pages.groupsPage.coverageNone')}
										</span>
									{:else if coverageOf(member)}
										<span class="mono-data rounded-sm bg-muted px-2 py-0.5 text-muted-foreground">
											{coverageOf(member)}
										</span>
									{/if}
									{#if draft.strategy === 'weighted'}
										<label class="flex items-center gap-1.5 text-xs text-muted-foreground">
											{$t('ui.pages.groupsPage.weight')}
											<Input
												class="w-14 text-center"
												type="number"
												min="1"
												max="1000"
												aria-label={$t('ui.pages.groupsPage.weight')}
												value={member.weight}
												oninput={(event) =>
													replace(index, { weight: Number(event.currentTarget.value) || 1 })}
											/>
											<span class="mono-data">{weightShare(member, draft.members)}%</span>
										</label>
									{/if}
									<span class="flex-1"></span>
									{#if manual}
										<IconAction
											icon="arrow-up"
											label={$t('ui.pages.groupsPage.moveUp')}
											disabled={position === 0}
											onclick={() => moveRow(index, -1)}
										/>
										<IconAction
											icon="arrow-down"
											label={$t('ui.pages.groupsPage.moveDown')}
											disabled={position === shown.length - 1}
											onclick={() => moveRow(index, 1)}
										/>
									{/if}
									<Button variant="destructive" size="sm" onclick={() => removeAt(index)}>
										<Icon name="trash" size={13} />
										{$t('ui.common.remove')}
									</Button>
								</div>
							</li>
						{/each}
					</ul>
				</div>
			{/if}
			{#if problems.members}
				<p class="field-error shrink-0" role="alert">
					<Icon name="circle-alert" size={12} />{$t(problems.members)}
				</p>
			{/if}
			{#if capabilities.reasoningMixed}
				<p
					class="flex shrink-0 items-center gap-2 rounded-lg bg-warn/10 shadow-float-1 p-3 text-sm text-warn"
					role="status"
				>
					<Icon name="alert-triangle" size={14} />{$t('ui.pages.groupsPage.warnReasoning')}
				</p>
			{/if}
			{#if capabilities.visionMixed}
				<p
					class="flex shrink-0 items-center gap-2 rounded-lg bg-warn/10 shadow-float-1 p-3 text-sm text-warn"
					role="status"
				>
					<Icon name="alert-triangle" size={14} />{$t('ui.pages.groupsPage.warnVision')}
				</p>
			{/if}
		</div>
	</div>
</div>
