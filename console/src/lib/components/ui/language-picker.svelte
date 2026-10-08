<script lang="ts">
	import { tick } from 'svelte';
	import { t } from 'svelte-i18n';

	import { moveRadio } from '$lib/appearance-keyboard';
	import Icon from '$lib/components/ui/icon.svelte';
	import {
		auto,
		languageChoices,
		languageFlags,
		languageNames,
		type LanguageChoice,
		type Locale
	} from '$lib/i18n';
	import { cn } from '$lib/utils';

	interface Props {
		layout?: 'page' | 'popover';
		value: LanguageChoice;
		disabled?: boolean;
		onselect?: (code: LanguageChoice) => void;
	}

	let { layout = 'page', value, disabled = false, onselect }: Props = $props();

	let group: HTMLDivElement | undefined = $state();

	const current = $derived(languageChoices.find((choice) => choice === value) ?? auto);

	function onKeydown(event: KeyboardEvent) {
		if (!group || disabled) return;
		void moveRadio({
			event,
			root: group,
			values: languageChoices,
			select: (code) => onselect?.(code),
			flush: tick
		});
	}

	function isLocale(code: LanguageChoice): code is Locale {
		return code !== auto;
	}
</script>

<div
	bind:this={group}
	class={cn(
		layout === 'page'
			? 'flex flex-wrap gap-2'
			: 'grid max-h-56 grid-cols-5 gap-2 overflow-y-auto pr-0.5',
		disabled && 'pointer-events-none opacity-60'
	)}
	role="radiogroup"
	aria-label={$t(layout === 'page' ? 'ui.settings.language.label' : 'ui.topbar.language')}
	aria-disabled={disabled}
>
	{#each languageChoices as code (code)}
		{@const selected = value === code}
		{@const label = code === auto ? $t('ui.settings.language.auto') : languageNames[code]}
		<button
			type="button"
			role="radio"
			aria-checked={selected}
			aria-label={label}
			title={label}
			tabindex={code === current ? 0 : -1}
			{disabled}
			onkeydown={onKeydown}
			class={cn(
				'relative border-2 transition-colors focus-visible:z-10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none disabled:cursor-not-allowed',
				layout === 'page'
					? 'flex w-[4.75rem] flex-col items-center gap-1.5 rounded-lg px-1 py-1.5'
					: 'flex size-11 items-center justify-center rounded-lg',
				selected ? 'border-foreground bg-muted' : 'border-transparent hover:bg-muted'
			)}
			onclick={() => onselect?.(code)}
		>
			<span
				class={cn(
					'flex items-center justify-center rounded-sm',
					layout === 'page' ? 'size-6 text-lg leading-none' : 'size-6 text-base leading-none'
				)}
				aria-hidden="true"
			>
				{#if code === auto}
					<Icon name="world" size={layout === 'page' ? 18 : 16} class="text-muted-foreground" />
				{:else if isLocale(code)}
					{languageFlags[code]}
				{/if}
			</span>
			{#if layout === 'page'}
				<span
					class={cn(
						'w-full text-center text-[11px] leading-tight',
						selected ? 'font-semibold text-foreground' : 'text-muted-foreground'
					)}
				>
					{label}
				</span>
			{/if}
		</button>
	{/each}
</div>
