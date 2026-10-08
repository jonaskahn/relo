<script lang="ts">
	import { tick } from 'svelte';
	import { t } from 'svelte-i18n';

	import { moveRadio } from '$lib/appearance-keyboard';
	import Icon from '$lib/components/ui/icon.svelte';
	import { consoleState } from '$lib/console-state.svelte';
	import type { ThemeChoice } from '$lib/theme.svelte';
	import { cn } from '$lib/utils';

	interface Props {
		layout?: 'page' | 'popover';
	}

	let { layout = 'page' }: Props = $props();

	let group: HTMLDivElement | undefined = $state();

	const choices = ['system', 'light', 'dark'] as const satisfies readonly ThemeChoice[];
	const labelKey = {
		system: 'ui.topbar.themeSystem',
		light: 'ui.topbar.themeLight',
		dark: 'ui.topbar.themeDark'
	} as const;

	const current = $derived(
		choices.find((choice) => choice === consoleState.settings.theme) ?? 'system'
	);

	function onKeydown(event: KeyboardEvent) {
		if (!group) return;
		void moveRadio({
			event,
			root: group,
			values: choices,
			select: (theme) => consoleState.setTheme(theme),
			flush: tick
		});
	}
</script>

{#snippet window(mode: 'light' | 'dark')}
	<span class="preview-window {mode}">
		<span class="preview-side"></span>
		<span class="preview-lines"><i></i><i></i><b></b></span>
	</span>
{/snippet}

<div
	bind:this={group}
	class={cn('grid grid-cols-3', layout === 'page' ? 'gap-2' : 'gap-2.5')}
	role="radiogroup"
	aria-label={$t(layout === 'page' ? 'ui.settingsPage.themeLabel' : 'ui.topbar.theme')}
>
	{#each choices as choice (choice)}
		{@const selected = consoleState.settings.theme === choice}
		<button
			type="button"
			role="radio"
			aria-checked={selected}
			tabindex={choice === current ? 0 : -1}
			onkeydown={onKeydown}
			class={cn(
				'flex min-w-0 flex-col rounded-panel border-2 text-left transition-colors focus-visible:z-10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
				layout === 'page' ? 'gap-2 p-1.5' : 'gap-1.5 p-1.5',
				selected ? 'border-ink bg-canvas' : 'border-transparent hover:bg-hover'
			)}
			onclick={() => consoleState.setTheme(choice)}
		>
			<span
				class={cn(
					'relative block overflow-hidden rounded-control border border-line',
					layout === 'page' ? 'h-14' : 'h-[54px]'
				)}
				aria-hidden="true"
			>
				{#if choice === 'system'}
					<span class="absolute inset-y-0 left-0 w-1/2">{@render window('light')}</span>
					<span class="absolute inset-y-0 right-0 w-1/2">{@render window('dark')}</span>
					<span class="absolute inset-y-0 left-1/2 z-10 w-px bg-line"></span>
				{:else}
					{@render window(choice === 'dark' ? 'dark' : 'light')}
				{/if}
			</span>
			<span
				class={cn(
					'flex items-center justify-center gap-1 text-center text-xs leading-tight',
					selected ? 'font-semibold text-ink' : 'text-muted-foreground'
				)}
			>
				{#if selected}
					<Icon name="check" size={12} class="shrink-0" />
				{/if}
				{$t(labelKey[choice])}
			</span>
		</button>
	{/each}
</div>

<style>
	.preview-window {
		display: flex;
		height: 100%;
		width: 100%;
		gap: 6px;
		padding: 8px;
	}
	.preview-window.light {
		background: var(--preview-light);
	}
	.preview-window.dark {
		background: var(--preview-dark);
	}
	.preview-side {
		width: 8px;
		flex: none;
		border-radius: 3px;
	}
	.preview-lines {
		display: flex;
		flex: 1;
		flex-direction: column;
		gap: 4px;
	}
	.preview-lines i {
		height: 4px;
		border-radius: 2px;
	}
	.preview-lines b {
		height: 6px;
		width: 16px;
		margin-top: auto;
		align-self: flex-end;
		border-radius: 3px;
		background: var(--accent);
	}
	.light .preview-side,
	.light .preview-lines i {
		background: var(--preview-light-line);
	}
	.light .preview-lines i + i {
		opacity: 0.5;
	}
	.dark .preview-side,
	.dark .preview-lines i {
		background: var(--preview-dark-line);
	}
	.dark .preview-lines i + i {
		opacity: 0.5;
	}
</style>
