// The chat tester answers a turn frame by frame when the conversation asks it
// to: one frame per piece of the reply, and one frame at the end carrying the
// answer the turn is settled with. This module reads that wire, apart from the
// page, so the parsing holds under a test.

import type { ChatTesterAnswer } from './types';

/** One frame of a streamed tester answer: text the provider wrote, or the answer that closed the
 *  turn. */
export type ChatStreamFrame =
	{ type: 'delta'; text: string } | { type: 'answer'; answer: ChatTesterAnswer };

/** Reads the frames one stream carries, however they are split across the chunks the reader
 *  hands it. */
export interface ChatStreamParser {
	push(chunk: string): ChatStreamFrame[];
}

/** Returns a parser for one stream. A frame that carries nothing this console knows — a comment,
 *  another frame type, or text that is not JSON — is skipped rather than failing the turn, so a
 *  later daemon may add to the wire without breaking an older console. */
export function createChatStreamParser(): ChatStreamParser {
	let rest = '';
	return {
		push(chunk: string): ChatStreamFrame[] {
			rest += chunk.replace(/\r\n/g, '\n');
			const frames: ChatStreamFrame[] = [];
			for (;;) {
				const boundary = rest.indexOf('\n\n');
				if (boundary === -1) return frames;
				const raw = rest.slice(0, boundary);
				rest = rest.slice(boundary + 2);
				const frame = parseFrame(raw);
				if (frame !== null) frames.push(frame);
			}
		}
	};
}

/** Hands every frame of one stream to the caller as it arrives, which is what lets a reply be
 *  read while it is still being written. */
export async function readChatStream(
	body: ReadableStream<Uint8Array>,
	onFrame: (frame: ChatStreamFrame) => void
): Promise<void> {
	const reader = body.getReader();
	const decoder = new TextDecoder();
	const parser = createChatStreamParser();
	try {
		for (;;) {
			const { done, value } = await reader.read();
			if (done) return;
			for (const frame of parser.push(decoder.decode(value, { stream: true }))) onFrame(frame);
		}
	} finally {
		reader.releaseLock();
	}
}

/** Reports whether the answer a closing frame carries is one the tester can render: every field
 *  the daemon's own answer struct always writes, of the kind it writes it. */
export function isChatTesterAnswer(value: unknown): value is ChatTesterAnswer {
	if (typeof value !== 'object' || value === null) return false;
	return (
		'request_id' in value &&
		typeof value.request_id === 'string' &&
		'ok' in value &&
		typeof value.ok === 'boolean' &&
		'text' in value &&
		typeof value.text === 'string' &&
		'status' in value &&
		typeof value.status === 'number' &&
		'input_tokens' in value &&
		typeof value.input_tokens === 'number' &&
		'output_tokens' in value &&
		typeof value.output_tokens === 'number' &&
		'duration_ms' in value &&
		typeof value.duration_ms === 'number' &&
		(!('provider' in value) || typeof value.provider === 'string') &&
		(!('model' in value) || typeof value.model === 'string') &&
		(!('route_reason' in value) || typeof value.route_reason === 'string') &&
		(!('error' in value) || typeof value.error === 'string') &&
		(!('error_code' in value) || typeof value.error_code === 'string')
	);
}

/** Reads the three fields a frame body can carry, claiming no shape it has not seen. */
function readFrame(value: unknown): { type: unknown; text: unknown; answer: unknown } | null {
	if (typeof value !== 'object' || value === null) return null;
	return {
		type: 'type' in value ? value.type : undefined,
		text: 'text' in value ? value.text : undefined,
		answer: 'answer' in value ? value.answer : undefined
	};
}

function parseFrame(raw: string): ChatStreamFrame | null {
	const data: string[] = [];
	for (const line of raw.split('\n')) {
		if (!line.startsWith('data:')) continue;
		data.push(line.slice('data:'.length).replace(/^ /, ''));
	}
	if (data.length === 0) return null;
	let parsed: unknown;
	try {
		parsed = JSON.parse(data.join('\n'));
	} catch {
		return null;
	}
	const frame = readFrame(parsed);
	if (frame === null) return null;
	if (frame.type === 'delta' && typeof frame.text === 'string') {
		return { type: 'delta', text: frame.text };
	}
	if (frame.type === 'answer' && isChatTesterAnswer(frame.answer)) {
		return { type: 'answer', answer: frame.answer };
	}
	return null;
}
