import { get } from 'svelte/store';
import { t } from 'svelte-i18n';

import { ApiError, api, apiStream } from './api';
import { readChatStream } from './chat-stream';
import {
	activeThread,
	chatRequest,
	contextMessages,
	failedResult,
	interruptedResult,
	newId,
	resultOf,
	titleFromPrompt,
	withThread,
	type ChatStore,
	type ChatThread,
	type ChatTurn,
	type ChatTurnResult
} from './chat-threads';
import type { ChatTesterAnswer, ChatTesterRequest } from './types';

// The turn being sent is the page's slowest piece of work: it opens a stream,
// shows each frame as it arrives, and writes the answer back into the
// conversation that asked for it. Holding it here keeps the page's own state
// to the draft and the panes, and lets the streaming rules be read without a
// component around them.

/** What the sender needs from the page it drives: the conversations it writes into, and the two
 *  side effects that writing carries. */
export interface ChatSendHost {
	/** The conversations as they currently read. */
	store(): ChatStore;
	/** Writes the conversations back. */
	setStore(next: ChatStore): void;
	/** Writes the conversations to this browser now. */
	persist(): void;
	/** Empties the composer, once the prompt it held has become a turn. */
	clearDraft(): void;
	/** Drops a queued draft save, so a prompt that became a turn is not written twice. */
	cancelDraftSave(): void;
}

/** Sends one turn and follows the answer into the conversation it belongs to. */
export class ChatSender {
	/** True while a turn is in flight, which is what a composer reads to offer its stop. */
	sending = $state(false);
	/** The conversation the in-flight turn belongs to, so its own rail row can say so. */
	inflightThreadId = $state('');

	#host: ChatSendHost | null = null;
	#controller: AbortController | null = null;
	#saveTimer: ReturnType<typeof setTimeout> | undefined;

	/** Points the sender at the page it drives. Called once, when the page starts. */
	bind(host: ChatSendHost): void {
		this.#host = host;
	}

