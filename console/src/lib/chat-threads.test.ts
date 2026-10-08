import { describe, expect, it } from 'vitest';

import {
	CHAT_PANELS_KEY,
	CHAT_STORAGE_KEY,
	activeThread,
	chatRequest,
	clearChatStore,
	contextMessages,
	defaultChatSettings,
	emptyStore,
	groupThreads,
	interruptedResult,
	isChatFormat,
	newThread,
	parseChatStore,
	readChatPanels,
	readChatStore,
	resultOf,
	retryableTurn,
	settle,
	settingsDiffer,
	sortedThreads,
	titleFromPrompt,
	withoutThread,
	writeChatPanels,
	writeChatStore,
	type ChatThread,
	type ChatTurn,
	type ChatTurnStatus,
	type StorageLike
} from './chat-threads';

// A conversation of n turns, all with the same ending, which is enough to
// check what a later request replays and what the rail shows.
function thread(
	id: string,
	turns: number,
	status: ChatTurnStatus = 'success',
	updatedAt = 1
): ChatThread {
	const list: ChatTurn[] = [];
	for (let index = 0; index < turns; index += 1) {
		list.push({
			id: id + '-turn-' + index,
			prompt: 'prompt ' + index,
			at: updatedAt,
			result: status === 'success' ? { status, text: 'answer ' + index } : { status, text: '' }
		});
	}
	return {
		id,
		title: 'Thread ' + id,
		createdAt: 1,
		updatedAt,
		draft: '',
		settings: defaultChatSettings(),
		turns: list
	};
}

// A storage that behaves like a browser's, one key deep.
function memoryStorage(): { storage: StorageLike; read: () => string | null } {
	let value: string | null = null;
	return {
		storage: {
			getItem: (key) => (key === CHAT_STORAGE_KEY ? value : null),
			setItem: (key, next) => {
				if (key === CHAT_STORAGE_KEY) value = next;
			},
			removeItem: (key) => {
				if (key === CHAT_STORAGE_KEY) value = null;
			}
		},
		read: () => value
	};
}

function withTurns(store = emptyStore(), ...threads: ChatThread[]) {
	return { ...store, activeThreadId: threads[0]?.id ?? '', threads };
}

describe('a thread title', () => {
	it('takes the first line that carries anything, with the spacing collapsed', () => {
		expect(titleFromPrompt('\n\n  Explain   the   router \n second line')).toBe(
			'Explain the router'
		);
	});

	it('cuts a long prompt to what the rail shows', () => {
		const title = titleFromPrompt('a'.repeat(80));
		expect(title).toHaveLength(48);
		expect(title.endsWith('…')).toBe(true);
	});

	it('is empty for a prompt with nothing in it', () => {
		expect(titleFromPrompt('   \n  ')).toBe('');
	});
});

describe('settling a store', () => {
	it('keeps a thread that has turns and drops one that has none', () => {
		const store = withTurns(emptyStore(), thread('a', 1), thread('b', 0));
		expect(settle(store).threads.map((entry) => entry.id)).toEqual(['a']);
	});

	it('marks a turn that was still running when the console closed', () => {
		const store = withTurns(emptyStore(), thread('a', 1, 'pending'));
		expect(settle(store).threads[0].turns[0].result.status).toBe('interrupted');
	});

	it('leaves the active thread on the newest one that survived', () => {
		const store = {
			...withTurns(emptyStore(), thread('old', 1, 'success', 10), thread('new', 1, 'success', 20)),
			activeThreadId: 'gone'
		};
		expect(settle(store).activeThreadId).toBe('new');
	});
});

