// The chat tester keeps its conversations in the browser. A thread lives in
// localStorage so an operator can leave the console and come back to it, and
// the daemon stores nothing of its own: a test is an ordinary request that
// lands in the log as internal traffic.
//
// The rules that shape a store — which thread is active, what a request may
// carry, what a turn becomes once it fails — live here, apart from the
// components, so a test can hold them directly.

import type { ChatTesterAnswer, ChatTesterMessage, ChatTesterRequest } from './types';

/** The one key the console owns for chat history.
 *  The version is part of the key, so a later shape is a new key rather than a migration that
 *  could misread an old value. */
export const CHAT_STORAGE_KEY = 'relo.chat.v1';

/** The shape inside that key. */
export const CHAT_STORE_VERSION = 1;

/** How a turn chooses what it talks to. */
export type ChatMode = 'direct' | 'format';
/** Whether a turn names a model or a group. */
export type ChatTarget = 'model' | 'route';
/** The wire dialect the tester speaks. */
export type ChatFormat = 'openai-chat' | 'openai-responses' | 'anthropic';

/** Reports whether a choice a select handed back is one the tester can speak, so an unexpected
 *  value leaves the conversation on the format it was already using. */
export function isChatFormat(value: string): value is ChatFormat {
	return value === 'openai-chat' || value === 'openai-responses' || value === 'anthropic';
}

/** How one attempted turn ended. Only a success that carried text can feed a later request: the
 *  backend refuses an empty message, so a failed, interrupted, or empty turn stays on screen
 *  without being sent again. */
export type ChatTurnStatus = 'pending' | 'success' | 'empty' | 'failed' | 'interrupted';

/** A turn's target, format and drafting controls. */
export interface ChatSettings {
	mode: ChatMode;
	// target names what a format test points at. A direct test always names one
	// connection's model.
	target: ChatTarget;
	format: ChatFormat;
	providerId: string;
	modelId: string;
	routeId: string;
	system: string;
	maxTokens: number;
	// stream is whether a turn asks for the reply frame by frame. It belongs
	// to the target a conversation is tested with, so changing it after a turn
	// starts a new conversation the way every other setting does.
	stream: boolean;
}

/** One turn as the transcript shows it. */
export interface ChatTurnResult {
	status: ChatTurnStatus;
	text: string;
	provider?: string;
	model?: string;
	requestId?: string;
	httpStatus?: number;
	error?: string;
	errorCode?: string;
	inputTokens?: number;
	outputTokens?: number;
	durationMs?: number;
}

/** One exchange in a thread. */
export interface ChatTurn {
	id: string;
	prompt: string;
	result: ChatTurnResult;
	at: number;
}

/** A conversation, with the settings it was started under. */
export interface ChatThread {
	id: string;
	title: string;
	createdAt: number;
	updatedAt: number;
	draft: string;
	settings: ChatSettings;
	turns: ChatTurn[];
}

/** Every thread the tester holds, and which one is open. */
export interface ChatStore {
	version: number;
	activeThreadId: string;
	threads: ChatThread[];
}

/** Explains why history is not being written. An empty value means the store is being saved
 *  normally. */
export type ChatStorageNotice = '' | 'corrupt' | 'blocked';

/** The slice of localStorage this module uses, so a test can hand it a fake that throws the way
 *  a browser with storage denied does. */
export interface StorageLike {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
	removeItem(key: string): void;
}

/** LocalStorage where the console runs in a browser.
 *  A browser that blocks storage can throw on the property itself, which is the same condition
 *  as a refused write. */
export function browserStorage(): StorageLike | undefined {
	try {
		return typeof localStorage === 'undefined' ? undefined : localStorage;
	} catch {
		return undefined;
	}
}

/** Builds a tester with no threads. */
export function emptyStore(): ChatStore {
	return { version: CHAT_STORE_VERSION, activeThreadId: '', threads: [] };
}

/** The settings a new thread starts under. */
export function defaultChatSettings(): ChatSettings {
	return {
		mode: 'direct',
		target: 'model',
		format: 'openai-chat',
		providerId: '',
		modelId: '',
		routeId: '',
		system: '',
		maxTokens: 1024,
		stream: false
	};
}

