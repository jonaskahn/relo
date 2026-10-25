<script lang="ts">
	import { t } from 'svelte-i18n';
	import { laneCounts } from '$lib/group-suggestions';
	import type { Provider } from '$lib/types';
	import type { RouteDraft } from '$lib/route-stepper';
	import Button from '$lib/components/ui/button.svelte';
	import Icon from '$lib/components/ui/icon.svelte';

	interface Props {
		draft: RouteDraft;
		configured: Provider[];
		ondraft: (patch: Partial<RouteDraft>) => void;
		onskip: () => void;
	}

	let { draft, configured, ondraft, onskip }: Props = $props();

	const lanes = $derived(draft.lanes);
	const counts = $derived(laneCounts(configured));

	const paygCount = $derived(counts.payg);
	const planCount = $derived(counts.plan);
	const freeCount = $derived(counts.free);
	const manualCount = $derived(configured.length);

	function hasLane(lane: 'payg' | 'plan' | 'free' | 'manual'): boolean {
		return lanes.includes(lane);
	}

	function toggleLane(lane: 'payg' | 'plan' | 'free' | 'manual') {
		if (lane === 'manual') {
			ondraft({ lanes: [] });
			return;
		}
		const next = new Set(
			lanes.filter((entry: 'payg' | 'plan' | 'free' | 'manual') => entry !== 'manual')
		);
		if (next.has(lane)) next.delete(lane);
		else next.add(lane);
		if (next.size === 0) ondraft({ lanes: [] });
		else ondraft({ lanes: [...next] });
	}
</script>

<div class="flex flex-col gap-6">
	<div>
		<h3 class="text-sm font-semibold">{$t('ui.pages.groupsPage.billingTitle')}</h3>
		<p class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.billingHint')}</p>
	</div>
	<div class="grid gap-2.5 md:grid-cols-2">
		<button
			type="button"
			class="flex flex-col gap-2 rounded-panel border p-4 text-left transition-colors {hasLane(
				'payg'
			)
				? 'border-primary bg-accent-soft'
				: 'border-border hover:border-accent-line'}"
			aria-pressed={hasLane('payg')}
			disabled={paygCount === 0}
			onclick={() => toggleLane('payg')}
		>
			<div class="flex items-center gap-2">
				<Icon name="currency-dollar" size={16} />
				<span class="text-sm font-semibold">{$t('ui.pages.providersPage.list.tagPayAsYouGo')}</span>
				<span class="ms-auto mono-data text-xs text-muted-foreground">{paygCount}</span>
			</div>
			<span class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.billingLanePayg')}</span>
		</button>
		<button
			type="button"
			class="flex flex-col gap-2 rounded-panel border p-4 text-left transition-colors {hasLane(
				'plan'
			)
				? 'border-primary bg-accent-soft'
				: 'border-border hover:border-accent-line'}"
			aria-pressed={hasLane('plan')}
			disabled={planCount === 0}
			onclick={() => toggleLane('plan')}
		>
			<div class="flex items-center gap-2">
				<Icon name="layers" size={16} />
				<span class="text-sm font-semibold">{$t('ui.pages.providersPage.list.tagPaidPlan')}</span>
				<span class="ms-auto mono-data text-xs text-muted-foreground">{planCount}</span>
			</div>
			<span class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.billingLanePlan')}</span>
		</button>
		<button
			type="button"
			class="flex flex-col gap-2 rounded-panel border p-4 text-left transition-colors {hasLane(
				'free'
			)
				? 'border-primary bg-accent-soft'
				: 'border-border hover:border-accent-line'}"
			aria-pressed={hasLane('free')}
			disabled={freeCount === 0}
			onclick={() => toggleLane('free')}
		>
			<div class="flex items-center gap-2">
				<Icon name="sparkles" size={16} />
				<span class="text-sm font-semibold">{$t('ui.pages.providersPage.list.tagFree')}</span>
				<span class="ms-auto mono-data text-xs text-muted-foreground">{freeCount}</span>
			</div>
			<span class="text-xs text-muted-foreground">{$t('ui.pages.groupsPage.billingLaneFree')}</span>
		</button>
		<button
			type="button"
			class="flex flex-col gap-2 rounded-panel border p-4 text-left transition-colors {hasLane(
				'manual'
			)
				? 'border-primary bg-accent-soft'
				: 'border-border hover:border-accent-line'}"
			aria-pressed={hasLane('manual')}
			onclick={() => toggleLane('manual')}
		>
			<div class="flex items-center gap-2">
				<Icon name="settings" size={16} />
				<span class="text-sm font-semibold">{$t('ui.pages.groupsPage.laneManual')}</span>
				<span class="ms-auto mono-data text-xs text-muted-foreground">{manualCount}</span>
			</div>
			<span class="text-xs text-muted-foreground"
				>{$t('ui.pages.groupsPage.billingLaneManual')}</span
			>
		</button>
	</div>
	<div class="flex justify-end">
		<Button variant="ghost" size="sm" onclick={onskip}>
			{$t('ui.pages.groupsPage.skipBilling')}
		</Button>
	</div>
</div>
