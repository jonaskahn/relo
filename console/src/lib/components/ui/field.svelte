<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	import Icon from '$lib/components/ui/icon.svelte';

	// Field is one labelled control: label, control and an optional hint or
	// error. Inside .field-grid the three rows are shared across the row, so
	// inputs line up whatever the length of a hint or the wrap of a label.
	interface Props {
		id: string;
		label: string;
		hint?: string;
		error?: string;
		optional?: boolean;
		class?: string;
		labelAside?: Snippet;
		children: Snippet;
	}

	let {
		id,
		label,
		hint = '',
		error = '',
		optional = false,
		class: className = '',
		labelAside,
		children
	}: Props = $props();
</script>

<div class={cn('field', className)}>
	<label class="field-label" for={id}>
		{label}
		{#if optional}
			<span class="font-normal text-muted-foreground/80">{$t('ui.common.optional')}</span>
		{/if}
		{@render labelAside?.()}
	</label>
	{@render children()}
	{#if error}
		<p class="field-error" id={id + '-error'} role="alert">
			<Icon name="circle-alert" size={12} />
			{error}
		</p>
	{:else if hint}
		<p class="field-hint" id={id + '-hint'}>{hint}</p>
	{/if}
</div>
