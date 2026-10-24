// Reading a captured agent request back into the turns the Chat tester can
// carry. The capture is the raw wire payload an agent sent, in one of the three
// dialects Relo speaks; the tester's conversation is text-only, so the reader
// keeps the words and names what it had to drop rather than hiding it.

import {
	newId,
	type ChatFormat,
	type ChatSettings,
	type ChatThread,
	type ChatThreadSeed,
	type ChatTurn
} from './chat-threads';
import type { CapturedEntry, CapturedPart, ChatTesterMessage } from './types';

/** A captured request read as the seed of a chat thread. */
export interface CapturedChatSeed {
	/** The connection the request went to, which the caller names because the
	 *  wire payload does not carry it. */
	providerId: string;
	/** The model the body asked for. */
	modelId: string;
	/** The dialect the body is written in, which is what a re-send must speak. */
	format: ChatFormat;
	/** The instructions the body carried outside its turns. */
	system: string;
	maxTokens: number;
	stream: boolean;
	/** Every completed exchange, oldest first. */
	messages: ChatTesterMessage[];
	/** The full exchange as the client sent it, including tool turns the tester
	 *  cannot re-send. Read-only: it is what the transcript draws, not what
	 *  the next request carries. */
	entries: CapturedEntry[];
	/** The trailing user message, prefilled in the composer rather than sent: a
	 *  link must never spend tokens without a keypress. */
	draft: string;
	/** The request the seed came from, so the page can say so. */
	sourceRequestId: string;
	/** The parts the tester cannot carry, named so the page can state them. */
	dropped: CapturedDrop[];
}

/** A part of the captured body a text-only conversation cannot hold. */
export type CapturedDrop = 'tools' | 'images' | 'thinking';

/** The dialects a captured body can be written in, as the surface names them. */
const SURFACE_FORMAT: Record<string, ChatFormat> = {
	'chat-completions': 'openai-chat',
	responses: 'openai-responses',
	messages: 'anthropic'
};

/** Names the surface a captured URL belongs to, which is more reliable than
 *  sniffing the body when two dialects share a field name. */
export function surfaceOfURL(url: string): string {
	const path = url.toLowerCase().split('?')[0];
	if (path.endsWith('/chat/completions')) return 'chat-completions';
	if (path.endsWith('/responses')) return 'responses';
	if (path.endsWith('/messages')) return 'messages';
	return '';
}

/** Reads a captured agent-request body as the seed of one conversation.
 *  A body this build has no reader for answers null, and the caller leaves the
 *  action off rather than seeding half a request. */
export function seedFromCapture(
	body: string,
	options: { providerId: string; surface?: string; requestId?: string }
): CapturedChatSeed | null {
	let parsed: unknown;
	try {
		parsed = JSON.parse(body);
	} catch {
		return null;
	}
	if (!isRecord(parsed)) return null;
	const format = SURFACE_FORMAT[options.surface ?? ''] ?? formatOf(parsed);
	if (format === null) return null;

	const turns =
		format === 'openai-responses'
			? readResponsesInput(parsed)
			: readMessages(parsed.messages);
	const split = splitTrailingUser(turns);
	const system = readSystem(parsed.system, format, parsed.messages);
	const dropped = droppedParts(parsed);
	const entries = readEntries(parsed, format);
	return {
		providerId: options.providerId,
		modelId: textOf(parsed.model),
		format,
		system,
		maxTokens: readMaxTokens(parsed, format),
		stream: parsed.stream === true,
		messages: split.messages,
		entries,
		draft: split.draft,
		sourceRequestId: options.requestId ?? '',
		dropped
	};
}

/** Names the dialect a body is written in by its own shape, for a capture whose
 *  URL does not say. */
function formatOf(body: Record<string, unknown>): ChatFormat | null {
	if (Array.isArray(body.input)) return 'openai-responses';
	if (!Array.isArray(body.messages)) return null;
	// An Anthropic body always carries max_tokens, and its system is either a
	// string or a list of blocks at the top level. A chat body puts the system
	// in the messages themselves.
	const system = body.system;
	if (body.max_tokens !== undefined) return 'anthropic';
	if (typeof system === 'string' || Array.isArray(system)) return 'anthropic';
	return 'openai-chat';
}

