import { addMessages, init, locale } from 'svelte-i18n';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';

import { ChatSender, type ChatSendHost } from './chat-send.svelte';
import {
	activeThread,
	defaultChatSettings,
	emptyStore,
	newThread,
	withActive,
	withThread,
	type ChatSettings,
	type ChatStore
} from './chat-threads';
import en from './i18n/locales/en.json';

// The sender writes into a conversation store it is handed, so the tests drive
// it with a plain host and read what it left behind: which turn it added, what
// the frames did to it, and how a dropped request leaves it.

beforeAll(async () => {
	addMessages('en', en);
	await init({ fallbackLocale: 'en', initialLocale: 'en' });
});

beforeEach(() => {
	locale.set('en');
	vi.stubGlobal('document', {
		get cookie() {
			return 'relo_csrf=token';
		},
		documentElement: { setAttribute: () => undefined }
	});
	vi.stubGlobal('window', { dispatchEvent: () => true });
});

afterEach(() => {
	vi.unstubAllGlobals();
	vi.useRealTimers();
});

/** A sender bound to a store it owns, with the composer side of the host recorded. */
function harness(settings: Partial<ChatSettings> = {}) {
	const thread = newThread({ ...defaultChatSettings(), ...settings });
	let store: ChatStore = withActive(withThread(emptyStore(), thread), thread.id);
	let saves = 0;
	let cleared = false;
	const sender = new ChatSender();
	sender.bind({
		store: () => store,
		setStore: (next) => (store = next),
		persist: () => {
			saves++;
		},
		clearDraft: () => {
			cleared = true;
		},
		cancelDraftSave: () => undefined
	} satisfies ChatSendHost);
	return {
		sender,
		settings: thread.settings,
		store: () => store,
		turn: () => activeThread(store)?.turns.at(-1),
		saves: () => saves,
		draftCleared: () => cleared
	};
}

function delta(text: string): string {
	return 'data: ' + JSON.stringify({ type: 'delta', text }) + '\n\n';
}

function answer(text: string): string {
	return (
		'data: ' +
		JSON.stringify({
			type: 'answer',
			answer: {
				request_id: 'r1',
				ok: true,
				text,
				status: 200,
				input_tokens: 0,
				output_tokens: 0,
				duration_ms: 0
			}
		}) +
		'\n\n'
	);
}

/** A request that never answers on its own, and rejects the way the platform's fetch does when
 *  the caller takes it back. */
function pendingFetch() {
	return vi.fn(
		(_url: string, init: RequestInit) =>
			new Promise((_resolve, reject) => {
				init.signal?.addEventListener('abort', () =>
					reject(new DOMException('The operation was aborted.', 'AbortError'))
				);
			})
	);
}

function streamOf(body: string): ReadableStream<Uint8Array> {
	const encoder = new TextEncoder();
	return new ReadableStream({
		start(controller) {
			controller.enqueue(encoder.encode(body));
			controller.close();
		}
	});
}

describe('sending a turn', () => {
	it('writes the prompt as a pending turn and settles it with the answer', async () => {
		const h = harness();
		vi.stubGlobal(
			'fetch',
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					json: () =>
						Promise.resolve({
							request_id: 'r1',
							ok: true,
							text: 'Hello',
							status: 200,
							input_tokens: 1,
							output_tokens: 1,
							duration_ms: 1
						})
				})
			)
		);

		await h.sender.send('Hi');

		expect(h.turn()?.prompt).toBe('Hi');
		expect(h.turn()?.result).toMatchObject({ status: 'success', text: 'Hello' });
		expect(h.draftCleared()).toBe(true);
		expect(h.sender.sending).toBe(false);
		expect(h.sender.inflightThreadId).toBe('');
	});

	it('takes the title from the first prompt and keeps it after that', async () => {
		const h = harness();
		vi.stubGlobal(
			'fetch',
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					json: () => Promise.resolve({ ok: true, text: 'a' })
				})
			)
		);
		await h.sender.send('First question');
		await h.sender.send('Second question');
		expect(activeThread(h.store())?.title).toBe('First question');
	});

	it('refuses a prompt with nothing in it', async () => {
		const h = harness();
		const fetchMock = vi.fn();
		vi.stubGlobal('fetch', fetchMock);
		await h.sender.send('   ');
		expect(fetchMock).not.toHaveBeenCalled();
		expect(activeThread(h.store())?.turns).toHaveLength(0);
	});

	it('refuses a second turn while the first is still running', async () => {
		const h = harness();
		let release: (value: unknown) => void = () => undefined;
		vi.stubGlobal(
			'fetch',
			vi.fn(
				() =>
					new Promise((resolve) => {
						release = resolve;
					})
			)
		);
		const first = h.sender.send('One');
		await h.sender.send('Two');
		expect(activeThread(h.store())?.turns).toHaveLength(1);
		release({ ok: true, status: 200, json: () => Promise.resolve({ ok: true, text: 'a' }) });
		await first;
	});
});

