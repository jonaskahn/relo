import { describe, expect, it } from 'vitest';

import { seedFromCapture, settingsOfSeed, threadOfSeed } from './log-replay';

const options = { providerId: 'openai', requestId: '42' };

/** Reads the drawn exchange out of a body the surface names. */
function entriesOf(body: unknown, surface: string) {
	return seedFromCapture(typeof body === 'string' ? body : JSON.stringify(body), {
		...options,
		surface
	})?.entries;
}

describe('the exchange a chat capture draws', () => {
	it('keeps a plain string turn as one text part and drops the instructions', () => {
		expect(
			entriesOf(
				{
					model: 'm',
					messages: [
						{ role: 'system', content: 'Instructions' },
						{ role: 'developer', content: 'More instructions' },
						{ role: 'user', content: 'Hello' },
						{ role: 'assistant', content: 'Hi' }
					]
				},
				'chat-completions'
			)
		).toEqual([
			{ role: 'user', parts: [{ kind: 'text', text: 'Hello' }] },
			{ role: 'assistant', parts: [{ kind: 'text', text: 'Hi' }] }
		]);
	});

	it('leaves out a turn whose words are blank', () => {
		expect(
			entriesOf(
				{
					model: 'm',
					messages: [
						{ role: 'user', content: '   ' },
						{ role: 'user', content: '' },
						{ role: 'user', content: 5 }
					]
				},
				'chat-completions'
			)
		).toEqual([]);
	});

	it('reads typed blocks, keeping images and thinking as their kinds', () => {
		expect(
			entriesOf(
				{
					model: 'm',
					messages: [
						{
							role: 'user',
							content: [
								{ type: 'text', text: 'Look' },
								{ type: 'input_text', text: 'closer' },
								{ type: 'output_text', text: 'done' },
								{ type: 'text', text: '' },
								{ type: 'image_url', image_url: { url: 'data:image/png;base64,AA' } },
								{ type: 'input_image' },
								{ type: 'reasoning' },
								{ type: 'thinking' },
								{ type: 'redacted_thinking' },
								{ type: 'tool_result', content: 'ignored here' },
								'a bare string is not a block'
							]
						}
					]
				},
				'chat-completions'
			)
		).toEqual([
			{
				role: 'user',
				parts: [
					{ kind: 'text', text: 'Look' },
					{ kind: 'text', text: 'closer' },
					{ kind: 'text', text: 'done' },
					{ kind: 'image' },
					{ kind: 'image' },
					{ kind: 'thinking' },
					{ kind: 'thinking' },
					{ kind: 'thinking' }
				]
			}
		]);
	});

	it('reads tool calls in both spellings and the result that answers them', () => {
		const entries = entriesOf(
			{
				model: 'm',
				messages: [
					{
						role: 'assistant',
						content: '',
						tool_calls: [
							{ id: 'call_1', function: { name: 'read', arguments: '{"path":"a"}' } },
							{ id: 'call_2', name: 'write', arguments: { path: 'b' } },
							{ id: 'call_3', name: 'list' },
							'not a call'
						]
					},
					{ role: 'tool', tool_call_id: 'call_1', name: 'read', content: 'contents' },
					{
						role: 'tool',
						tool_call_id: 'call_2',
						name: 'write',
						content: [{ type: 'text', text: 'written' }]
					},
					{ role: 'narrator', content: 'not a role the transcript draws' }
				]
			},
			'chat-completions'
		);

		expect(entries).toEqual([
			{
				role: 'assistant',
				parts: [
					{ kind: 'tool_call', id: 'call_1', name: 'read', arguments: '{"path":"a"}' },
					{ kind: 'tool_call', id: 'call_2', name: 'write', arguments: '{"path":"b"}' },
					{ kind: 'tool_call', id: 'call_3', name: 'list', arguments: '""' }
				]
			},
			{
				role: 'tool',
				parts: [
					{ kind: 'tool_result', callId: 'call_1', name: 'read', text: 'contents' },
					{ kind: 'text', text: 'contents' }
				]
			},
			{
				role: 'tool',
				parts: [
					{ kind: 'tool_result', callId: 'call_2', name: 'write', text: 'written' },
					{ kind: 'text', text: 'written' }
				]
			}
		]);
	});

	it('answers nothing for a body whose messages are not a list', () => {
		expect(entriesOf({ model: 'm', messages: 'none' }, 'chat-completions')).toEqual([]);
	});
});

