<script lang="ts">
	import { tick } from 'svelte';
	import { t } from 'svelte-i18n';

	import { moveRadio } from '$lib/appearance-keyboard';
	import ColorRadar from '$lib/components/shell/color-radar.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { consoleState } from '$lib/console-state.svelte';
	import { isHexColour } from '$lib/color';
	import { ACCENTS } from '$lib/theme.svelte';
	import { cn } from '$lib/utils';

	interface Props {
		layout?: 'page' | 'popover';
	}

	let { layout = 'page' }: Props = $props();

	/** The board's 20 slots: the 19 presets in order, then the colour the radar picked. */
	const slots = [...ACCENTS, 'custom'] as const;
	type Slot = (typeof slots)[number];

	let group: HTMLDivElement | undefined = $state();
	let radarOpen = $state(false);

	const current = $derived<Slot>(
		isHexColour(consoleState.settings.accent) ? 'custom' : consoleState.settings.accent
	);

	function applySlot(slot: Slot) {
		void consoleState.setAccent(slot === 'custom' ? consoleState.settings.customAccent : slot);
	}

	/** A radio step only changes the accent: moving across the board by keyboard
	 *  must not open and shut the radar on the way past its slot. */
	function choose(slot: Slot) {
		if (slot !== 'custom') radarOpen = false;
		applySlot(slot);
	}

	/** The radar lives in Settings alone, where there is room for it, and opens on
	 *  the slot's own click beside the colour it applies. */
	function clickSlot(slot: Slot) {
		if (slot === 'custom') radarOpen = layout === 'page' ? !radarOpen : false;
		applySlot(slot);
	}

	function onKeydown(event: KeyboardEvent) {
		if (!group) return;
		void moveRadio({ event, root: group, values: slots, select: choose, flush: tick });
	}
</script>

<div>
	<div
		bind:this={group}
		class={layout === 'page' ? 'flex flex-wrap gap-2' : 'grid grid-cols-5 gap-2'}
		role="radiogroup"
		aria-label={$t(layout === 'page' ? 'ui.settingsPage.accentLabel' : 'ui.topbar.accent')}
	>
		{#each slots as slot (slot)}
			{@const selected = current === slot}
			{@const label = $t('ui.accent.' + slot)}
			<!-- data-accent scopes the slot's own colour, so every choice shows its light
			     theme base in both themes, and the radar's colour carries no hex here. -->
			<button
				type="button"
				role="radio"
				aria-checked={selected}
				aria-label={label}
				aria-controls={slot === 'custom' && radarOpen ? 'accent-radar' : undefined}
				title={label}
				tabindex={slot === current ? 0 : -1}
				data-accent={slot}
				onkeydown={onKeydown}
				onclick={() => clickSlot(slot)}
				class={cn(
					'relative transition-colors focus-visible:z-10 focus-visible:outline-none',
					layout === 'page'
						? 'flex w-[4.75rem] flex-col items-center gap-1.5 rounded-panel border-2 px-1 py-1.5'
						: 'flex size-11 items-center justify-center rounded-panel',
					layout === 'page'
						? selected
							? 'border-accent-border bg-accent-soft'
							: 'border-transparent hover:bg-hover'
						: selected
							? 'bg-accent-soft ring-1 ring-accent-border'
							: 'hover:bg-hover'
				)}
			>
				<span class="flex size-6 items-center justify-center rounded-sm bg-[var(--accent-base)]">
					{#if selected}
						<Icon name="check" size={14} class="text-[var(--on-accent)]" />
					{/if}
				</span>
				{#if layout === 'page'}
					<span
						class={cn(
							'w-full text-center text-[0.6875rem] leading-tight',
							selected ? 'font-semibold text-ink' : 'text-muted-foreground'
						)}
					>
						{label}
					</span>
				{/if}
			</button>
		{/each}
	</div>

	{#if layout === 'page' && radarOpen}
		<ColorRadar
			value={consoleState.settings.customAccent}
			onchange={(colour) => void consoleState.setAccent(colour)}
		/>
	{/if}
</div>
