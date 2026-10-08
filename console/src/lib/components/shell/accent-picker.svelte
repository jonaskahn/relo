<script lang="ts">
	import { tick } from 'svelte';
	import { t } from 'svelte-i18n';

	import { moveRadio } from '$lib/appearance-keyboard';
	import Icon from '$lib/components/ui/icon.svelte';
	import { consoleState } from '$lib/console-state.svelte';
	import { ACCENTS } from '$lib/theme.svelte';
	import { cn } from '$lib/utils';

	interface Props {
		layout?: 'page' | 'popover';
	}

	let { layout = 'page' }: Props = $props();

	let group: HTMLDivElement | undefined = $state();

	const current = $derived(
		ACCENTS.find((accent) => accent === consoleState.settings.accent) ?? ACCENTS[0]
	);

	function onKeydown(event: KeyboardEvent) {
		if (!group) return;
		void moveRadio({
			event,
			root: group,
			values: ACCENTS,
			select: (accent) => consoleState.setAccent(accent),
			flush: tick
		});
	}
</script>

<div>
	<div
		bind:this={group}
		class={layout === 'page' ? 'flex flex-wrap gap-2' : 'grid grid-cols-5 gap-2'}
		role="radiogroup"
		aria-label={$t(layout === 'page' ? 'ui.settingsPage.accentLabel' : 'ui.topbar.accent')}
	>
		{#each ACCENTS as accent (accent)}
			{@const selected = consoleState.settings.accent === accent}
			<!-- data-accent scopes --accent-base and --on-accent to this swatch, so
			     every choice shows its light-theme base in both themes. -->
			<button
				type="button"
				role="radio"
				aria-checked={selected}
				aria-label={$t('ui.accent.' + accent)}
				title={$t('ui.accent.' + accent)}
				tabindex={accent === current ? 0 : -1}
				data-accent={accent}
				onkeydown={onKeydown}
				class={cn(
					'relative transition-colors focus-visible:z-10 focus-visible:outline-none',
					layout === 'page'
						? 'flex w-[4.75rem] flex-col items-center gap-1.5 rounded-panel border-2 px-1 py-1.5'
						: 'flex size-11 items-center justify-center rounded-panel',
					layout === 'page'
						? selected
							? 'border-ink bg-canvas'
							: 'border-transparent hover:bg-hover'
						: selected
							? 'shadow-[0_0_0_3px_var(--surface),0_0_0_5px_var(--ink)]'
							: 'hover:bg-hover'
				)}
				onclick={() => consoleState.setAccent(accent)}
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
						{$t('ui.accent.' + accent)}
					</span>
				{/if}
			</button>
		{/each}
	</div>
</div>
