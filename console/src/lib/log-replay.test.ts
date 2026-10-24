import { describe, expect, it } from 'vitest';

import { seedFromCapture, settingsOfSeed, surfaceOfURL, threadOfSeed } from './log-replay';

const options = { providerId: 'openai', requestId: '42' };

describe('surfaceOfURL', () => {
	it('names the dialect a captured path belongs to', () => {
		expect(surfaceOfURL('/v1/chat/completions')).toBe('chat-completions');
		expect(surfaceOfURL('/v1/responses?stream=true')).toBe('responses');
		expect(surfaceOfURL('/v1/messages')).toBe('messages');
		expect(surfaceOfURL('/v1/embeddings')).toBe('');
	});
});

describe('seedFromCapture', () => {
	it('reads an OpenAI chat body as turns and a prefilled prompt', () => {
		const body = JSON.stringify({
			model: 'gpt-5',
			max_tokens: 512,
			stream: true,
			messages: [
				{ role: 'system', content: 'You are terse.' },
				{ role: 'user', content: 'What is Relo?' },
				{ role: 'assistant', content: 'A relay.' },
				{ role: 'user', content: 'Say more.' }
			]
		});
		const seed = seedFromCapture(body, { ...options, surface: 'chat-completions' });
		expect(seed).not.toBeNull();
		expect(seed?.format).toBe('openai-chat');
		expect(seed?.modelId).toBe('gpt-5');
		expect(seed?.system).toBe('You are terse.');
		expect(seed?.maxTokens).toBe(512);
		expect(seed?.stream).toBe(true);
		expect(seed?.messages).toEqual([
			{ role: 'user', content: 'What is Relo?' },
			{ role: 'assistant', content: 'A relay.' }
		]);
		// The last user message is prefilled, not sent: a link never spends tokens.
		expect(seed?.draft).toBe('Say more.');
		expect(seed?.sourceRequestId).toBe('42');
		expect(seed?.dropped).toEqual([]);
	});

	it('reads an Anthropic body, whose system sits outside the turns', () => {
		const body = JSON.stringify({
			model: 'claude-sonnet-4',
			max_tokens: 2048,
			system: 'Be brief.',
			messages: [
				{ role: 'user', content: [{ type: 'text', text: 'Hello' }] },
				{ role: 'assistant', content: [{ type: 'text', text: 'Hi' }] }
			]
		});
		const seed = seedFromCapture(body, { ...options, surface: 'messages' });
		expect(seed?.format).toBe('anthropic');
		expect(seed?.system).toBe('Be brief.');
		expect(seed?.maxTokens).toBe(2048);
		expect(seed?.messages).toEqual([
			{ role: 'user', content: 'Hello' },
			{ role: 'assistant', content: 'Hi' }
		]);
		// A body that ends on the answer has nothing to prefill.
		expect(seed?.draft).toBe('');
	});

	it('reads a Responses body, whose input may be one string', () => {
		const list = seedFromCapture(
			JSON.stringify({
				model: 'gpt-5',
				max_output_tokens: 256,
				input: [
					{ role: 'user', content: [{ type: 'input_text', text: 'One' }] },
					{ role: 'assistant', content: [{ type: 'output_text', text: 'Two' }] },
					{ role: 'user', content: [{ type: 'input_text', text: 'Three' }] }
				]
			}),
			{ ...options, surface: 'responses' }
		);
		expect(list?.format).toBe('openai-responses');
		expect(list?.maxTokens).toBe(256);
		expect(list?.draft).toBe('Three');

		const single = seedFromCapture(JSON.stringify({ model: 'gpt-5', input: 'Just this' }), {
			...options,
			surface: 'responses'
		});
		expect(single?.messages).toEqual([]);
		expect(single?.draft).toBe('Just this');
	});

	it('names the parts a text conversation cannot carry', () => {
		const seed = seedFromCapture(
			JSON.stringify({
				model: 'gpt-5',
				tools: [{ type: 'function', function: { name: 'read' } }],
				messages: [
					{
						role: 'user',
						content: [
							{ type: 'text', text: 'Look at this' },
							{ type: 'image_url', image_url: { url: 'data:image/png;base64,AA' } }
						]
					}
				]
			}),
			{ ...options, surface: 'chat-completions' }
		);
		expect(seed?.messages).toEqual([]);
		expect(seed?.draft).toBe('Look at this');
		expect(seed?.dropped).toEqual(['tools', 'images']);
	});

	it('reads a body whose URL does not say which dialect it is', () => {
		expect(
			seedFromCapture(JSON.stringify({ model: 'm', input: [{ role: 'user', content: 'x' }] }), {
				...options
			})?.format
		).toBe('openai-responses');
		expect(
			seedFromCapture(
				JSON.stringify({ model: 'm', system: 's', max_tokens: 5, messages: [] }),
				options
			)?.format
		).toBe('anthropic');
		expect(seedFromCapture(JSON.stringify({ model: 'm', messages: [] }), options)?.format).toBe(
			'openai-chat'
		);
	});

	it('answers null for a body it cannot read', () => {
		expect(seedFromCapture('not json', options)).toBeNull();
		expect(seedFromCapture('[]', options)).toBeNull();
		expect(seedFromCapture('{"model":"m"}', options)).toBeNull();
		// A surface the reader does not know falls back to the body's own shape.
		expect(
			seedFromCapture(JSON.stringify({ model: 'm', messages: [] }), {
				...options,
				surface: 'embeddings'
			})?.format
		).toBe('openai-chat');
	});

	// A dialect's own block shapes are read, and anything else is left to the
	// dropped list rather than being read as text it is not.
	it('reads the text of a block content and skips what is not text', () => {
		const seed = seedFromCapture(
			JSON.stringify({
				model: 'claude',
				max_tokens: 8,
				messages: [
					{
						role: 'user',
						content: [
							{ type: 'text', text: 'One' },
							{ type: 'text', text: 'Two' },
							{ type: 'tool_result', content: 'ignored' }
						]
					}
				]
			}),
			{ ...options, surface: 'messages' }
		);
		expect(seed?.draft).toBe('One\nTwo');
	});

	it('leaves a turn with no text out of the exchange', () => {
		const seed = seedFromCapture(
			JSON.stringify({
				model: 'm',
				messages: [
					{ role: 'user', content: [{ type: 'image_url' }] },
					{ role: 'user', content: 'Kept' }
				]
			}),
			options
		);
		expect(seed?.draft).toBe('Kept');
	});

	it('reads a Responses turn list and names a body with no input', () => {
		expect(
			seedFromCapture(JSON.stringify({ model: 'm', input: 5 }), {
				...options,
				surface: 'responses'
			})?.messages
		).toEqual([]);
		expect(
			seedFromCapture(JSON.stringify({ model: 'm', input: [{ role: 'user', content: 'Only' }] }), {
				...options,
				surface: 'responses'
			})?.draft
		).toBe('Only');
	});

	it('joins the developer instructions a chat body carries as turns', () => {
		const seed = seedFromCapture(
			JSON.stringify({
				model: 'm',
				messages: [
					{ role: 'developer', content: 'Be exact.' },
					{ role: 'system', content: 'And brief.' },
					{ role: 'user', content: 'Hi' }
				]
			}),
			options
		);
		expect(seed?.system).toBe('Be exact.\n\nAnd brief.');
		expect(seed?.draft).toBe('Hi');
	});

	it('names a thinking part it cannot carry', () => {
		const seed = seedFromCapture(
			JSON.stringify({
				model: 'm',
				messages: [
					{
						role: 'user',
						content: [
							{ type: 'thinking', thinking: 'hmm' },
							{ type: 'text', text: 'Hi' }
						]
					}
				]
			}),
			{ ...options, surface: 'messages' }
		);
		expect(seed?.dropped).toContain('thinking');
		expect(seed?.draft).toBe('Hi');
	});

	it('falls back to the tester default when the body names no limit', () => {
		const seed = seedFromCapture(JSON.stringify({ messages: [] }), options);
		expect(seed?.maxTokens).toBe(1024);
		expect(seed?.stream).toBe(false);
	});
});