describe('reading stored history', () => {
	it('reads nothing as an empty store with no complaint', () => {
		const { store, notice } = readChatStore(memoryStorage().storage);
		expect(notice).toBe('');
		expect(store.threads).toEqual([]);
	});

	it('refuses a value it cannot parse, without touching it', () => {
		const { storage, read } = memoryStorage();
		storage.setItem(CHAT_STORAGE_KEY, '{not json');
		const { store, notice } = readChatStore(storage);
		expect(notice).toBe('corrupt');
		expect(store.threads).toEqual([]);
		expect(read()).toBe('{not json');
	});

	it('refuses a store from a shape it does not know', () => {
		expect(
			parseChatStore(JSON.stringify({ version: 99, activeThreadId: '', threads: [] }))
		).toBeNull();
		expect(parseChatStore(JSON.stringify({ activeThreadId: '', threads: [] }))).toBeNull();
	});

	it('keeps a thread that is missing an identifier out of the store', () => {
		const parsed = parseChatStore(
			JSON.stringify({
				version: 1,
				activeThreadId: 'a',
				threads: [
					{ id: '', turns: [], settings: {}, title: 'no id' },
					{ id: 'a', turns: [], settings: {}, title: 'kept' }
				]
			})
		);
		expect(parsed?.threads.map((entry) => entry.id)).toEqual(['a']);
	});

	it('names storage the browser will not hand over', () => {
		const { notice } = readChatStore({
			getItem: () => {
				throw new Error('denied');
			},
			setItem: () => {},
			removeItem: () => {}
		});
		expect(notice).toBe('blocked');
	});

	it('reads a target stored before the tester could stream as one reply', () => {
		const parsed = parseChatStore(
			JSON.stringify({
				version: 1,
				activeThreadId: 'a',
				threads: [
					{
						id: 'a',
						turns: [],
						title: 'kept',
						settings: { mode: 'direct', providerId: 'openai', modelId: 'gpt-4o' }
					}
				]
			})
		);
		expect(parsed?.threads[0].settings.stream).toBe(false);
	});
});

describe('writing stored history', () => {
	it('round-trips the conversations through the one key', () => {
		const { storage, read } = memoryStorage();
		const store = withTurns(emptyStore(), thread('a', 2));
		expect(writeChatStore(store, storage)).toBe(true);
		expect(read()).not.toBeNull();
		expect(readChatStore(storage).store.threads[0].turns).toHaveLength(2);
	});

	it('answers false when the browser refuses the write', () => {
		const refused: StorageLike = {
			getItem: () => null,
			setItem: () => {
				throw new Error('quota');
			},
			removeItem: () => {}
		};
		expect(writeChatStore(withTurns(emptyStore(), thread('a', 1)), refused)).toBe(false);
	});

	it('clears only the key the console owns', () => {
		const { storage, read } = memoryStorage();
		storage.setItem(CHAT_STORAGE_KEY, 'anything');
		expect(clearChatStore(storage)).toBe(true);
		expect(read()).toBeNull();
	});
});

describe('the conversation a request replays', () => {
	it('keeps only the turns that came back with text', () => {
		const store = withTurns(emptyStore(), {
			...thread('a', 0),
			turns: [
				{ id: '1', prompt: 'first', at: 1, result: { status: 'success', text: 'one' } },
				{ id: '2', prompt: 'second', at: 2, result: { status: 'failed', text: '', error: 'nope' } },
				{ id: '3', prompt: 'third', at: 3, result: { status: 'empty', text: '' } },
				{ id: '4', prompt: 'fourth', at: 4, result: { status: 'pending', text: '' } }
			]
		});
		expect(contextMessages(activeThread(store)!)).toEqual([
			{ role: 'user', content: 'first' },
			{ role: 'assistant', content: 'one' }
		]);
	});

	it('offers a retry for the last turn once it has settled', () => {
		expect(retryableTurn(thread('a', 2))?.id).toBe('a-turn-1');
		expect(retryableTurn(thread('a', 1, 'failed'))?.id).toBe('a-turn-0');
		expect(retryableTurn(thread('a', 1, 'interrupted'))?.id).toBe('a-turn-0');
		expect(retryableTurn(thread('a', 1, 'empty'))?.id).toBe('a-turn-0');
		const mixed: ChatThread = {
			...thread('a', 0),
			turns: [
				{ id: '1', prompt: 'p', at: 1, result: { status: 'failed', text: '' } },
				{ id: '2', prompt: 'q', at: 2, result: { status: 'success', text: 'ok' } }
			]
		};
		// The failure is no longer the last turn, so it is not what a retry
		// would replace.
		expect(retryableTurn(mixed)?.id).toBe('2');
	});

	it('offers nothing to retry while the last turn is still running', () => {
		expect(retryableTurn(thread('a', 1, 'pending'))).toBeNull();
		expect(retryableTurn({ ...thread('a', 0), turns: [] })).toBeNull();
	});
});

