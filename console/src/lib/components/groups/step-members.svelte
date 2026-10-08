<script lang="ts">
	import { untrack } from 'svelte';
	import { t } from 'svelte-i18n';
	import { modelPrice, moveMember, providerLabel } from '$lib/catalog';
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
	const shown = $derived(displayMembers(draft, meta));
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
				onadd={(member) => onmembers([...draft.members, member])}
				{onlearn}
			/>
		</div>

		<div
			class="{pane === 'members'
				? 'flex'
				: 'hidden'} min-h-0 flex-1 flex-col gap-2.5 rounded-panel bg-background p-4 md:flex"
		>
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
				<div class="min-h-0 flex-1 overflow-y-auto">
					{#if noteKey}
						<p
							class="mb-2.5 rounded-lg border border-border bg-card p-2.5 text-xs text-muted-foreground"
						>
							{$t(noteKey)}
						</p>
					{/if}
					<ul class="flex flex-col gap-2.5" {@attach sortable(sortableOptions)} role="list">
						{#each shown as member, position (member.provider_id + '/' + member.model_id)}
							{@const index = draft.members.indexOf(member)}
							{@const standing = memberStanding(member)}
							<li
								data-sortable-row
								class="flex flex-col gap-2.5 rounded-panel border border-border bg-card p-3 {member.enabled
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
										<span class="badge border border-warn/40 bg-warn/10 text-warn">
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
					class="flex shrink-0 items-center gap-2 rounded-lg border border-warn/40 bg-warn/10 p-3 text-sm text-warn"
					role="status"
				>
					<Icon name="alert-triangle" size={14} />{$t('ui.pages.groupsPage.warnReasoning')}
				</p>
			{/if}
			{#if capabilities.visionMixed}
				<p
					class="flex shrink-0 items-center gap-2 rounded-lg border border-warn/40 bg-warn/10 p-3 text-sm text-warn"
					role="status"
				>
					<Icon name="alert-triangle" size={14} />{$t('ui.pages.groupsPage.warnVision')}
				</p>
			{/if}
		</div>
	</div>
</div>
