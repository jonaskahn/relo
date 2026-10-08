<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import Input from '$lib/components/ui/input.svelte';

	// RetryWaitsEditor edits the three low–high waits between retries. It is
	// presentational: the card that holds it owns the draft and the save.
	interface Props {
		windows: [number, number][];
		onchange?: (next: [number, number][]) => void;
		disabled?: boolean;
		invalid?: boolean;
	}

	let { windows, onchange, disabled = false, invalid = false }: Props = $props();

	// A number input the operator has emptied has no number in it, and a wait
	// of "not a number" is what makes the row read invalid rather than silently
	// becoming a zero-second wait.
	function readWait(raw: string): number {
		return raw.trim() === '' ? Number.NaN : Number(raw);
	}

	function setWait(index: number, slot: 0 | 1, raw: string): void {
		const value = readWait(raw);
		onchange?.(
			windows.map((pair, at) => {
				if (at !== index) return pair;
				const updated: [number, number] = [pair[0], pair[1]];
				updated[slot] = value;
				return updated;
			})
		);
	}

	const labels = [
		'ui.settingsPage.retryFirst',
		'ui.settingsPage.retrySecond',
		'ui.settingsPage.retryThird'
	];
</script>

<div class="flex flex-col gap-2">
	{#each windows as window, index (labels[index])}
		{@const label = labels[index]}
		<div class="flex flex-wrap items-center gap-2">
			<span class="w-full text-sm text-muted-foreground sm:w-28">{$t(label)}</span>
			<Input
				type="number"
				min="0"
				max="600"
				class="w-20 max-sm:h-11"
				value={window[0]}
				oninput={(event) => setWait(index, 0, event.currentTarget.value)}
				{disabled}
				aria-label={$t('ui.settingsPage.retryLow') + ' · ' + $t(label)}
			/>
			<span class="text-sm text-muted-foreground">–</span>
			<Input
				type="number"
				min="0"
				max="600"
				class="w-20 max-sm:h-11"
				value={window[1]}
				oninput={(event) => setWait(index, 1, event.currentTarget.value)}
				{disabled}
				aria-label={$t('ui.settingsPage.retryHigh') + ' · ' + $t(label)}
			/>
			<span class="text-sm text-muted-foreground">s</span>
		</div>
	{/each}
	{#if invalid}
		<p class="field-error" role="alert">
			<Icon name="circle-alert" size={12} />
			{$t('ui.settingsPage.retryInvalid')}
		</p>
	{/if}
</div>
