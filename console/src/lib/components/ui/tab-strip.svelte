<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { spring } from 'svelte/motion';
	import { Tabs } from 'bits-ui';

	import { cn } from '$lib/utils';
	import { prefersReducedMotion, TAB_SPRING, thumbTarget } from '$lib/tab-motion';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';

	// TabStrip is the tablist every content-pane tab strip shares: the same
	// card-like strip and sliding thumb as the segmented control, with the
	// triggers delegating selection to the Tabs root above them.
	export type TabStripTab = {
		value: string;
		label: string;
		icon?: IconName;
		// count names a total beside the label, such as a connection's model
		// count. A negative count hides the figure.
		count?: number;
	};

	interface Props {
		tabs: TabStripTab[];
		value: string;
		ariaLabel: string;
		class?: string;
	}

	let { tabs, value, ariaLabel, class: className = '' }: Props = $props();

	// The thumb rides the same spring as the segmented control from the
	// active trigger's measured offset. Reduced motion parks it hard.
	let list: HTMLDivElement | null = $state(null);
	let thumbReady = $state(false);
	let reduced = $state(false);
	let measuredKey = '';
	const thumb = spring({ left: 0, width: 0 }, TAB_SPRING);

	function measure(hard: boolean, key = '') {
		// A key that has not changed means the geometry has not moved, so the
		// thumb is already where it belongs.
		if (!hard && measuredKey === key) return;
		measuredKey = key;
		const root = list;
		if (!root) return;
		const active = root.querySelector<HTMLButtonElement>('[role="tab"][data-state="active"]');
		if (!active) return;
		const target = thumbTarget([{ left: active.offsetLeft, width: active.offsetWidth }], 0);
		if (hard) {
			// The store types expose no hard set, so a full spring parks the
			// thumb instantly instead.
			const { stiffness, damping } = thumb;
			thumb.stiffness = 1;
			thumb.damping = 1;
			void thumb.set(target);
			thumb.stiffness = stiffness;
			thumb.damping = damping;
		} else {
			void thumb.set(target);
		}
		thumbReady = true;
		// A strip that overflows on a phone brings the active tab back into
		// view; a strip that fits never scrolls anything.
		if (root.scrollWidth > root.clientWidth) {
			active.scrollIntoView({
				block: 'nearest',
				inline: 'nearest',
				behavior: reduced ? 'auto' : 'smooth'
			});
		}
	}

	onMount(() => {
		reduced = prefersReducedMotion();
		if (reduced) {
			thumb.stiffness = 1;
			thumb.damping = 1;
		}
		measure(true);
		const onResize = () => measure(true);
		window.addEventListener('resize', onResize);
		return () => window.removeEventListener('resize', onResize);
	});

	// A new value or a new locale re-measures after paint, so the thumb sits
	// under the translated label. The key names exactly what moves the thumb;
	// measure() reads it so the effect subscribes to it.
	const thumbKey = $derived(`${value} ${tabs.map((tab) => tab.label).join(' ')}`);

	$effect(() => {
		const key = thumbKey;
		void tick().then(() => measure(false, key));
	});
</script>

<Tabs.List class={cn('tab-track', className)} aria-label={ariaLabel} bind:ref={list}>
	<div
		class="tab-thumb"
		aria-hidden="true"
		style="left: {$thumb.left}px; width: {$thumb.width}px; opacity: {thumbReady ? 1 : 0};"
	></div>
	{#each tabs as entry (entry.value)}
		<Tabs.Trigger value={entry.value} class="tab-seg">
			{#if entry.icon}<Icon name={entry.icon} size={14} />{/if}
			{entry.label}{#if entry.count !== undefined && entry.count >= 0}<span
					class="ms-1.5 text-xs text-muted-foreground">{entry.count}</span
				>{/if}
		</Tabs.Trigger>
	{/each}
</Tabs.List>
