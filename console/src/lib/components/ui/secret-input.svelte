<script lang="ts">
	import { t } from 'svelte-i18n';
	import type { HTMLInputAttributes } from 'svelte/elements';

	import Input from '$lib/components/ui/input.svelte';
	import Icon from '$lib/components/ui/icon.svelte';
	import { cn } from '$lib/utils';

	// A field holding a secret: the value is masked until the operator asks to
	// read it, and the control that reveals it sits inside the field so the
	// secret and the way to read it are one thing to the eye and to the tab key.
	// The caller's own type is ignored, because a secret is always masked first.
	interface Props extends Omit<HTMLInputAttributes, 'type'> {
		class?: string;
	}

	let { class: className, value = $bindable(), disabled, ...rest }: Props = $props();

	let revealed = $state(false);
</script>

<div class="relative w-full">
	<Input
		bind:value
		{disabled}
		type={revealed ? 'text' : 'password'}
		class={cn('pr-9', className)}
		{...rest}
	/>
	<button
		type="button"
		class="absolute right-1 top-1/2 flex h-control w-8 -translate-y-1/2 items-center justify-center rounded-control text-muted-foreground outline-none transition-colors hover:text-ink focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
		aria-label={revealed ? $t('ui.common.hide') : $t('ui.common.reveal')}
		aria-pressed={revealed}
		{disabled}
		onclick={() => (revealed = !revealed)}
	>
		<Icon name={revealed ? 'eye-off' : 'eye'} size={14} />
	</button>
</div>
