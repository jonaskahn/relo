<script lang="ts">
	import { t } from 'svelte-i18n';
	import Field from '$lib/components/ui/field.svelte';
	import FieldSwitch from '$lib/components/ui/field-switch.svelte';
	import Input from '$lib/components/ui/input.svelte';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';
	import OrderingFlow from './ordering-flow.svelte';
	import { STRATEGIES } from '$lib/catalog';
	import type { RouteDraft } from '$lib/route-stepper';

	interface Props {
		draft: RouteDraft;
		creating: boolean;
		problems?: Record<string, string>;
		ondraft: (patch: Partial<RouteDraft>) => void;
	}

	let { draft, creating, problems = {}, ondraft }: Props = $props();

	const orderIcons: Record<(typeof STRATEGIES)[number], IconName> = {
		priority: 'list',
		'round-robin': 'refresh',
		weighted: 'chart-bar',
		cheapest: 'tag',
		fastest: 'bolt'
	};
</script>

<div class="flex flex-col gap-6">
	<div class="field-grid">
		<Field
			id="route-id"
			label={$t('ui.pages.groupsPage.id')}
			error={problems.id ? $t(problems.id) : ''}
		>
			<Input
				id="route-id"
				class="font-mono"
				value={draft.id}
				disabled={!creating}
				placeholder="fast"
				oninput={(event) => ondraft({ id: event.currentTarget.value })}
			/>
		</Field>
		<Field id="route-label" label={$t('ui.pages.groupsPage.labelLabel')}>
			<Input
				id="route-label"
				value={draft.label}
				placeholder={draft.id.trim()}
				oninput={(event) => ondraft({ label: event.currentTarget.value })}
			/>
		</Field>
	</div>

	<div class="flex flex-col gap-2.5">
		<span class="field-label">{$t('ui.pages.groupsPage.strategy')}</span>
		<div
			class="grid grid-cols-2 gap-2.5 min-[760px]:grid-cols-5"
			role="radiogroup"
			aria-label={$t('ui.pages.groupsPage.strategy')}
		>
			{#each STRATEGIES as option (option)}
				<button
					type="button"
					role="radio"
					aria-checked={draft.strategy === option}
					class="flex w-full flex-col gap-2.5 rounded-panel border p-3.5 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring {draft.strategy ===
					option
						? 'border-primary bg-accent-soft ring-[3px] ring-accent-line'
						: 'border-border hover:border-accent-line'}"
					onclick={() => ondraft({ strategy: option })}
				>
					<span
						class="flex size-8.5 items-center justify-center rounded-control transition-colors {draft.strategy ===
						option
							? 'bg-primary text-primary-foreground'
							: 'bg-sunken text-muted-foreground'}"
					>
						<Icon name={orderIcons[option]} size={18} />
					</span>
					<span class="text-[0.8125rem] font-semibold">
						{$t('ui.pages.groupsPage.strategyName.' + option)}
					</span>
					<span class="text-xs leading-snug text-muted-foreground">
						{$t('ui.pages.groupsPage.strategyHint.' + option)}
					</span>
				</button>
			{/each}
		</div>
	</div>

	<OrderingFlow name={draft.id.trim()} strategy={draft.strategy} />

	<div class="grid gap-2.5 md:grid-cols-2">
		<div class="rounded-panel border border-border p-4">
			<FieldSwitch
				id="route-enabled"
				checked={draft.enabled}
				label={$t('ui.pages.groupsPage.enabled')}
				description={$t('ui.pages.groupsPage.enabledHint')}
				onCheckedChange={(checked) => ondraft({ enabled: checked })}
			/>
		</div>
		<div class="rounded-panel border border-border p-4">
			<FieldSwitch
				id="route-listed"
				checked={draft.listed}
				label={$t('ui.pages.groupsPage.listed')}
				description={$t('ui.pages.groupsPage.listedHint')}
				onCheckedChange={(checked) => ondraft({ listed: checked })}
			/>
		</div>
		<div class="rounded-panel border border-border p-4">
			<FieldSwitch
				id="route-switch-on-4xx"
				checked={draft.switchOn4xx}
				label={$t('ui.pages.groupsPage.switchOn4xx')}
				description={$t('ui.pages.groupsPage.switchOn4xxHint')}
				onCheckedChange={(checked) => ondraft({ switchOn4xx: checked })}
			/>
		</div>
		<div class="rounded-panel border border-border p-4">
			<FieldSwitch
				id="route-switch-on-5xx"
				checked={draft.switchOn5xx}
				label={$t('ui.pages.groupsPage.switchOn5xx')}
				description={$t('ui.pages.groupsPage.switchOn5xxHint')}
				onCheckedChange={(checked) => ondraft({ switchOn5xx: checked })}
			/>
		</div>
	</div>
</div>
