<script lang="ts">
	import { t } from 'svelte-i18n';
	import Icon from '$lib/components/ui/icon.svelte';

	// OrderingFlow is the illustration under the ordering cards: a request
	// chip, an arrow, three sample members and one sentence about what the
	// chosen rule does. The samples are illustrative, so they read Member
	// A/B/C rather than the draft's real members.
	interface Props {
		name: string;
		strategy: string;
	}

	let { name, strategy }: Props = $props();

	type FlowNode = {
		label: string;
		meta: string;
		// dim is how far a node falls behind the one before it: 0 normal,
		// 1 first fallback, 2 second fallback.
		dim: 0 | 1 | 2;
		// bar is the sample percentage a weighted node draws, else null.
		bar: number | null;
	};

	const nodes = $derived(buildNodes(strategy));

	function member(letter: string): string {
		return $t('ui.pages.groupsPage.flow.member', { values: { letter } });
	}

	function buildNodes(kind: string): FlowNode[] {
		const a = member('A');
		const b = member('B');
		const c = member('C');
		if (kind === 'round-robin') {
			return [
				{ label: a, meta: requestN(1), dim: 0, bar: null },
				{ label: b, meta: requestN(2), dim: 0, bar: null },
				{ label: c, meta: requestN(3), dim: 0, bar: null }
			];
		}
		if (kind === 'weighted') {
			return [
				{ label: a, meta: '60%', dim: 0, bar: 60 },
				{ label: b, meta: '30%', dim: 0, bar: 30 },
				{ label: c, meta: '10%', dim: 0, bar: 10 }
			];
		}
		if (kind === 'cheapest') {
			const prices = ['$0.10', '$2.00', '$10.00'];
			return [
				{ label: a, meta: prices[0], dim: 0, bar: null },
				{ label: b, meta: prices[1], dim: 1, bar: null },
				{ label: c, meta: prices[2], dim: 2, bar: null }
			];
		}
		if (kind === 'fastest') {
			const times = ['180 ms', '320 ms', '540 ms'];
			return [
				{ label: a, meta: times[0], dim: 0, bar: null },
				{ label: b, meta: times[1], dim: 1, bar: null },
				{ label: c, meta: times[2], dim: 2, bar: null }
			];
		}
		return [
			{ label: a, meta: $t('ui.pages.groupsPage.flow.first'), dim: 0, bar: null },
			{
				label: b,
				meta: $t('ui.pages.groupsPage.flow.ifFails', { values: { letter: 'A' } }),
				dim: 1,
				bar: null
			},
			{
				label: c,
				meta: $t('ui.pages.groupsPage.flow.ifFails', { values: { letter: 'B' } }),
				dim: 2,
				bar: null
			}
		];
	}

	function requestN(n: number): string {
		return $t('ui.pages.groupsPage.flow.requestN', { values: { n } });
	}
</script>

<div class="grid items-center gap-5 rounded-panel bg-background p-4.5 md:grid-cols-[auto_1fr]">
	<div class="flex items-center gap-3.5">
		<span
			class="mono-data shrink-0 rounded-control bg-primary px-2.5 py-1.5 text-primary-foreground"
			>{name || 'fast'}</span
		>
		<Icon name="arrow-right" size={20} class="shrink-0 text-primary" />
		<div class="flex min-w-0 flex-1 flex-col gap-1.5 md:min-w-46">
			{#each nodes as node (node.label + node.meta)}
				<div
					class="flex items-center justify-between gap-3 rounded-control border border-border bg-card px-2.5 py-1.5 text-xs {node.dim ===
					1
						? 'opacity-[0.55]'
						: node.dim === 2
							? 'opacity-[0.35]'
							: ''}"
				>
					<span class="truncate">{node.label}</span>
					{#if node.bar !== null}
						<span class="h-1.5 w-16 shrink-0 overflow-hidden rounded-full bg-secondary">
							<span class="block h-full rounded-full bg-primary" style="width: {node.bar}%"></span>
						</span>
					{/if}
					<span class="shrink-0 text-muted-foreground">{node.meta}</span>
				</div>
			{/each}
		</div>
	</div>
	<p class="text-[0.8125rem] leading-relaxed text-muted-foreground">
		{$t('ui.pages.groupsPage.flow.caption.' + strategy)}
	</p>
</div>
