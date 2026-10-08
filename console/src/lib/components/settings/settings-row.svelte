<script lang="ts">
	import type { Snippet } from 'svelte';

	// SettingsRow pairs a label and its helper text with the control it
	// names on the shared settings row grid, so every value, control and
	// action in a card starts on one line. labelFor turns the label into the
	// control's own label, which keeps a switch clickable by its name.
	interface Props {
		label: string;
		hint?: string;
		labelFor?: string;
		children: Snippet;
	}

	let { label, hint = '', labelFor = '', children }: Props = $props();
</script>

<div class="settings-field">
	<div class="min-w-0">
		{#if labelFor}
			<label class="text-sm font-medium leading-snug" for={labelFor}>{label}</label>
		{:else}
			<p class="text-sm font-medium leading-snug">{label}</p>
		{/if}
		{#if hint}
			<p class="mt-1 text-xs leading-relaxed text-muted-foreground">{hint}</p>
		{/if}
	</div>
	<div class="min-w-0">{@render children()}</div>
</div>