/** Mints an identifier for a thread or a turn. The browser's own generator is used where it
 *  exists, and the fallback keeps the function usable in a test. */
export function newId(): string {
	const generator = typeof crypto === 'undefined' ? undefined : crypto;
	if (generator && typeof generator.randomUUID === 'function') return generator.randomUUID();
	return 'chat-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 10);
}

/** Builds an empty thread. */
export function newThread(settings: ChatSettings, now = Date.now()): ChatThread {
	return {
		id: newId(),
		title: '',
		createdAt: now,
		updatedAt: now,
		draft: '',
		settings: { ...settings },
		turns: []
	};
}

/** The name a thread takes from its first prompt: the first line that carries anything,
 *  whitespace collapsed, and cut to what the rail can show. */
export function titleFromPrompt(prompt: string): string {
	const line = prompt.split(/\r?\n/).find((candidate) => candidate.trim() !== '') ?? '';
	const flat = line.trim().replace(/\s+/g, ' ');
	if (flat === '') return '';
	return flat.length > 48 ? flat.slice(0, 47).trimEnd() + '…' : flat;
}

/** Applies the rules that hold a store together: a thread with no turns is not kept, a turn that
 *  was still running when the console closed is marked interrupted, and the active thread always
 *  names a thread that exists. */
export function settle(store: ChatStore): ChatStore {
	const threads = store.threads
		.filter((thread) => thread.turns.length > 0)
		.map((thread) => ({
			...thread,
			// The annotation is what keeps 'interrupted' a status rather than a
			// string: the object below has no other type to be read against.
			turns: thread.turns.map((turn): ChatTurn =>
				turn.result.status === 'pending'
					? { ...turn, result: { ...turn.result, status: 'interrupted' } }
					: turn
			)
		}));
	const activeThreadId = threads.some((thread) => thread.id === store.activeThreadId)
		? store.activeThreadId
		: newestThreadId(threads);
	return { version: CHAT_STORE_VERSION, activeThreadId, threads };
}

/** Lists conversations most recently active first, which is the order the rail shows and the
 *  order a delete falls back on. */
export function sortedThreads(store: ChatStore): ChatThread[] {
	return store.threads.slice().sort((left, right) => right.updatedAt - left.updatedAt);
}

/** One heading in the rail: the conversations last touched in one window of time, in the order
 *  the list already holds them. */
export interface ChatThreadGroup {
	key: 'today' | 'yesterday' | 'week' | 'older';
	threads: ChatThread[];
}

function dayStart(ms: number): number {
	const date = new Date(ms);
	date.setHours(0, 0, 0, 0);
	return date.getTime();
}

/** Splits a sorted list into the headings the rail shows.
 *  The windows are local calendar days, so a conversation touched at 23:50 is not filed under
 *  yesterday by the time midnight passes, and a heading with nothing under it is dropped rather
 *  than shown empty. */
export function groupThreads(threads: ChatThread[], now = Date.now()): ChatThreadGroup[] {
	const today = dayStart(now);
	const yesterdayDate = new Date(today);
	yesterdayDate.setDate(yesterdayDate.getDate() - 1);
	const yesterday = yesterdayDate.getTime();
	const weekDate = new Date(today);
	weekDate.setDate(weekDate.getDate() - 7);
	const week = weekDate.getTime();

	const groups: ChatThreadGroup[] = [
		{ key: 'today', threads: [] },
		{ key: 'yesterday', threads: [] },
		{ key: 'week', threads: [] },
		{ key: 'older', threads: [] }
	];
	for (const thread of threads) {
		const index =
			thread.updatedAt >= today
				? 0
				: thread.updatedAt >= yesterday
					? 1
					: thread.updatedAt >= week
						? 2
						: 3;
		groups[index].threads.push(thread);
	}
	return groups.filter((group) => group.threads.length > 0);
}

/** The thread the tester is showing. */
export function activeThread(store: ChatStore): ChatThread | null {
	return store.threads.find((thread) => thread.id === store.activeThreadId) ?? null;
}