/** Reads a list of turns from either dialect's `messages`. */
function readMessages(value: unknown): ChatTesterMessage[] {
	if (!Array.isArray(value)) return [];
	const turns: ChatTesterMessage[] = [];
	for (const entry of value) {
		if (!isRecord(entry)) continue;
		const role = textOf(entry.role, 'user');
		if (role !== 'user' && role !== 'assistant') continue;
		const content = textOfBlocks(entry.content);
		if (content === '') continue;
		turns.push({ role, content });
	}
	return turns;
}

/** Reads a Responses body's `input`, which is either one string or a list of
 *  turns whose content is a list of typed parts. */
function readResponsesInput(body: Record<string, unknown>): ChatTesterMessage[] {
	const input = body.input;
	if (typeof input === 'string') {
		return input.trim() === '' ? [] : [{ role: 'user', content: input }];
	}
	if (!Array.isArray(input)) return [];
	const turns: ChatTesterMessage[] = [];
	for (const entry of input) {
		if (!isRecord(entry)) continue;
		const role = textOf(entry.role, 'user');
		if (role !== 'user' && role !== 'assistant') continue;
		const content = textOfBlocks(entry.content);
		if (content === '') continue;
		turns.push({ role, content });
	}
	return turns;
}

/** Takes the words out of a content value: a plain string, or the text parts of
 *  a block list. Images and thinking blocks are left for droppedParts to name. */
function textOfBlocks(value: unknown): string {
	if (typeof value === 'string') return value;
	if (!Array.isArray(value)) return '';
	const parts: string[] = [];
	for (const block of value) {
		if (!isRecord(block)) continue;
		const kind = textOf(block.type);
		if (kind === 'text' || kind === 'input_text' || kind === 'output_text') {
			const text = textOf(block.text);
			if (text !== '') parts.push(text);
		}
	}
	return parts.join('\n');
}

/** The instructions the body carried outside its turns. In a chat body they are
 *  the `system` entries the messages carry instead. */
function readSystem(value: unknown, format: ChatFormat, messages: unknown): string {
	if (format === 'anthropic' || format === 'openai-responses') {
		const own = textOfBlocks(value);
		if (own !== '') return own;
	}
	if (!Array.isArray(messages)) return '';
	const parts: string[] = [];
	for (const entry of messages) {
		if (!isRecord(entry)) continue;
		if (textOf(entry.role) !== 'system' && textOf(entry.role) !== 'developer') continue;
		const text = textOfBlocks(entry.content);
		if (text !== '') parts.push(text);
	}
	return parts.join('\n\n');
}

/** What the body asked for, whichever spelling its dialect uses. */
function readMaxTokens(body: Record<string, unknown>, format: ChatFormat): number {
	const candidates =
		format === 'openai-responses'
			? [body.max_output_tokens, body.max_tokens]
			: [body.max_tokens, body.max_completion_tokens];
	for (const candidate of candidates) {
		if (typeof candidate === 'number' && Number.isFinite(candidate) && candidate > 0) {
			return candidate;
		}
	}
	return 1024;
}

/** Splits a turn list into the exchanges already done and one last prompt the
 *  operator sends. A body that ends on the assistant's turn has nothing to
 *  prefill. */
function splitTrailingUser(turns: ChatTesterMessage[]): {
	messages: ChatTesterMessage[];
	draft: string;
} {
	const last = turns[turns.length - 1];
	if (last === undefined || last.role !== 'user') return { messages: turns, draft: '' };
	return { messages: turns.slice(0, -1), draft: last.content };
}

/** Reads the full exchange a client sent, including tool turns the tester
 *  cannot re-send. Returns the turns in order, one per message the client
 *  carried. */
function readEntries(body: Record<string, unknown>, format: ChatFormat): CapturedEntry[] {
	if (format === 'openai-responses') return readResponsesEntries(body);
	if (format === 'anthropic') return readAnthropicEntries(body.messages);
	return readChatEntries(body.messages);
}

