<script lang="ts" generics="T extends string">
	import { onMount, tick } from 'svelte';
	import { spring } from 'svelte/motion';

	import { cn } from '$lib/utils';
	import { prefersReducedMotion, TAB_SPRING, thumbTarget } from '$lib/tab-motion';
	import Icon, { type IconName } from '$lib/components/ui/icon.svelte';

	// Segmented is the one control for a small exclusive choice, such as the
	// All / On / Off status filter: a card-like strip with a thumb that
	// slides under the active segment, with the arrow keys moving the
	// selection like a native radio group.
	type SegmentOption = {
		value: T;
		label: string;
		icon?: IconName;
		count?: number;
	};

	interface Props {
		value: T;
		options: SegmentOption[];
		ariaLabel: string;
		class?: string;
		onchange?: (value: T) => void;
		// iconOnly collapses each option to its icon; the label stays as the
		// radio's accessible name and tooltip.
		iconOnly?: boolean;
		// disabled freezes a choice that belongs to something no longer being
		// edited, such as a conversation that has already sent a turn.
		disabled?: boolean;
	}

	let {
		value = $bindable(),
		options,
		ariaLabel,
		class: className = '',
		onchange,
		iconOnly = false,
		disabled = false
	}: Props = $props();

	// The thumb rides a spring between measured offsets, so it glides when the
	// choice changes. Reduced motion parks it with a hard set instead.
	let track: HTMLDivElement | null = $state(null);
	let thumbReady = $state(false);
	let reduced = $state(false);
	let measuredKey = '';
	const thumb = spring({ left: 0, width: 0 }, TAB_SPRING);

	function measure(hard: boolean, key = '') {
		// A key that has not changed means the geometry has not moved, so the
		// thumb is already where it belongs.
		if (!hard && measuredKey === key) return;
		measuredKey = key;
		const root = track;
		if (!root) return;
		const active = root.querySelector<HTMLButtonElement>('[role="radio"][data-active="true"]');
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
		// A strip that overflows on a phone brings the active choice back into
		// view; a strip that fits never scrolls anything.
		if (root.scrollWidth > root.clientWidth) {
			active.scrollIntoView({
				block: 'nearest',
				inline: 'nearest',
				behavior: reduced ? 'auto' : 'smooth'
			});
		}
	}

	function select(next: T) {
		value = next;
		onchange?.(next);
	}

	function handleKeydown(event: KeyboardEvent, index: number) {
		if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
		event.preventDefault();
		const step = event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 1;
		const next = options[(index + step + options.length) % options.length];
		if (!next) return;
		select(next.value);
		const target = event.currentTarget instanceof HTMLElement ? event.currentTarget : null;
		target?.parentElement
			?.querySelectorAll<HTMLButtonElement>('[role="radio"]')
			[options.indexOf(next)]?.focus();
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

	// A new value, a new option list, or a new locale re-measures after paint,
	// so the thumb sits under the translated label. The key names exactly what
	// moves the thumb; measure() reads it so the effect subscribes to it.
	const thumbKey = $derived(`${String(value)} ${options.map((option) => option.label).join(' ')}`);

	$effect(() => {
		const key = thumbKey;
		void tick().then(() => measure(false, key));
	});
</script>

<div
	class={cn('tab-track shrink-0', className)}
	role="radiogroup"
	aria-label={ariaLabel}
	bind:this={track}
>
	<div
		class="tab-thumb"
		aria-hidden="true"
		style="left: {$thumb.left}px; width: {$thumb.width}px; opacity: {thumbReady ? 1 : 0};"
	></div>
	{#each options as option, index (option.value)}
		{@const active = value === option.value}
		<button
			type="button"
			role="radio"
			aria-checked={active}
			data-active={active}
			tabindex={active ? 0 : -1}
			aria-label={iconOnly ? option.label : undefined}
			title={iconOnly ? option.label : undefined}
			{disabled}
			class={cn('tab-seg', iconOnly && 'px-2')}
			onclick={() => select(option.value)}
			onkeydown={(event) => handleKeydown(event, index)}
		>
			{#if option.icon}<Icon name={option.icon} size={13} />{/if}
			{#if !iconOnly}{option.label}{/if}
			{#if option.count !== undefined}
				<span class="font-mono text-[0.65rem] opacity-70">{option.count}</span>
			{/if}
		</button>
	{/each}
</div>
