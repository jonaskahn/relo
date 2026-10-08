import { describe, expect, it, vi } from 'vitest';

import {
	createChatStreamParser,
	isChatTesterAnswer,
	readChatStream,
	type ChatStreamFrame
} from './chat-stream';

// A delta frame as the daemon writes it.
function delta(text: string): string {
	return 'data: ' + JSON.stringify({ type: 'delta', text }) + '\n\n';
}

// The frame that closes a turn, as the daemon writes it: every field
// chatTesterAnswer serialises without an omitempty, counters included.
function answerFrame(text: string) {
	return {
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
	};
}

// The frame above, on the wire.
function answer(text: string): string {
	return 'data: ' + JSON.stringify(answerFrame(text)) + '\n\n';
}

describe('reading a streamed answer', () => {
	it('reads every frame of one chunk in order', () => {
		const frames = createChatStreamParser().push(
			delta('Hello') + delta(' world') + answer('Hello world')
		);
		expect(frames).toEqual([
			{ type: 'delta', text: 'Hello' },
			{ type: 'delta', text: ' world' },
			answerFrame('Hello world')
		]);
	});

	it('holds a frame that arrives in pieces until it is whole', () => {
		const parser = createChatStreamParser();
		const frame = delta('half') + answer('half an answer');
		expect(parser.push(frame.slice(0, 12))).toEqual([]);
		expect(parser.push(frame.slice(12, 24))).toEqual([]);
		expect(parser.push(frame.slice(24))).toEqual([
			{ type: 'delta', text: 'half' },
			answerFrame('half an answer')
		]);
	});

	it('joins the data lines of one frame, which is how a multi-line payload arrives', () => {
		const frames = createChatStreamParser().push(
			'data: {"type":"delta",\ndata: "text":"two lines"}\n\n'
		);
		expect(frames).toEqual([{ type: 'delta', text: 'two lines' }]);
	});

	it('skips a comment, another frame type, and text that is not JSON', () => {
		const frames = createChatStreamParser().push(
			': keep-alive\n\nevent: ping\n\ndata: {not json\n\ndata: {"type":"later"}\n\n'
		);
		expect(frames).toEqual([]);
	});

	it('reads a value that arrived with the carriage returns another writer sends', () => {
		const frames = createChatStreamParser().push('data: {"type":"delta","text":"windows"}\r\n\r\n');
		expect(frames).toEqual([{ type: 'delta', text: 'windows' }]);
	});
});

describe('reading a streamed answer from a body', () => {
	it('hands every frame to the caller as the stream is read', async () => {
		const chunks = [delta('one'), delta(' two'), answer('one two')];
		const body = new ReadableStream<Uint8Array>({
			start(controller) {
				for (const chunk of chunks) controller.enqueue(new TextEncoder().encode(chunk));
				controller.close();
			}
		});
		const seen: ChatStreamFrame[] = [];
		await readChatStream(body, (frame) => seen.push(frame));
		expect(seen).toEqual([
			{ type: 'delta', text: 'one' },
			{ type: 'delta', text: ' two' },
			answerFrame('one two')
		]);
	});

	it('lets go of the body once the stream is read', async () => {
		const body = new ReadableStream<Uint8Array>({
			start(controller) {
				controller.enqueue(new TextEncoder().encode(delta('one')));
				controller.close();
			}
		});
		const seen = vi.fn();
		await readChatStream(body, seen);
		expect(seen).toHaveBeenCalledOnce();
		expect(body.locked).toBe(false);
	});
});

describe('reading an answer a closing frame carries', () => {
	it('accepts every field the daemon always writes and refuses a gap in one', () => {
		expect(isChatTesterAnswer(answerFrame('hi').answer)).toBe(true);
		expect(isChatTesterAnswer({ ...answerFrame('hi').answer, duration_ms: 'soon' })).toBe(false);
		expect(isChatTesterAnswer({ request_id: 'r1', ok: true })).toBe(false);
		expect(isChatTesterAnswer('hi')).toBe(false);
		expect(isChatTesterAnswer(null)).toBe(false);
	});

	it('reads an answer that names its connection, and refuses one field of the wrong kind', () => {
		const answer = answerFrame('hi').answer;
		expect(isChatTesterAnswer({ ...answer, provider: 'openai', model: 'gpt' })).toBe(true);
		expect(isChatTesterAnswer({ ...answer, model: 42 })).toBe(false);
		expect(isChatTesterAnswer({ ...answer, error_code: null })).toBe(false);
	});
});