/** Names the thread a delete falls back on: the one that was touched last. */
export function newestThreadId(threads: ChatThread[]): string {
	let newest: ChatThread | null = null;
	for (const thread of threads) {
		if (!newest || thread.updatedAt > newest.updatedAt) newest = thread;
	}
	return newest === null ? '' : newest.id;
}

/** Names which thread is open. */
export function withActive(store: ChatStore, id: string): ChatStore {
	return { ...store, activeThreadId: id };
}

/** Folds one thread back into a store, adding it when it is new. */
export function withThread(store: ChatStore, thread: ChatThread): ChatStore {
	const known = store.threads.some((candidate) => candidate.id === thread.id);
	const threads = known
		? store.threads.map((candidate) => (candidate.id === thread.id ? thread : candidate))
		: [...store.threads, thread];
	return { ...store, threads };
}

/** Drops one conversation and, when it was the active one, leaves the newest conversation behind
 *  it selected. */
export function withoutThread(store: ChatStore, id: string): ChatStore {
	const threads = store.threads.filter((thread) => thread.id !== id);
	const activeThreadId =
		store.activeThreadId === id ? newestThreadId(threads) : store.activeThreadId;
	return { ...store, threads, activeThreadId };
}

/** The conversation a later request replays: every completed turn that came back with text, in
 *  order. */
export function contextMessages(thread: ChatThread): ChatTesterMessage[] {
	const messages: ChatTesterMessage[] = [];
	for (const turn of thread.turns) {
		if (turn.result.status !== 'success') continue;
		if (turn.result.text.trim() === '') continue;
		messages.push({ role: 'user', content: turn.prompt });
		messages.push({ role: 'assistant', content: turn.result.text });
	}
	return messages;
}

/** Names the turn a retry would replace: only the last turn can be retried, and only once it has
 *  settled.
 *  A last answer that arrived can be asked for again, and one that failed or was interrupted can
 *  be taken back. */
export function retryableTurn(thread: ChatThread): ChatTurn | null {
	const last = thread.turns[thread.turns.length - 1];
	if (!last) return null;
	return last.result.status === 'pending' ? null : last;
}

/** Whether a settings change has to start a new conversation: a thread that already sent a turn
 *  keeps the target it was tested with. */
export function needsNewThread(thread: ChatThread): boolean {
	return thread.turns.length > 0;
}

/** Reports whether two settings would run different turns. */
export function settingsDiffer(left: ChatSettings, right: ChatSettings): boolean {
	return (
		left.mode !== right.mode ||
		left.target !== right.target ||
		left.format !== right.format ||
		left.providerId !== right.providerId ||
		left.modelId !== right.modelId ||
		left.routeId !== right.routeId ||
		left.system !== right.system ||
		left.maxTokens !== right.maxTokens ||
		left.stream !== right.stream
	);
}

/** Whether a turn follows a route instead of naming a model: only a format test may, and only
 *  when it says so. */
export function usesRoute(settings: ChatSettings): boolean {
	return settings.mode === 'format' && settings.target === 'route';
}

/** Whether the chosen target names something to call. */
export function settingsReady(settings: ChatSettings): boolean {
	if (usesRoute(settings)) return settings.routeId !== '';
	return settings.providerId !== '' && settings.modelId !== '';
}

/** The body one turn posts: the shape the management API documents, built from a thread's
 *  settings and the text so far. */
export function chatRequest(
	settings: ChatSettings,
	prompt: string,
	messages: ChatTesterMessage[]
): ChatTesterRequest {
	const route = usesRoute(settings);
	return {
		mode: settings.mode,
		format: settings.format,
		provider_id: route ? '' : settings.providerId,
		model_id: route ? '' : settings.modelId,
		route_id: route ? settings.routeId : '',
		system: settings.system,
		max_tokens: settings.maxTokens,
		stream: settings.stream,
		messages: [...messages, { role: 'user', content: prompt }]
	};
}

/** Folds one answer into the state a turn shows.
 *  A refusal keeps the error the daemon named, and a reply with no text is its own state, so an
 *  empty answer is never mistaken for one that can carry context. */
