<script lang="ts">
	import { t } from 'svelte-i18n';

	import Icon from '$lib/components/ui/icon.svelte';
	import Button from '$lib/components/ui/button.svelte';

	// ChatComposer is the one place a prompt is written: a surface card at the
	// foot of the transcript with the send button in its action row. Enter
	// sends and Shift+Enter opens a line, which is the pair a coding console
	// trains its operators to expect. While the answer to this conversation is
	// on its way the same slot stops it, because the wait is the one moment the
	// request is worth cancelling. The target chip beside the button names
	// what a send would call and opens the panel that changes it.
	interface Props {
		value: string;
		disabled?: boolean;
		// activity names what is in flight: nothing, the conversation on
		// screen, or another one. The two in-flight answers differ here — a
		// stop belongs to the conversation being answered, a disabled send
		// does not care which one is.
		activity?: 'idle' | 'this' | 'other';
		sendable?: boolean;
		placeholder: string;
		// targetTitle and targetDetail name the model or route a send calls.
		targetTitle?: string;
		targetDetail?: string;
		// locked is whether this conversation keeps the target it was tested with.
		locked?: boolean;
		onsend: () => void;
		onstop?: () => void;
		ontarget?: () => void;
	}

	let {
		value = $bindable(''),
		disabled = false,
		activity = 'idle',
		sendable = true,
		placeholder,
		targetTitle = '',
		targetDetail = '',
		locked = false,
		onsend,
		onstop,
		ontarget
	}: Props = $props();

	let field = $state<HTMLTextAreaElement | null>(null);

	// The field is one line tall until the text wraps, then follows it up to a
	// cap. The cap is what keeps a long draft from covering the answer above it.
	const FIELD_MAX = 200;

	const stopping = $derived(activity === 'this' && onstop !== undefined);
	const primaryDisabled = $derived(
		disabled || (stopping ? false : !sendable || activity !== 'idle')
	);
	const targetLabel = $derived(
		targetTitle +
			(targetDetail === '' ? '' : ' · ' + targetDetail) +
			(locked ? ' · ' + $t('ui.pages.chatPage.locked') : '')
	);

	// focus puts the caret in the field, which is where a new conversation and
	// a conversation just switched to both start.
	export function focus() {
		field?.focus();
	}

	function grow() {
		if (field === null) return;
		field.style.height = 'auto';
		field.style.height = Math.min(field.scrollHeight, FIELD_MAX) + 'px';
	}

	// The stored draft is the truth: a conversation switch, a sent message and
	// a suggestion all change the value, and the field follows each one.
	$effect(() => {
		void value;
		grow();
	});

	function onkeydown(event: KeyboardEvent) {
		if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
		event.preventDefault();
		if (!sendable || disabled || activity !== 'idle') return;
		onsend();
	}
</script>

<div
	class="section-surface flat-surface p-2 transition-[border-color,box-shadow] duration-150 focus-within:border-accent-border focus-within:shadow-[var(--shadow-hover)]"
>
	<textarea
		bind:this={field}
		bind:value
		rows="1"
		{placeholder}
		{disabled}
		title={$t('ui.pages.chatPage.composerHint')}
		class="max-h-[12.5rem] min-h-[2.75rem] w-full resize-none bg-transparent px-2 py-2 text-[15px] leading-6 text-ink outline-none placeholder:text-faint focus-visible:shadow-none disabled:cursor-not-allowed disabled:opacity-60"
		oninput={grow}
		{onkeydown}></textarea>
	<div class="flex items-center gap-2 px-1 pb-1">
		<Button
			variant="ghost"
			class="min-w-0 max-w-[60%] gap-1.5 px-2 font-normal text-muted-foreground"
			title={$t('ui.pages.chatPage.showSettings')}
			aria-label={targetLabel === ''
				? $t('ui.pages.chatPage.showSettings')
				: targetLabel + ' · ' + $t('ui.pages.chatPage.showSettings')}
			onclick={() => ontarget?.()}
		>
			<span class="truncate">{targetTitle}</span>
			{#if targetDetail !== ''}
				<span class="hidden truncate text-faint sm:inline">· {targetDetail}</span>
			{/if}
			{#if locked}
				<Icon name="lock" size={12} class="shrink-0" />
			{/if}
			<Icon name="chevron-down" size={14} class="shrink-0" />
		</Button>
		<div class="flex-1"></div>
		{#if stopping}
			<Button variant="outline" {disabled} onclick={() => onstop?.()}>
				<Icon name="player-stop" size={16} />
				{$t('ui.pages.chatPage.stop')}
			</Button>
		{:else}
			<Button disabled={primaryDisabled} onclick={onsend}>
				<Icon name="arrow-up" size={16} />
				{$t('ui.pages.chatPage.send')}
			</Button>
		{/if}
	</div>
</div>
