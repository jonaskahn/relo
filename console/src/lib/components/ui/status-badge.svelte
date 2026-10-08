<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from './icon.svelte';
	import { cn } from '$lib/utils';

	// The status pill: a full-radius chip carrying a word, a tone fill and a
	// shape cue, so state never rests on color. Warn and danger carry an
	// icon; the settled states carry a dot.
	interface Props {
		kind: 'pass' | 'warn' | 'fail' | 'muted';
		label?: string;
		class?: string;
	}

	let { kind, label, class: className }: Props = $props();

	const tone: Record<string, string> = {
		pass: 'bg-ok/10 text-ok',
		warn: 'bg-warn/10 text-warn',
		fail: 'bg-danger/10 text-danger',
		muted: 'bg-sunken text-muted-foreground'
	};
</script>

<span
	class={cn(
		'inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium',
		tone[kind],
		className
	)}
>
	{#if kind === 'warn' || kind === 'fail'}
		<Icon name={kind === 'fail' ? 'circle-x' : 'alert-triangle'} size={12} class="shrink-0" />
	{:else}
		<span aria-hidden="true" class="size-1.5 rounded-full bg-current"></span>
	{/if}
	{label ?? $t('ui.status.badge.' + kind)}
</span>
