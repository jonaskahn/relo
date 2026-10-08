<script lang="ts">
	import { t } from 'svelte-i18n';

	// ErrorArt is the mark an error page carries: the same doorway, with the
	// detail that says why it did not open. It is drawn the way the console
	// draws everything else — hairline strokes over the node and border tokens
	// and the accent spent once — and the strokes are non-scaling, so the
	// mark stays a hairline at the size the page draws it.
	interface Props {
		kind: 'missing' | 'locked' | 'barred' | 'fault';
	}

	let { kind }: Props = $props();

	const LABEL: Record<string, string> = {
		missing: 'ui.error.artMissing',
		locked: 'ui.error.artLocked',
		barred: 'ui.error.artBarred',
		fault: 'ui.error.artFault'
	};

	// The strokes every kind shares: the frame, the threshold, and the dashed
	// edge of an opening with nothing behind it. One accent stroke per kind
	// says what went wrong.
	const frame = {
		fill: 'none',
		stroke: 'var(--node-ring)',
		'stroke-width': 1.5,
		'vector-effect': 'non-scaling-stroke'
	};
	const opening = {
		fill: 'none',
		stroke: 'var(--border)',
		'stroke-width': 1,
		'stroke-dasharray': '3 5',
		'vector-effect': 'non-scaling-stroke'
	};
	const accent = {
		fill: 'none',
		stroke: 'var(--flow-active)',
		'stroke-width': 1.5,
		'vector-effect': 'non-scaling-stroke'
	};
</script>

<svg
	viewBox="0 0 160 200"
	class="h-56 w-56 shrink-0 sm:h-80 sm:w-80"
	role="img"
	aria-label={$t(LABEL[kind])}
>
	<rect x="34" y="16" width="92" height="152" rx="6" {...frame} />
	<rect x="46" y="28" width="68" height="128" rx="2" {...opening} />
	<line x1="28" y1="172" x2="132" y2="172" {...frame} />
	{#if kind === 'missing'}
		<!-- A door that is there, and nothing to open it onto. -->
		<circle cx="110" cy="92" r="3.5" {...accent} />
	{:else if kind === 'locked'}
		<!-- Locked: the address is real and the session is what is missing. -->
		<path d="M76 92 V84 A7 7 0 0 1 90 84 V92" {...accent} />
		<rect x="70" y="92" width="20" height="16" rx="2" {...accent} />
	{:else if kind === 'barred'}
		<!-- Barred: the page exists and this session is not let through. -->
		<line x1="68" y1="40" x2="68" y2="144" {...accent} />
		<line x1="92" y1="40" x2="92" y2="144" {...accent} />
	{:else}
		<!-- A fault: Relo answered, and what it answered was a failure. -->
		<path d="M80 74 L94 102 H66 Z" stroke-linejoin="round" {...accent} />
		<line x1="80" y1="86" x2="80" y2="94" {...accent} />
		<circle cx="80" cy="98" r="1.2" fill="var(--flow-active)" stroke="none" />
	{/if}
</svg>
