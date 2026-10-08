<script lang="ts">
	import { onMount } from 'svelte';
	import { t } from 'svelte-i18n';
	import { toast } from 'svelte-sonner';

	import {
		retryableTurn,
		type ChatThread,
		type ChatTurn,
		type ChatTurnStatus
	} from '$lib/chat-threads';
	import Icon from '$lib/components/ui/icon.svelte';
	import StatusBadge from '$lib/components/ui/status-badge.svelte';
	import IconAction from '$lib/components/ui/icon-action.svelte';
	import { formatDuration, formatTokens } from '$lib/format';
	import { parseMarkdown } from '$lib/chat-markdown';
	import ChatMarkdown from './chat-markdown.svelte';

	// ChatTranscript is the conversation itself: one turn row-card per
	// exchange, the prompt in a sunken inset and the answer under it, with the
	// turn's state as a pill in the head. A turn that failed or stopped keeps
	// its prompt on screen with a retry, so a failure costs a look rather than
	// a retype.
	interface Props {
		thread: ChatThread | null;
		sending: boolean;
		onretry: () => void;
	}

	let { thread, sending, onretry }: Props = $props();

	let copiedId = $state('');
	let timer: ReturnType<typeof setTimeout> | undefined;

	onMount(() => () => clearTimeout(timer));

	const turns = $derived(thread === null ? [] : thread.turns);
	const retryId = $derived(thread === null ? '' : (retryableTurn(thread)?.id ?? ''));

	// Copy takes what the model wrote, not what the page drew: the text is
	// what a reader would paste into an editor.
	async function copy(id: string, text: string) {
		try {
			await navigator.clipboard.writeText(text);
			copiedId = id;
			clearTimeout(timer);
			timer = setTimeout(() => (copiedId = ''), 1500);
		} catch {
			toast.error($t('ui.common.error'));
		}
	}

	// The pill is the turn's state read at a glance; the tone never carries
	// the meaning alone, the word does.
	function statusKind(status: ChatTurnStatus): 'pass' | 'warn' | 'fail' | 'muted' {
		if (status === 'failed') return 'fail';
		if (status === 'interrupted') return 'warn';
		if (status === 'pending') return 'muted';
		return 'pass';
	}

	function statusLabel(status: ChatTurnStatus): string {
		if (status === 'pending') return $t('ui.pages.chatPage.statusReceiving');
		if (status === 'failed') return $t('ui.pages.chatPage.statusFailed');
		if (status === 'interrupted') return $t('ui.pages.chatPage.statusInterrupted');
		return $t('ui.pages.chatPage.statusAnswered');
	}

	function labelOf(turn: ChatTurn): string {
		const model = turn.result.model;
		return model !== undefined && model !== '' ? model : $t('ui.pages.chatPage.assistant');
	}

	// The model stays out when the label above already names it.
	function metaOf(turn: ChatTurn): string {
		const parts: string[] = [];
		const identity = [turn.result.provider, turn.result.model]
			.filter((part): part is string => part !== undefined && part !== '')
			.join(' · ');
		if (identity !== '' && identity !== turn.result.model) parts.push(identity);
		if (turn.result.requestId) {
			parts.push($t('ui.pages.chatPage.request', { values: { id: turn.result.requestId } }));
		}
		if (turn.result.inputTokens !== undefined || turn.result.outputTokens !== undefined) {
			parts.push(
				$t('ui.pages.chatPage.tokens', {
					values: {
						input: formatTokens(turn.result.inputTokens ?? 0),
						output: formatTokens(turn.result.outputTokens ?? 0)
					}
				})
			);
		}
		if (turn.result.durationMs !== undefined) parts.push(formatDuration(turn.result.durationMs));
		// The status is what tells a refused turn apart from a dropped one, so
		// it is shown where a failure is already being read.
		if (turn.result.status === 'failed' && turn.result.httpStatus !== undefined) {
			parts.push('HTTP ' + turn.result.httpStatus);
		}
		if (turn.result.errorCode) parts.push(turn.result.errorCode);
		return parts.join(' · ');
	}
</script>

<ol class="mx-auto flex w-full max-w-3xl flex-col gap-6">
	{#each turns as turn (turn.id)}
		{@const canRetry = turn.id === retryId}
		{@const meta = metaOf(turn)}
		<li class="section-surface p-[var(--card-pad)]">
			<div class="flex items-center justify-between gap-3">
				<h3 class="text-xs font-medium text-faint">{$t('ui.pages.chatPage.you')}</h3>
				<StatusBadge
					kind={statusKind(turn.result.status)}
					label={statusLabel(turn.result.status)}
				/>
			</div>

			<div class="mt-3 rounded-panel bg-sunken px-3.5 py-3">
				<p class="text-sm leading-6 break-words whitespace-pre-wrap text-ink">{turn.prompt}</p>
				{#if canRetry}
					<div class="mt-1 flex justify-end">
						<IconAction
							icon="refresh"
							label={$t('ui.pages.chatPage.retry')}
							disabled={sending}
							onclick={onretry}
							class="rounded-none hover:bg-transparent"
						/>
					</div>
				{/if}
			</div>

			<div class="mt-4">
				<h3 class="sr-only">{labelOf(turn)}</h3>
				{#if turn.result.status === 'pending'}
					{#if turn.result.text !== ''}
						<p class="text-[15px] leading-7 break-words whitespace-pre-wrap">{turn.result.text}</p>
					{/if}
				{:else if turn.result.status === 'failed'}
					<div
						class="flex items-start gap-2 rounded-panel border border-danger/30 bg-danger/5 px-3 py-2.5 text-sm text-danger"
						role="alert"
					>
						<Icon name="circle-alert" size={15} class="mt-0.5 shrink-0" />
						<p class="min-w-0 flex-1 break-words">
							{turn.result.error || $t('ui.pages.chatPage.failed')}
						</p>
					</div>
				{:else if turn.result.status === 'interrupted'}
					{#if turn.result.text !== ''}
						<ChatMarkdown blocks={parseMarkdown(turn.result.text)} />
					{:else}
						<p class="text-sm text-muted-foreground">{$t('ui.pages.chatPage.interrupted')}</p>
					{/if}
				{:else if turn.result.status === 'empty'}
					<p class="text-sm text-muted-foreground">{$t('ui.pages.chatPage.noText')}</p>
				{:else}
					<ChatMarkdown blocks={parseMarkdown(turn.result.text)} />
				{/if}
				{#if turn.result.text !== '' && turn.result.status !== 'pending'}
					<div class="mt-2 flex justify-end">
						<IconAction
							icon={copiedId === turn.id ? 'check' : 'copy'}
							label={copiedId === turn.id ? $t('ui.common.copied') : $t('ui.common.copy')}
							onclick={() => copy(turn.id, turn.result.text)}
							class="rounded-none hover:bg-transparent"
						/>
					</div>
				{/if}
			</div>

			{#if meta !== ''}
				<div class="mt-4 flex flex-wrap items-center gap-x-3 gap-y-2">
					<p class="mono-data ms-auto min-w-0 text-right break-words text-muted-foreground">
						{meta}
					</p>
				</div>
			{/if}
		</li>
	{/each}
</ol>