function readChatEntries(value: unknown): CapturedEntry[] {
	if (!Array.isArray(value)) return [];
	const entries: CapturedEntry[] = [];
	for (const entry of value) {
		if (!isRecord(entry)) continue;
		const role = textOf(entry.role, 'user');
		if (role === 'system' || role === 'developer') continue;
		if (role !== 'user' && role !== 'assistant' && role !== 'tool') continue;
		const parts: CapturedPart[] = [];
		const content = entry.content;
		if (typeof content === 'string' && content.trim() !== '') parts.push({ kind: 'text', text: content });
		else if (Array.isArray(content)) for (const block of content) {
			if (!isRecord(block)) continue;
			const kind = textOf(block.type);
			if (kind === 'text' || kind === 'input_text' || kind === 'output_text') {
				const text = textOf(block.text);
				if (text !== '') parts.push({ kind: 'text', text });
			} else if (kind === 'image' || kind === 'image_url' || kind === 'input_image') parts.push({ kind: 'image' });
			else if (kind === 'thinking' || kind === 'redacted_thinking' || kind === 'reasoning') parts.push({ kind: 'thinking' });
		}
		const calls = entry.tool_calls;
		if (Array.isArray(calls)) for (const call of calls) {
			if (!isRecord(call)) continue;
			const fn = isRecord(call.function) ? call.function : call;
			parts.push({
				kind: 'tool_call',
				id: textOf(call.id),
				name: textOf(fn.name),
				arguments: typeof fn.arguments === 'string' ? fn.arguments : JSON.stringify(fn.arguments ?? '')
			});
		}
		if (role === 'tool') {
			const text = typeof content === 'string' ? content : textOfBlocks(content);
			parts.unshift({ kind: 'tool_result', callId: textOf(entry.tool_call_id), name: textOf(entry.name), text });
		}
		if (parts.length > 0) entries.push({ role: role as CapturedEntry['role'], parts });
	}
	return entries;
}

function readAnthropicEntries(value: unknown): CapturedEntry[] {
	if (!Array.isArray(value)) return [];
	const entries: CapturedEntry[] = [];
	for (const entry of value) {
		if (!isRecord(entry)) continue;
		const role = textOf(entry.role, 'user');
		if (role !== 'user' && role !== 'assistant') continue;
		const content = entry.content;
		const textParts: CapturedPart[] = [];
		const toolCalls: CapturedPart[] = [];
		const toolResults: CapturedPart[] = [];
		if (typeof content === 'string') {
			if (content.trim() !== '') textParts.push({ kind: 'text', text: content });
		} else if (Array.isArray(content)) for (const block of content) {
			if (!isRecord(block)) continue;
			const kind = textOf(block.type);
			if (kind === 'text') {
				const text = textOf(block.text);
				if (text !== '') textParts.push({ kind: 'text', text });
			} else if (kind === 'tool_use') {
				toolCalls.push({
					kind: 'tool_call',
					id: textOf(block.id),
					name: textOf(block.name),
					arguments: typeof block.input === 'string' ? block.input : JSON.stringify(block.input ?? {})
				});
			} else if (kind === 'tool_result') {
				toolResults.push({
					kind: 'tool_result',
					callId: textOf(block.tool_use_id),
					name: '',
					text: textOfBlocks(block.content)
				});
			} else if (kind === 'image' || kind === 'image_url' || kind === 'input_image') textParts.push({ kind: 'image' });
			else if (kind === 'thinking' || kind === 'redacted_thinking') textParts.push({ kind: 'thinking' });
		}
		// Anthropic carries tool results inside user turns: split them into their
		// own cards so the exchange reads the way it happened.
		if (toolResults.length > 0) entries.push({ role: 'tool', parts: toolResults });
		const rest = [...textParts, ...toolCalls];
		if (rest.length > 0) entries.push({ role: role as CapturedEntry['role'], parts: rest });
	}
	return entries;
}

function readResponsesEntries(body: Record<string, unknown>): CapturedEntry[] {
	const input = body.input;
	if (typeof input === 'string') {
		return input.trim() === '' ? [] : [{ role: 'user', parts: [{ kind: 'text', text: input }] }];
	}
	if (!Array.isArray(input)) return [];
	const entries: CapturedEntry[] = [];
	for (const entry of input) {
		if (!isRecord(entry)) continue;
		const type = textOf(entry.type, 'message');
		if (type === 'function_call') {
			entries.push({
				role: 'assistant',
				parts: [{
					kind: 'tool_call',
					id: textOf(entry.call_id ?? entry.id),
					name: textOf(entry.name),
					arguments: typeof entry.arguments === 'string' ? entry.arguments : JSON.stringify(entry.arguments ?? '')
				}]
			});
			continue;
		}
		if (type === 'function_call_output') {
			entries.push({
				role: 'tool',
				parts: [{
					kind: 'tool_result',
					callId: textOf(entry.call_id),
					name: '',
					text: textOf(entry.output)
				}]
			});
			continue;
		}
		if (type === 'reasoning') { entries.push({ role: 'assistant', parts: [{ kind: 'thinking' }] }); continue; }
		const role = textOf(entry.role, 'user');
		if (role !== 'user' && role !== 'assistant' && role !== 'system') continue;
		const content = entry.content;
		const parts: CapturedPart[] = [];
		if (typeof content === 'string' && content.trim() !== '') parts.push({ kind: 'text', text: content });
		else if (Array.isArray(content)) for (const block of content) {
			if (!isRecord(block)) continue;
			const kind = textOf(block.type);
			if (kind === 'input_text' || kind === 'output_text' || kind === 'text') {
				const text = textOf(block.text);
				if (text !== '') parts.push({ kind: 'text', text });
			} else if (kind === 'image' || kind === 'input_image') parts.push({ kind: 'image' });
			else if (kind === 'reasoning') parts.push({ kind: 'thinking' });
		}
		if (parts.length > 0) entries.push({ role: role as CapturedEntry['role'], parts });
	}
	return entries;
}