describe('following a streamed answer', () => {
	it('shows each delta and settles on the frame that closes the turn', async () => {
		const h = harness({ stream: true });
		vi.stubGlobal(
			'fetch',
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					body: streamOf(delta('Hel') + delta('lo') + answer('Hello'))
				})
			)
		);

		await h.sender.send('Hi');

		expect(h.turn()?.result).toMatchObject({ status: 'success', text: 'Hello' });
	});

	it('leaves the turn interrupted when the frame that closes it never arrives', async () => {
		const h = harness({ stream: true });
		vi.stubGlobal(
			'fetch',
			vi.fn(() => Promise.resolve({ ok: true, status: 200, body: streamOf(delta('half')) }))
		);

		await h.sender.send('Hi');

		// Half an answer is never shown as a whole one: the turn reports the
		// stream ended rather than keeping the text it had shown.
		expect(h.turn()?.result).toMatchObject({ status: 'failed', text: '' });
		expect(h.turn()?.result.error).toBeTruthy();
	});
});

describe('taking back a turn', () => {
	it('stops the wait and leaves the last turn ready for a retry', async () => {
		const h = harness({ stream: true });
		vi.stubGlobal('fetch', pendingFetch());
		const running = h.sender.send('Hi');
		await Promise.resolve();
		expect(h.sender.sending).toBe(true);

		h.sender.stop();

		expect(h.sender.sending).toBe(false);
		expect(h.sender.inflightThreadId).toBe('');
		expect(h.turn()?.result.status).toBe('interrupted');
		await running;
	});

	it('drops an answer for a conversation that is already gone', async () => {
		const h = harness();
		let release: (value: unknown) => void = () => undefined;
		vi.stubGlobal(
			'fetch',
			vi.fn(
				() =>
					new Promise((resolve) => {
						release = resolve;
					})
			)
		);
		const running = h.sender.send('Hi');
		let written = 0;
		// The operator deleted the conversation while its request ran.
		h.sender.bind({
			store: () => ({ ...h.store(), threads: [], activeThreadId: '' }),
			setStore: () => {
				written++;
			},
			persist: () => undefined,
			clearDraft: () => undefined,
			cancelDraftSave: () => undefined
		});
		release({ ok: true, status: 200, json: () => Promise.resolve({ ok: true, text: 'late' }) });
		await running;
		// The answer is dropped rather than recreating the conversation.
		expect(written).toBe(0);
	});
});

describe('retrying a turn', () => {
	it('replaces the turn it was given rather than adding another', async () => {
		const h = harness();
		vi.stubGlobal(
			'fetch',
			vi.fn(() =>
				Promise.resolve({
					ok: true,
					status: 200,
					json: () => Promise.resolve({ ok: true, text: 'a' })
				})
			)
		);
		await h.sender.send('Hi');
		const first = h.turn();
		await h.sender.send('Hi again', first?.id);
		const turns = activeThread(h.store())?.turns ?? [];
		expect(turns).toHaveLength(1);
		expect(turns[0].prompt).toBe('Hi again');
	});
});
