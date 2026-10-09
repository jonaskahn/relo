<script lang="ts">
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import { isCardOpenClick } from '$lib/card-target';
	import Card from '$lib/components/ui/card.svelte';
	import CardContent from '$lib/components/ui/card-content.svelte';
	import CardHeader from '$lib/components/ui/card-header.svelte';
	import CardTitle from '$lib/components/ui/card-title.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import ProviderLogo from '$lib/components/ui/provider-logo.svelte';
	import {
		capabilityWarningKeys,
		eligibleMembers,
		exposedName,
		groupMembersOf,
		groupSetupChips,
		groupProviderCount,
		providerIconLimit,
		providerLabel,
		providerModelCounts,
		visibleProviderModelCounts,
		type GroupSetupChip
	} from '$lib/catalog';
	import { consoleState } from '$lib/console-state.svelte';
	import { cardLayoutCard } from '$lib/theme.svelte';
	import type { Group, Provider } from '$lib/types';

	// GroupCard is one group as a card: what it can promise, the name an agent
	// asks for, what it counts as, and the connections whose logos stand for
	// its members. Clicking anywhere on the card opens the preview; the copy,
	// preview, edit and remove controls keep their own jobs.

	interface Props {
		group: Group;
		providers: Provider[];
		onopen: (group: Group) => void;
		onedit: (group: Group) => void;
		onremove: (group: Group) => void;
	}

	let { group, providers, onopen, onedit, onremove }: Props = $props();

	const members = $derived(groupMembersOf(group));
	const chips = $derived(groupSetupChips(group));
	const pills = $derived(statePills(group));

	function logoId(providerID: string): string {
		if (providerID === '') return '';
		const provider = providers.find((item) => item.id === providerID);
		return provider?.template_id || providerID;
	}

	function chipText(chip: GroupSetupChip): string {
		return chip.label ?? $t(chip.labelKey ?? '');
	}

	function openLabel(group: Group): string {
		const name = $t('ui.common.openCard', {
			values: { name: group.label || group.id }
		});
		const setup = groupSetupChips(group)
			.map((chip) => chip.label ?? $t(chip.labelKey ?? ''))
			.join(', ');
		return setup === '' ? name : name + ', ' + setup;
	}

	async function copyExposed() {
		try {
			await navigator.clipboard.writeText(exposedName(group));
			toast.success($t('ui.pages.groupsPage.copied', { values: { id: exposedName(group) } }));
		} catch {
			toast.error($t('ui.pages.groupsPage.copyFailed'));
		}
	}

	type StatePill = { id: string; label: string; warn: boolean; tip: string };

	// A neutral pill carries a dot; a warning carries an icon as well as its
	// colour, so the state never rests on colour alone.
	function statePills(group: Group): StatePill[] {
		const pills: StatePill[] = [];
		if (!group.enabled) {
			pills.push({
				id: 'off',
				label: $t('ui.pages.groupsPage.chipDisabled'),
				warn: false,
				tip: ''
			});
		}
		if (!group.listed) {
			pills.push({
				id: 'unlisted',
				label: $t('ui.pages.groupsPage.chipUnlisted'),
				warn: false,
				tip: ''
			});
		}
		if (group.shadows_model) {
			pills.push({
				id: 'shadow',
				label: $t('ui.pages.groupsPage.chipShadow'),
				warn: false,
				tip: ''
			});
		}
		// vision_off and reasoning_off already read on the capability line
		// above, so the badge row only repeats what that line cannot say.
		for (const warning of (group.capability_warnings ?? []).filter(
			(warning) => warning !== 'reasoning_off' && warning !== 'vision_off'
		)) {
			const keys = capabilityWarningKeys(warning);
			pills.push({
				id: warning,
				label: $t(keys.chip),
				warn: true,
				tip: $t(keys.tooltip)
			});
		}
		return pills;
	}
</script>

<Card
	class="section-surface card-press {cardLayoutCard(consoleState.settings.cardLayout)} gap-4 py-5"
	onclick={(event) => {
		if (isCardOpenClick(event)) onopen(group);
	}}