describe('the exchange an Anthropic capture draws', () => {
	it('keeps text and tool calls together and splits a result into its own turn', () => {
		expect(
			entriesOf(
				{
					model: 'claude',
					max_tokens: 8,
					messages: [
						{ role: 'user', content: 'Hello' },
						{
							role: 'assistant',
							content: [
								{ type: 'text', text: '' },
								{ type: 'text', text: '' }
							]
						},
						{
							role: 'assistant',
							content: [
								{ type: 'text', text: 'Calling' },
								{ type: 'tool_use', id: 'toolu_1', name: 'read', input: { path: 'a' } },
								{ type: 'tool_use', id: 'toolu_2', name: 'write', input: 'raw input' }
							]
						},
						{
							role: 'user',
							content: [
								{
									type: 'tool_result',
									tool_use_id: 'toolu_1',
									content: [{ type: 'text', text: 'contents' }]
								},
								{ type: 'image' },
								{ type: 'thinking' },
								{ type: 'redacted_thinking' },
								{ type: 'text', text: '' },
								'recognized kinds only'
							]
						},
						{ role: 'system', content: 'not a turn' }
					]
				},
				'messages'
			)
		).toEqual([
			{ role: 'user', parts: [{ kind: 'text', text: 'Hello' }] },
			{
				role: 'assistant',
				parts: [
					{ kind: 'text', text: 'Calling' },
					{ kind: 'tool_call', id: 'toolu_1', name: 'read', arguments: '{"path":"a"}' },
					{ kind: 'tool_call', id: 'toolu_2', name: 'write', arguments: 'raw input' }
				]
			},
			{
				role: 'tool',
				parts: [{ kind: 'tool_result', callId: 'toolu_1', name: '', text: 'contents' }]
			},
			{
				role: 'user',
				parts: [{ kind: 'image' }, { kind: 'thinking' }, { kind: 'thinking' }]
			}
		]);
	});

	it('leaves out a turn with no recognized content', () => {
		expect(
			entriesOf(
				{
					model: 'claude',
					max_tokens: 8,
					messages: [
						{ role: 'user', content: '' },
						{ role: 'user', content: 5 },
						{ role: 'user', content: [{ type: 'tool_use', id: 'toolu_1', name: 'read' }] },
						{ role: 'user', content: 'kept' }
					]
				},
				'messages'
			)
		).toEqual([
			{
				role: 'user',
				parts: [{ kind: 'tool_call', id: 'toolu_1', name: 'read', arguments: '{}' }]
			},
			{ role: 'user', parts: [{ kind: 'text', text: 'kept' }] }
		]);
	});

	it('answers nothing for a body whose messages are not a list', () => {
		expect(entriesOf({ model: 'claude', max_tokens: 8, messages: 5 }, 'messages')).toEqual([]);
	});
});

