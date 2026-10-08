<script lang="ts">
	import { untrack } from 'svelte';
	import { cubicOut } from 'svelte/easing';
	import { Tween } from 'svelte/motion';
	import { reducedMotion } from '$lib/card-layout-motion';

	// AnimatedNumber counts a displayed figure from its previous value to the
	// next one whenever the source changes, so a window switch reads as motion
	// rather than a jump. The formatting stays the caller's: this component
	// only carries the number.
	interface Props {
		value: number;
		format: (value: number) => string;
		duration?: number;
	}

	let { value, format, duration = 400 }: Props = $props();

	// The initial read is one-time on purpose: later values arrive through the
	// effect below, which is what counts from the old figure to the new one.
	const shown = new Tween(
		untrack(() => value),
		{
			duration: untrack(() => duration),
			easing: cubicOut
		}
	);

	// A reduced-motion visitor gets the new figure at once, with no count.
	$effect(() => {
		void shown.set(value, { duration: reducedMotion() ? 0 : duration });
	});
</script>

{format(shown.current)}