describe('the body one turn posts', () => {
	const settings = defaultChatSettings();

	it('names the connection and the model in a direct test', () => {
		const body = chatRequest({ ...settings, providerId: 'openai', modelId: 'gpt-4o' }, 'hi', []);
		expect(body).toMatchObject({
			mode: 'direct',
			provider_id: 'openai',
			model_id: 'gpt-4o',
			route_id: ''
		});
		expect(body.messages).toEqual([{ role: 'user', content: 'hi' }]);
	});

	it('names only the route when a format test follows one', () => {
		const body = chatRequest(
			{
				...settings,
				mode: 'format',
				target: 'route',
				format: 'anthropic',
				providerId: 'openai',
				modelId: 'gpt-4o',
				routeId: 'fast'
			},
			'hi',
			[{ role: 'user', content: 'earlier' }]
		);
		expect(body).toMatchObject({
			mode: 'format',
			format: 'anthropic',
			provider_id: '',
			model_id: '',
			route_id: 'fast'
		});
		expect(body.messages).toEqual([
			{ role: 'user', content: 'earlier' },
			{ role: 'user', content: 'hi' }
		]);
	});

	it('carries the system prompt and the token ceiling', () => {
		const body = chatRequest(
			{ ...settings, providerId: 'openai', modelId: 'gpt-4o', system: 'be brief', maxTokens: 64 },
			'hi',
			[]
		);
		expect(body.system).toBe('be brief');
		expect(body.max_tokens).toBe(64);
	});

	it('asks for the reply frame by frame only when the conversation streams', () => {
		const streamed = chatRequest(
			{ ...settings, providerId: 'openai', modelId: 'gpt-4o', stream: true },
			'hi',
			[]
		);
		expect(streamed.stream).toBe(true);
		const complete = chatRequest(
			{ ...settings, providerId: 'openai', modelId: 'gpt-4o' },
			'hi',
			[]
		);
		expect(complete.stream).toBe(false);
	});
});

describe('the stream a conversation asks for', () => {
	it('is off in the settings a conversation starts with', () => {
		expect(defaultChatSettings().stream).toBe(false);
	});

	it('is part of what tells one target apart from another', () => {
		expect(settingsDiffer(defaultChatSettings(), { ...defaultChatSettings(), stream: true })).toBe(
			true
		);
		expect(settingsDiffer({ ...defaultChatSettings(), stream: true }, defaultChatSettings())).toBe(
			true
		);
		expect(settingsDiffer(defaultChatSettings(), defaultChatSettings())).toBe(false);
	});
});

describe('a turn that was taken back', () => {
	it('keeps the text a stopped stream had already shown', () => {
		expect(interruptedResult('half an answer')).toEqual({
			status: 'interrupted',
			text: 'half an answer'
		});
		expect(interruptedResult().text).toBe('');
	});
});

describe('what one answer becomes', () => {
	it('is a success when text came back', () => {
		const result = resultOf({
			request_id: 'r1',
			ok: true,
			text: 'hello',
			status: 200,
			input_tokens: 3,
			output_tokens: 4,
			duration_ms: 12
		} as never);
		expect(result.status).toBe('success');
		expect(result.requestId).toBe('r1');
		expect(result.outputTokens).toBe(4);
	});

	it('is its own state when the reply carried no text', () => {
		expect(resultOf({ request_id: 'r1', ok: true, text: '', status: 200 } as never).status).toBe(
			'empty'
		);
	});

	it('keeps the error the daemon named', () => {
		const result = resultOf({
			request_id: 'r1',
			ok: false,
			text: '',
			status: 502,
			error: 'upstream down',
			error_code: 'upstream_error'
		} as never);
		expect(result.status).toBe('failed');
		expect(result.error).toBe('upstream down');
		expect(result.errorCode).toBe('upstream_error');
	});
});

describe('the thread rail', () => {
	it('lists the most recently used conversation first', () => {
		const store = withTurns(
			emptyStore(),
			thread('old', 1, 'success', 10),
			thread('new', 1, 'success', 20)
		);
		expect(sortedThreads(store).map((entry) => entry.id)).toEqual(['new', 'old']);
	});

	it('falls back to the newest conversation when the active one is deleted', () => {
		const store = {
			...withTurns(emptyStore(), thread('old', 1, 'success', 10), thread('new', 1, 'success', 20)),
			activeThreadId: 'new'
		};
		const next = withoutThread(store, 'new');
		expect(next.activeThreadId).toBe('old');
		expect(next.threads.map((entry) => entry.id)).toEqual(['old']);
	});

	it('keeps the selection when another conversation is deleted', () => {
		const store = {
			...withTurns(emptyStore(), thread('old', 1, 'success', 10), thread('new', 1, 'success', 20)),
			activeThreadId: 'old'
		};
		expect(withoutThread(store, 'new').activeThreadId).toBe('old');
	});
});

describe('a new conversation', () => {
	it('starts with the settings it inherits and nothing sent', () => {
		const fresh = newThread({ ...defaultChatSettings(), providerId: 'openai', modelId: 'gpt-4o' });
		expect(fresh.turns).toEqual([]);
		expect(fresh.title).toBe('');
		expect(fresh.draft).toBe('');
		expect(fresh.settings.modelId).toBe('gpt-4o');
	});
});

