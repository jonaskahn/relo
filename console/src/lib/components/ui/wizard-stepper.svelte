<script lang="ts" generics="T extends string">
	import { t } from 'svelte-i18n';
	import Icon from '$lib/components/ui/icon.svelte';

	// WizardStepper is the one progress header the group editor and the add
	// connection flow share: numbered circles on filling bars, done steps
	// clickable, the current label alone on a phone.
	interface Props {
		steps: readonly T[];
		current: T;
		labelKeys: Record<T, string>;
		onjump: (step: T) => void;
		stepOfKey?: string;
	}

	let { steps, current, labelKeys, onjump, stepOfKey = '' }: Props = $props();
</script>

<div class="flex items-center gap-2">
	{#each steps as entry, index (entry)}
		{@const isCurrent = entry === current}
		{@const done = steps.indexOf(current) > index}
		<button
			type="button"
			disabled={!done}
			aria-current={isCurrent ? 'step' : undefined}
			class="flex items-center gap-2.5 rounded-control px-0.5 py-0.5 text-left transition-colors focus-visible:outline-none disabled:cursor-default {isCurrent
				? 'text-ink'
				: done
					? 'text-ink hover:text-accent-ink'
					: 'text-muted-foreground/60'}"
			onclick={() => done && onjump(entry)}
		>
			<span
				class="flex size-6.5 shrink-0 items-center justify-center rounded-full border-[1.5px] text-xs font-semibold {isCurrent
					? 'border-primary bg-primary text-primary-foreground'
					: done
						? 'border-primary bg-accent-soft text-accent-ink'
						: 'border-border text-muted-foreground/60'}"
			>
				{#if done}
					<Icon name="check" size={13} />
				{:else}
					{index + 1}
				{/if}
			</span>
			<span class="hidden text-[0.8125rem] font-medium sm:inline">{$t(labelKeys[entry])}</span>
		</button>
		{#if index < steps.length - 1}
			<span class="h-0.5 min-w-4 flex-1 overflow-hidden rounded-full bg-border" aria-hidden="true">
				<span
					class="block h-full origin-left rounded-full bg-primary transition-transform duration-300 {done
						? 'scale-x-100'
						: 'scale-x-0'}"
				></span>
			</span>
		{/if}
	{/each}
	{#if stepOfKey}
		<span class="ms-auto text-xs text-muted-foreground sm:hidden">
			{$t(stepOfKey, {
				values: {
					index: steps.indexOf(current) + 1,
					total: steps.length,
					label: $t(labelKeys[current])
				}
			})}
		</span>
	{/if}
</div>