export function resultOf(answer: ChatTesterAnswer): ChatTurnResult {
	if (!answer.ok) {
		return {
			status: 'failed',
			text: answer.text ?? '',
			provider: answer.provider,
			model: answer.model,
			requestId: answer.request_id,
			httpStatus: answer.status,
			error: answer.error ?? '',
			errorCode: answer.error_code
		};
	}
	const text = answer.text ?? '';
	return {
		status: text.trim() === '' ? 'empty' : 'success',
		text,
		provider: answer.provider,
		model: answer.model,
		requestId: answer.request_id,
		httpStatus: answer.status,
		inputTokens: answer.input_tokens,
		outputTokens: answer.output_tokens,
		durationMs: answer.duration_ms
	};
}

/** The state a turn takes when the request never reached the daemon, or when the console gave up
 *  waiting for it. */
export function failedResult(error: string, errorCode = ''): ChatTurnResult {
	return { status: 'failed', text: '', error, errorCode };
}

/** The state a turn takes when the wait was taken back.
 *  A streamed reply that was stopped keeps what it had already shown, so the text an operator
 *  watched arrive does not vanish with the request. */
export function interruptedResult(text = ''): ChatTurnResult {
	return { status: 'interrupted', text };
}

/** Reads one stored value. It answers null for a value it cannot trust — text that is not JSON,
 *  a different version, or a shape that is not a store — so the caller can say so rather than
 *  overwrite it. */
export function parseChatStore(raw: string | null): ChatStore | null {
	if (raw === null || raw.trim() === '') return null;
	let parsed: unknown;
	try {
		parsed = JSON.parse(raw);
	} catch {
		return null;
	}
	if (!isRecord(parsed)) return null;
	if (numberOf(parsed.version, 0) !== CHAT_STORE_VERSION) return null;
	if (!Array.isArray(parsed.threads)) return null;
	const threads = parsed.threads
		.map(normalizeThread)
		.filter((thread): thread is ChatThread => thread !== null);
	return { version: CHAT_STORE_VERSION, activeThreadId: textOf(parsed.activeThreadId), threads };
}

/** Loads the history and names the one thing that could stop it from being saved: a stored value
 *  that cannot be read, or storage the browser will not hand over. */
export function readChatStore(storage: StorageLike | undefined = browserStorage()): {
	store: ChatStore;
	notice: ChatStorageNotice;
} {
	if (storage === undefined) return { store: emptyStore(), notice: 'blocked' };
	let raw: string | null;
	try {
		raw = storage.getItem(CHAT_STORAGE_KEY);
	} catch {
		return { store: emptyStore(), notice: 'blocked' };
	}
	if (raw === null) return { store: emptyStore(), notice: '' };
	const parsed = parseChatStore(raw);
	if (parsed === null) return { store: emptyStore(), notice: 'corrupt' };
	return { store: settle(parsed), notice: '' };
}

/** Saves the history. It answers false when the browser refuses the write — storage denied, or
 *  the quota full — which is what tells the console to say that a conversation will not survive
 *  a reload. */
export function writeChatStore(
	store: ChatStore,
	storage: StorageLike | undefined = browserStorage()
): boolean {
	if (storage === undefined) return false;
	try {
		storage.setItem(CHAT_STORAGE_KEY, JSON.stringify(settle(store)));
		return true;
	} catch {
		return false;
	}
}

/** Removes the one key the console owns, and answers whether the browser allowed it. */
export function clearChatStore(storage: StorageLike | undefined = browserStorage()): boolean {
	if (storage === undefined) return false;
	try {
		storage.removeItem(CHAT_STORAGE_KEY);
		return true;
	} catch {
		return false;
	}
}

/** Which of the two side panes the chat page shows beside the conversation.
 *  It is a layout, not history, so it lives under its own key. */
export interface ChatPanels {
	threads: boolean;
	settings: boolean;
}

/** The second key the console owns on this page.
 *  Like the history key it carries its version, so a later shape is a new key rather than a
 *  migration that could misread an old value. */
export const CHAT_PANELS_KEY = 'relo.chat.panels.v1';

/** Answers the panes to show. Anything unreadable — no stored value, storage the browser will
 *  not hand over, a value from another shape — falls back to the defaults, because a layout is
 *  not worth interrupting the page over. */