describe('the conversation a seed builds', () => {
	const seed = {
		providerId: 'openai',
		modelId: 'gpt-5',
		format: 'openai-chat' as const,
		system: 'Be terse.',
		maxTokens: 512,
		stream: true,
		messages: [
			{ role: 'user' as const, content: 'What is Relo?' },
			{ role: 'assistant' as const, content: 'A relay.' }
		],
		draft: 'Say more.',
		sourceRequestId: '42',
		dropped: []
	};

	it('takes the captured target and dialect as its settings', () => {
		const settings = settingsOfSeed(seed);
		expect(settings).toEqual({
			mode: 'format',
			target: 'model',
			format: 'openai-chat',
			providerId: 'openai',
			modelId: 'gpt-5',
			routeId: '',
			system: 'Be terse.',
			maxTokens: 512,
			stream: true
		});
	});

	it('follows the route a resolved request went through', () => {
		const settings = settingsOfSeed(seed, { providerId: 'openai', routeId: 'coding' });
		expect(settings.mode).toBe('format');
		expect(settings.target).toBe('route');
		expect(settings.routeId).toBe('coding');
	});

	it('seeds every exchange and leaves the prompt in the composer', () => {
		const thread = threadOfSeed(seed, settingsOfSeed(seed));
		expect(thread.turns).toHaveLength(1);
		expect(thread.turns[0].prompt).toBe('What is Relo?');
		expect(thread.turns[0].result).toEqual({ status: 'success', text: 'A relay.' });
		expect(thread.draft).toBe('Say more.');
		expect(thread.title).toBe('What is Relo?');
		expect(thread.settings).toEqual(settingsOfSeed(seed));
	});

	// The tester only replays a turn that came back with an answer, so an
	// unanswered prompt becomes the draft rather than being dropped.
	it('keeps an unanswered prompt as the draft', () => {
		const thread = threadOfSeed(
			{
				...seed,
				messages: [
					{ role: 'user', content: 'First' },
					{ role: 'user', content: 'Second' }
				],
				draft: ''
			},
			settingsOfSeed(seed)
		);
		expect(thread.turns).toEqual([]);
		expect(thread.draft).toBe('Second');
	});

	it('starts a conversation whose first prompt is the draft', () => {
		const thread = threadOfSeed(
			{ ...seed, messages: [], draft: 'Only this' },
			settingsOfSeed(seed)
		);
		expect(thread.turns).toEqual([]);
		expect(thread.draft).toBe('Only this');
		expect(thread.title).toBe('Only this');
	});

	it('reads an Anthropic system given as blocks, and keeps a long title short', () => {
		const parsed = seedFromCapture(
			JSON.stringify({
				model: 'claude',
				max_tokens: 8,
				system: [{ type: 'text', text: 'Be brief.' }],
				messages: [{ role: 'user', content: 'x'.repeat(60) }]
			}),
			{ ...options, surface: 'messages' }
		);
		expect(parsed?.system).toBe('Be brief.');
		const thread = threadOfSeed(parsed!, settingsOfSeed(parsed!));
		expect(thread.title.length).toBe(48);
		expect(thread.title.endsWith('…')).toBe(true);
	});

	it('leaves a conversation untitled when the seed carried no words', () => {
		const thread = threadOfSeed(
			{ ...seed, messages: [], draft: '', system: 'Only instructions' },
			settingsOfSeed(seed)
		);
		expect(thread.title).toBe('');
		expect(thread.draft).toBe('');
	});

	// A body can carry an empty text block, and can open on the assistant's turn.
	// Neither is an exchange, and neither may break the one that follows.
	it('skips an empty block and an answer that opens the body', () => {
		const parsed = seedFromCapture(
			JSON.stringify({
				model: 'm',
				system: [{ type: 'text', text: '' }],
				messages: [
					{ role: 'assistant', content: 'Opens' },
					{
						role: 'user',
						content: [
							{ type: 'text', text: '' },
							{ type: 'text', text: 'Kept' }
						]
					}
				]
			}),
			{ ...options, surface: 'messages' }
		);
		expect(parsed?.system).toBe('');
		expect(parsed?.draft).toBe('Kept');
		const thread = threadOfSeed(parsed!, settingsOfSeed(parsed!));
		expect(thread.turns).toEqual([]);
	});
});