	// growTurn shows the text a streamed reply has written so far. History is
	// not written on every delta: the debounce behind it carries the latest
	// one to storage once the text settles.
	#grow(threadId: string, turnId: string, text: string): void {
		const host = this.#require();
		const target = host.store().threads.find((entry) => entry.id === threadId);
		if (target === undefined) return;
		host.setStore(
			withThread(host.store(), {
				...target,
				turns: target.turns.map((turn) =>
					turn.id === turnId ? { ...turn, result: { ...turn.result, text } } : turn
				)
			})
		);
	}

	// The debounce writes history a moment after the last delta, so a reply
	// still being written is not saved a token at a time.
	#queuePersist(): void {
		if (this.#saveTimer) clearTimeout(this.#saveTimer);
		this.#saveTimer = setTimeout(() => {
			this.#saveTimer = undefined;
			this.#require().persist();
		}, 400);
	}

	// settleTurn writes the answer into the conversation that asked for it. A
	// conversation deleted while its request ran is simply gone, so the answer
	// is dropped rather than recreated.
	#settle(threadId: string, turnId: string, result: ChatTurnResult): void {
		const host = this.#require();
		const target = host.store().threads.find((entry) => entry.id === threadId);
		if (target === undefined) return;
		host.setStore(
			withThread(host.store(), {
				...target,
				updatedAt: Date.now(),
				turns: target.turns.map((turn) => (turn.id === turnId ? { ...turn, result } : turn))
			})
		);
		host.persist();
	}

	// shownText reads the text a turn has already shown, which is what its
	// failure or its stop keeps.
	#shown(threadId: string, turnId: string): string {
		const target = this.#require()
			.store()
			.threads.find((entry) => entry.id === threadId);
		const turn =
			target === undefined ? undefined : target.turns.find((entry) => entry.id === turnId);
		return turn === undefined ? '' : turn.result.text;
	}

	#failure(error: unknown, streamed = ''): ChatTurnResult {
		if (error instanceof DOMException && error.name === 'AbortError')
			return interruptedResult(streamed);
		if (error instanceof Error && error.name === 'AbortError') return interruptedResult(streamed);
		const text = error instanceof Error ? error.message : String(error);
		return failedResult(text, error instanceof ApiError ? error.code : '');
	}

	// refusal reads the refusal the daemon answers a turn with before any
	// stream opens, so a malformed request reads the way every other refused
	// request does.
	async #refusal(response: Response): Promise<ApiError> {
		const payload = await response.json().catch(() => null);
		const code = typeof payload?.error?.code === 'string' ? payload.error.code : 'error';
		const message =
			typeof payload?.error?.message === 'string'
				? payload.error.message
				: get(t)('ui.common.requestFailed', { values: { status: response.status } });
		return new ApiError(response.status, code, message);
	}

	// streamAnswer runs one turn frame by frame: every delta the daemon
	// forwards is shown as it arrives, and the frame that closes the stream is
	// what settles the turn.
	async #stream(
		threadId: string,
		turnId: string,
		request: ChatTesterRequest,
		signal: AbortSignal
	): Promise<void> {
		const response = await apiStream('/integrations/chat', {
			method: 'POST',
			headers: { Accept: 'text/event-stream' },
			body: JSON.stringify(request),
			signal
		});
		if (!response.ok) throw await this.#refusal(response);
		if (response.body === null) throw new Error(get(t)('ui.pages.chatPage.streamEnded'));
		let text = '';
		let closed = false;
		await readChatStream(response.body, (frame) => {
			if (frame.type === 'delta') {
				text += frame.text;
				this.#grow(threadId, turnId, text);
				this.#queuePersist();
				return;
			}
			closed = true;
			this.#settle(threadId, turnId, resultOf(frame.answer));
		});
		// A stream that ended without the frame that closes a turn leaves the
		// turn saying so, rather than showing half an answer as a whole one.
		if (!closed) throw new Error(get(t)('ui.pages.chatPage.streamEnded'));
	}

	/** Sends a prompt as the next turn of the active conversation, or replaces the turn the
	 *  operator asked to retry. */
	async send(promptText: string, replaceTurnId = ''): Promise<void> {
		const host = this.#require();
		const current = activeThread(host.store());
		if (current === null || this.sending) return;
		const prompt = promptText.trim();
		if (prompt === '') return;
		host.cancelDraftSave();

		const now = Date.now();
		const keptTurns =
			replaceTurnId === ''
				? current.turns
				: current.turns.filter((turn) => turn.id !== replaceTurnId);
		const turn: ChatTurn = {
			id: newId(),
			prompt,
			result: { status: 'pending', text: '' },
			at: now
		};
		const next: ChatThread = {
			...current,
			title: current.title === '' ? titleFromPrompt(prompt) : current.title,
			updatedAt: now,
			draft: '',
			turns: [...keptTurns, turn]
		};
		// The turns that answered carry the context; the one being retried and
		// the new prompt are added on top of it.
		const messages = contextMessages(next);
		host.setStore(withThread(host.store(), next));
		if (replaceTurnId === '') host.clearDraft();
		host.persist();

		this.sending = true;
		this.inflightThreadId = next.id;
		this.#controller = new AbortController();
		try {
			const request = chatRequest(next.settings, prompt, messages);
			if (next.settings.stream) {
				await this.#stream(next.id, turn.id, request, this.#controller.signal);
			} else {
				const answer = await api<ChatTesterAnswer>('/integrations/chat', {
					method: 'POST',
					body: JSON.stringify(request),
					signal: this.#controller.signal
				});
				this.#settle(next.id, turn.id, resultOf(answer));
			}
		} catch (error) {
			// A reply that had already streamed keeps the text it showed, so a
			// failure or a stop leaves the reader what they read.
			this.#settle(next.id, turn.id, this.#failure(error, this.#shown(next.id, turn.id)));
		} finally {
			this.sending = false;
			this.inflightThreadId = '';
			this.#controller = null;
		}
	}

	/** Drops the in-flight request and forgets it, without settling its turn. */
	abort(): void {
		if (this.#controller !== null) {
			this.#controller.abort();
			this.#controller = null;
		}
		this.sending = false;
		this.inflightThreadId = '';
		if (this.#saveTimer) {
			clearTimeout(this.#saveTimer);
			this.#saveTimer = undefined;
		}
	}

	/** Takes back the wait: the request is dropped and the turn it was filling says so, which
	 *  leaves it ready for a retry. A reply that had already streamed keeps the text it showed. */
	stop(): void {
		const current = activeThread(this.#require().store());
		const last = current === null ? undefined : current.turns[current.turns.length - 1];
		const shown = last === undefined ? '' : last.result.text;
		this.abort();
		if (current === null || last === undefined || last.result.status !== 'pending') return;
		this.#settle(current.id, last.id, interruptedResult(shown));
	}

	#require(): ChatSendHost {
		if (this.#host === null) throw new Error('ChatSender is not bound to a page');
		return this.#host;
	}
}