/** Names every part of the body the tester cannot carry. */
function droppedParts(body: Record<string, unknown>): CapturedDrop[] {
	const dropped: CapturedDrop[] = [];
	if (Array.isArray(body.tools) && body.tools.length > 0) dropped.push('tools');
	if (mentionsBlock(body, ['image', 'image_url', 'input_image'])) dropped.push('images');
	if (mentionsBlock(body, ['thinking', 'redacted_thinking', 'reasoning'])) dropped.push('thinking');
	return dropped;
}

/** Reports whether any content block of the body is one of the named kinds. */
function mentionsBlock(value: unknown, kinds: readonly string[]): boolean {
	if (Array.isArray(value)) return value.some((entry) => mentionsBlock(entry, kinds));
	if (!isRecord(value)) return false;
	const kind = textOf(value.type);
	if (kinds.includes(kind)) return true;
	return Object.entries(value).some(
		([key, entry]) =>
			(key === 'content' || key === 'input' || key === 'messages' || key === 'system') &&
			mentionsBlock(entry, kinds)
	);
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null;
}

function textOf(value: unknown, fallback = ''): string {
	return typeof value === 'string' ? value : fallback;
}

/** The settings a seeded conversation is tested with: the captured request's
 *  own dialect, system prompt, token limit, and stream choice, pointed at the
 *  connection the request went to. A request that named a route is replayed
 *  against either, so the caller passes whichever target it resolved. */
export function settingsOfSeed(
	seed: CapturedChatSeed,
	target: { providerId?: string; routeId?: string } = {}
): ChatSettings {
	const routeId = target.routeId ?? '';
	return {
		mode: 'format',
		target: routeId === '' ? 'model' : 'route',
		format: seed.format,
		providerId: target.providerId ?? seed.providerId,
		modelId: seed.modelId,
		routeId,
		system: seed.system,
		maxTokens: seed.maxTokens,
		stream: seed.stream
	};
}

/** Builds the conversation a seed describes: every completed exchange already in
 *  the transcript, so the next request carries the same history, and the trailing
 *  prompt left in the composer for the operator to send.
 *
 *  The tester replays only successful turns, so each captured assistant answer is
 *  stored as one. A seed with no answer to a prompt cannot be replayed that way,
 *  and its prompt becomes the draft instead of being lost. */
export function threadOfSeed(seed: CapturedChatSeed, settings: ChatSettings): ChatThread {
	const now = Date.now();
	const turns: ChatTurn[] = [];
	let pending = '';
	for (const message of seed.messages) {
		if (message.role === 'user') {
			// A prompt with no answer behind it cannot be a completed turn.
			pending = message.content;
			continue;
		}
		if (pending === '') continue;
		turns.push({
			id: newId(),
			prompt: pending,
			result: { status: 'success', text: message.content },
			at: now
		});
		pending = '';
	}
	return {
		id: newId(),
		title: titleOfSeed(seed, turns),
		createdAt: now,
		updatedAt: now,
		draft: seed.draft || pending,
		settings: { ...settings },
		turns,
		seed: { messages: [...seed.messages], entries: [...(seed.entries ?? [])], sourceRequestId: seed.sourceRequestId }
	};
}

/** Names a seeded conversation by the request it came from, so the rail says
 *  where it started rather than leaving an untitled thread. */
function titleOfSeed(seed: CapturedChatSeed, turns: readonly ChatTurn[]): string {
	const first = turns[0]?.prompt ?? seed.draft;
	const flat = first.split(/\r?\n/).find((line) => line.trim() !== '') ?? '';
	const trimmed = flat.trim().replace(/\s+/g, ' ');
	if (trimmed === '') return '';
	return trimmed.length > 48 ? trimmed.slice(0, 47).trimEnd() + '…' : trimmed;
}