// The rail reads a long list by when each conversation was last touched, so
// these are the windows it files them under, checked at their edges.
describe('the windows the rail files conversations under', () => {
	const now = new Date(2026, 8, 28, 15, 0, 0).getTime();

	it('files each conversation under the day it was last touched', () => {
		const listed = [
			thread('today', 1, 'success', new Date(2026, 8, 28, 0, 30, 0).getTime()),
			thread('late', 1, 'success', new Date(2026, 8, 27, 23, 50, 0).getTime()),
			thread('week', 1, 'success', new Date(2026, 8, 25, 12, 0, 0).getTime()),
			thread('old', 1, 'success', new Date(2026, 8, 20, 12, 0, 0).getTime())
		];
		const groups = groupThreads(listed, now);
		expect(groups.map((group) => group.key)).toEqual(['today', 'yesterday', 'week', 'older']);
		expect(groups.map((group) => group.threads.map((entry) => entry.id))).toEqual([
			['today'],
			['late'],
			['week'],
			['old']
		]);
	});

	it('drops a window nothing falls into', () => {
		const groups = groupThreads(
			[thread('a', 1, 'success', new Date(2026, 8, 28, 9, 0, 0).getTime())],
			now
		);
		expect(groups.map((group) => group.key)).toEqual(['today']);
	});

	it('ends the week at midnight seven days back', () => {
		const edge = new Date(2026, 8, 21, 9, 0, 0).getTime();
		const before = new Date(2026, 8, 20, 23, 0, 0).getTime();
		expect(groupThreads([thread('a', 1, 'success', edge)], now)[0].key).toBe('week');
		expect(groupThreads([thread('a', 1, 'success', before)], now)[0].key).toBe('older');
	});

	it('keeps the order it was given', () => {
		const listed = [
			thread('newer', 1, 'success', new Date(2026, 8, 28, 14, 0, 0).getTime()),
			thread('older', 1, 'success', new Date(2026, 8, 28, 13, 0, 0).getTime())
		];
		expect(groupThreads(listed, now)[0].threads.map((entry) => entry.id)).toEqual([
			'newer',
			'older'
		]);
	});
});

// The two side panes are a layout, not history: a value that cannot be read
// falls back rather than stopping the page.
describe('the layout the chat page remembers', () => {
	const defaults = { threads: true, settings: false };

	// A storage one key deep, which is all the layout needs.
	function panelsStorage(initial: string | null = null): StorageLike {
		let value = initial;
		return {
			getItem: (key) => (key === CHAT_PANELS_KEY ? value : null),
			setItem: (key, next) => {
				if (key === CHAT_PANELS_KEY) value = next;
			},
			removeItem: (key) => {
				if (key === CHAT_PANELS_KEY) value = null;
			}
		};
	}

	it('answers the defaults when nothing was stored', () => {
		expect(readChatPanels(defaults, panelsStorage())).toEqual(defaults);
	});

	it('keeps what the operator last chose', () => {
		const storage = panelsStorage();
		expect(writeChatPanels({ threads: false, settings: true }, storage)).toBe(true);
		expect(readChatPanels(defaults, storage)).toEqual({ threads: false, settings: true });
	});

	it('answers the defaults for a value it cannot read', () => {
		expect(readChatPanels({ threads: true, settings: true }, panelsStorage('not json'))).toEqual({
			threads: true,
			settings: true
		});
		expect(
			readChatPanels({ threads: false, settings: true }, panelsStorage('{"threads":"yes"}'))
		).toEqual({ threads: false, settings: true });
	});

	it('answers the defaults where the browser hands over no storage', () => {
		expect(readChatPanels(defaults, undefined)).toEqual(defaults);
		expect(writeChatPanels({ threads: false, settings: false }, undefined)).toBe(false);
	});

	it('writes no conversation history of its own', () => {
		const storage = panelsStorage();
		writeChatPanels({ threads: false, settings: true }, storage);
		expect(storage.getItem(CHAT_STORAGE_KEY)).toBeNull();
	});
});

describe('the guard over a wire dialect', () => {
	it('accepts only the three the tester speaks', () => {
		expect(isChatFormat('openai-chat')).toBe(true);
		expect(isChatFormat('openai-responses')).toBe(true);
		expect(isChatFormat('anthropic')).toBe(true);
		expect(isChatFormat('gemini')).toBe(false);
	});
});