>
	<CardHeader class="shrink-0 gap-2">
		<button
			type="button"
			data-plain
			class="flex w-full min-w-0 flex-col items-start gap-1.5 text-left"
			aria-label={openLabel(group)}
			onclick={() => onopen(group)}
		>
			<CardTitle class="w-full truncate text-sm">{group.label || group.id}</CardTitle>
			{#if chips.length > 0}
				<span class="flex flex-wrap gap-1">
					{#each chips as chip (chip.id)}
						<span
							class="badge border border-line text-muted-foreground {chip.id === 'context'
								? 'mono-data'
								: ''}"
							title={$t(chip.tooltipKey)}
						>
							{chipText(chip)}
						</span>
					{/each}
				</span>
			{/if}
		</button>

		<!-- The identity band: the group name and the rule it orders by,
		     then the name an agent actually asks for, which is the value
		     worth copying. -->
		<div class="flex min-w-0 flex-col gap-1">
			<span class="mono-data truncate text-xs text-muted-foreground">
				{group.id} · {$t('ui.pages.groupsPage.strategyName.' + group.strategy)}
			</span>
			<div class="flex min-w-0 items-center gap-1">
				<span
					class="mono-data min-w-0 truncate text-xs text-ink"
					title={$t('ui.pages.groupsPage.exposedHint')}
				>
					{exposedName(group)}
				</span>
				<IconAction
					icon="copy"
					label={$t('ui.pages.groupsPage.copyExposed')}
					onclick={() => void copyExposed()}
				/>
			</div>
		</div>
	</CardHeader>

	<CardContent class="flex min-h-0 flex-1 flex-col gap-3 text-xs">
		{#if pills.length > 0}
			<div class="flex flex-wrap items-center gap-1.5">
				{#each pills as pill (pill.id)}
					<span
						class="badge {pill.warn
							? 'border border-warn/40 bg-warn/10 text-warn'
							: 'bg-sunken text-muted-foreground'}"
						title={pill.tip === '' ? undefined : pill.tip}
					>
						{#if pill.warn}
							<Icon name="alert-triangle" size={11} />
						{:else}
							<span class="size-1.5 rounded-full bg-current" aria-hidden="true"></span>
						{/if}
						{pill.label}
					</span>
				{/each}
			</div>
		{/if}

		<div class="flex flex-wrap items-baseline gap-x-4 gap-y-1 text-muted-foreground">
			<span class="flex items-baseline gap-1.5">
				<span
					class="mono-data text-sm text-ink"
					aria-label={$t('ui.pages.groupsPage.providerCount', {
						values: { count: groupProviderCount(group) }
					})}
				>
					{groupProviderCount(group)}
				</span>
				{$t('ui.pages.groupsPage.providersUsed')}
			</span>
			<span class="flex items-baseline gap-1.5">
				<span
					class="mono-data text-sm text-ink"
					aria-label={$t('ui.pages.groupsPage.modelCount', {
						values: { count: members.length }
					})}
				>
					{members.length}
				</span>
				{$t('ui.pages.groupsPage.modelsUsed')}
			</span>
			<span class="mono-data text-ink">
				{$t('ui.pages.groupsPage.memberCount', {
					values: { eligible: eligibleMembers(group), total: members.length }
				})}
			</span>
		</div>

		{#if members.length === 0}
			<p
				class="rounded-panel border-[1.5px] border-dashed border-line-strong p-3 text-muted-foreground"
			>
				{$t('ui.pages.groupsPage.noMembers')}
			</p>
		{:else}
			{@const counts = providerModelCounts(members)}
			{@const limit = providerIconLimit(consoleState.settings.cardLayout)}
			{@const shown = visibleProviderModelCounts(counts, limit)}
			<ul class="flex flex-wrap gap-x-4 gap-y-3 px-1 pb-1">
				{#each shown.visible as count (count.providerId || 'auto')}
					{@const label =
						count.providerId === ''
							? $t('ui.pages.groupsPage.autoBadge')
							: providerLabel(providers, count.providerId)}
					<li
						class="relative"
						aria-label={$t('ui.pages.groupsPage.providerModels', {
							values: { label, count: count.count }
						})}
					>
						<ProviderLogo id={logoId(count.providerId)} {label} size="sm" round />
						<span
							class="absolute -right-1 -bottom-1 flex h-4 min-w-4 items-center justify-center rounded-full border border-line bg-surface px-1 text-[0.65rem] leading-none font-medium tabular-nums text-ink"
							aria-hidden="true"
						>
							{count.count}
						</span>
					</li>
				{/each}
				{#if shown.overflow > 0}
					<li
						class="relative"
						aria-label={$t('ui.pages.groupsPage.providersOverflow', {
							values: { count: shown.overflow }
						})}
					>
						<span
							class="flex size-9 shrink-0 items-center justify-center rounded-full border border-line bg-surface text-muted-foreground"
							aria-hidden="true"
						>
							<Icon name="dots" size={18} />
						</span>
					</li>
				{/if}
			</ul>
		{/if}

		<div class="mt-auto flex items-center justify-between border-t border-border pt-3 gap-2">
			<div class="flex items-center gap-1">
				<IconAction
					icon="eye"
					label={$t('ui.pages.groupsPage.preview')}
					variant="outline"
					onclick={() => onopen(group)}
				/>
				<IconAction
					icon="pencil"
					label={$t('ui.pages.groupsPage.edit')}
					variant="outline"
					onclick={() => onedit(group)}
				/>
			</div>
			<IconAction
				icon="trash"
				label={$t('ui.common.remove')}
				variant="outline"
				tone="destructive"
				onclick={() => onremove(group)}
			/>
		</div>
	</CardContent>
</Card>