describe('the exchange a Responses capture draws', () => {
	it('draws each tool call and its output as its own row', () => {
		expect(
			entriesOf(
				{
					model: 'gpt-5',
					input: [
						{ role: 'user', content: 'Hello' },
						{ type: 'reasoning' },
						{ type: 'function_call', id: 'fc_1', name: 'read', arguments: '{"path":"a"}' },
						{ type: 'function_call', call_id: 'fc_2', name: 'write', arguments: { path: 'b' } },
						{ type: 'function_call', call_id: 'fc_3', name: 'list' },
						{ type: 'function_call_output', call_id: 'fc_1', output: 'contents' },
						{ role: 'system', content: 'Instructions' },
						{
							role: 'assistant',
							content: [
								{ type: 'output_text', text: 'Done' },
								{ type: 'text', text: 'Really' },
								{ type: 'input_text', text: 'done' },
								{ type: 'text', text: '' },
								{ type: 'input_image' },
								{ type: 'image' },
								{ type: 'reasoning' },
								{ type: 'unknown' },
								{ type: 'function_call_output' },
								'a bare string is not a block'
							]
						},
						{ role: 'narrator', content: 'not a role the transcript draws' }
					]
				},
				'responses'
			)
		).toEqual([
			{ role: 'user', parts: [{ kind: 'text', text: 'Hello' }] },
			{ role: 'assistant', parts: [{ kind: 'thinking' }] },
			{
				role: 'assistant',
				parts: [{ kind: 'tool_call', id: 'fc_1', name: 'read', arguments: '{"path":"a"}' }]
			},
			{
				role: 'assistant',
				parts: [{ kind: 'tool_call', id: 'fc_2', name: 'write', arguments: '{"path":"b"}' }]
			},
			{
				role: 'assistant',
				parts: [{ kind: 'tool_call', id: 'fc_3', name: 'list', arguments: '""' }]
			},
			{
				role: 'tool',
				parts: [{ kind: 'tool_result', callId: 'fc_1', name: '', text: 'contents' }]
			},
			{ role: 'system', parts: [{ kind: 'text', text: 'Instructions' }] },
			{
				role: 'assistant',
				parts: [
					{ kind: 'text', text: 'Done' },
					{ kind: 'text', text: 'Really' },
					{ kind: 'text', text: 'done' },
					{ kind: 'image' },
					{ kind: 'image' },
					{ kind: 'thinking' }
				]
			}
		]);
	});

	it('reads a single-string input as one user row', () => {
		expect(entriesOf({ model: 'gpt-5', input: 'Just this' }, 'responses')).toEqual([
			{ role: 'user', parts: [{ kind: 'text', text: 'Just this' }] }
		]);
		expect(entriesOf({ model: 'gpt-5', input: '   ' }, 'responses')).toEqual([]);
		expect(entriesOf({ model: 'gpt-5', input: 5 }, 'responses')).toEqual([]);
	});

	it('leaves out a turn with no recognized content', () => {
		expect(
			entriesOf(
				{
					model: 'gpt-5',
					input: [
						{ role: 'user', content: '   ' },
						{ role: 'user', content: 5 },
						{ role: 'user', content: [{ type: 'unknown' }, 'not a block'] },
						{ role: 'user', content: 'kept' }
					]
				},
				'responses'
			)
		).toEqual([{ role: 'user', parts: [{ kind: 'text', text: 'kept' }] }]);
	});
});

describe('a capture the reader only partly understands', () => {
	it('answers null for JSON that is not an object', () => {
		expect(seedFromCapture('5', options)).toBeNull();
		expect(seedFromCapture('"text"', options)).toBeNull();
		expect(seedFromCapture('null', options)).toBeNull();
	});

	it('leaves the request id blank when the caller names none', () => {
		const seed = seedFromCapture(JSON.stringify({ model: 'm', messages: [] }), {
			providerId: 'openai'
		});
		expect(seed?.sourceRequestId).toBe('');
	});

	it('names an Anthropic body by its block system when it carries no limit', () => {
		const asBlocks = seedFromCapture(
			JSON.stringify({
				model: 'claude',
				system: [{ type: 'text', text: 'Be brief.' }],
				messages: []
			}),
			options
		);
		expect(asBlocks?.format).toBe('anthropic');
		expect(asBlocks?.system).toBe('Be brief.');

		const asText = seedFromCapture(
			JSON.stringify({ model: 'claude', system: 'Be brief.', messages: [] }),
			options
		);
		expect(asText?.format).toBe('anthropic');
	});

	it('skips a block and a message that are not records', () => {
		const seed = seedFromCapture(
			JSON.stringify({
				model: 'm',
				messages: [
					'not a message',
					{ role: 'user', content: ['not a block', { type: 'text', text: 'Kept' }] }
				]
			}),
			options
		);
		expect(seed?.draft).toBe('Kept');
	});

	it('leaves an empty instruction turn out of the system prompt', () => {
		const seed = seedFromCapture(
			JSON.stringify({
				model: 'm',
				messages: [
					{ role: 'system', content: '' },
					{ role: 'system', content: [{ type: 'text', text: '' }] },
					{ role: 'user', content: 'Hi' }
				]
			}),
			options
		);
		expect(seed?.system).toBe('');
	});

	it('carries the drawn exchange into the transcript', () => {
		const seed = seedFromCapture(
			JSON.stringify({ model: 'm', messages: [{ role: 'user', content: 'Hi' }] }),
			options
		)!;
		const thread = threadOfSeed(seed, settingsOfSeed(seed));
		expect(thread.seed?.entries).toEqual([{ role: 'user', parts: [{ kind: 'text', text: 'Hi' }] }]);
	});
});