export function readChatPanels(
	defaults: ChatPanels,
	storage: StorageLike | undefined = browserStorage()
): ChatPanels {
	const raw = readPanelsValue(storage);
	if (raw === undefined) return defaults;
	return {
		threads: typeof raw.threads === 'boolean' ? raw.threads : defaults.threads,
		settings: typeof raw.settings === 'boolean' ? raw.settings : defaults.settings
	};
}

/** Saves the layout and answers whether it was kept. */
export function writeChatPanels(
	panels: ChatPanels,
	storage: StorageLike | undefined = browserStorage()
): boolean {
	if (storage === undefined) return false;
	try {
		storage.setItem(CHAT_PANELS_KEY, JSON.stringify(panels));
		return true;
	} catch {
		return false;
	}
}

function readPanelsValue(storage: StorageLike | undefined): Record<string, unknown> | undefined {
	if (storage === undefined) return undefined;
	let raw: string | null;
	try {
		raw = storage.getItem(CHAT_PANELS_KEY);
	} catch {
		return undefined;
	}
	if (raw === null || raw.trim() === '') return undefined;
	try {
		const parsed: unknown = JSON.parse(raw);
		return isRecord(parsed) ? parsed : undefined;
	} catch {
		return undefined;
	}
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null;
}

function textOf(value: unknown, fallback = ''): string {
	return typeof value === 'string' ? value : fallback;
}

function optionalText(value: unknown): string | undefined {
	return typeof value === 'string' && value !== '' ? value : undefined;
}

function numberOf(value: unknown, fallback: number): number {
	return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function optionalNumber(value: unknown): number | undefined {
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function isFormat(value: unknown): value is ChatFormat {
	return value === 'openai-chat' || value === 'openai-responses' || value === 'anthropic';
}

function isStatus(value: unknown): value is ChatTurnStatus {
	return (
		value === 'pending' ||
		value === 'success' ||
		value === 'empty' ||
		value === 'failed' ||
		value === 'interrupted'
	);
}

function normalizeThread(value: unknown): ChatThread | null {
	if (!isRecord(value)) return null;
	const id = textOf(value.id);
	if (id === '') return null;
	if (!Array.isArray(value.turns)) return null;
	const createdAt = numberOf(value.createdAt, 0);
	return {
		id,
		title: textOf(value.title),
		createdAt,
		updatedAt: numberOf(value.updatedAt, createdAt),
		draft: textOf(value.draft),
		settings: normalizeSettings(value.settings),
		turns: value.turns.map(normalizeTurn).filter((turn): turn is ChatTurn => turn !== null)
	};
}

// A target stored before the tester could stream carries no stream field,
// which reads as the one reply the tester asked for then.
function normalizeSettings(value: unknown): ChatSettings {
	const defaults = defaultChatSettings();
	if (!isRecord(value)) return defaults;
	return {
		mode: value.mode === 'format' ? 'format' : 'direct',
		target: value.target === 'route' ? 'route' : 'model',
		format: isFormat(value.format) ? value.format : defaults.format,
		providerId: textOf(value.providerId),
		modelId: textOf(value.modelId),
		routeId: textOf(value.routeId),
		system: textOf(value.system),
		maxTokens: numberOf(value.maxTokens, defaults.maxTokens),
		stream: value.stream === true
	};
}

function normalizeTurn(value: unknown): ChatTurn | null {
	if (!isRecord(value)) return null;
	const id = textOf(value.id);
	if (id === '') return null;
	return {
		id,
		prompt: textOf(value.prompt),
		at: numberOf(value.at, 0),
		result: normalizeResult(value.result)
	};
}

function normalizeResult(value: unknown): ChatTurnResult {
	if (!isRecord(value)) return failedResult('');
	return {
		status: isStatus(value.status) ? value.status : 'failed',
		text: textOf(value.text),
		provider: optionalText(value.provider),
		model: optionalText(value.model),
		requestId: optionalText(value.requestId),
		httpStatus: optionalNumber(value.httpStatus),
		error: optionalText(value.error),
		errorCode: optionalText(value.errorCode),
		inputTokens: optionalNumber(value.inputTokens),
		outputTokens: optionalNumber(value.outputTokens),
		durationMs: optionalNumber(value.durationMs)
	};
}
