import { describe, expect, test } from 'vitest';

import {
	buildProbeBody,
	canJumpTo,
	commitBody,
	connectKindOf,
	emptyStepperState,
	followFormatDefaults,
	isDirty,
	presetStepperState,
	stepAfter,
	stepBefore,
	stepsFor,
	validateConnect,
	type StepperState
} from './provider-stepper';
import { signInTemplate, templateOf } from './provider-fixtures';

function stateOf(patch: Partial<StepperState> = {}): StepperState {
	return { ...emptyStepperState(), ...patch };
}

describe('the add stepper', () => {
	test('a new provider takes four steps and a key takes three', () => {
		expect(stepsFor('new')).toEqual(['provider', 'connect', 'verify', 'review']);
		expect(stepsFor('addKey')).toEqual(['provider', 'connect', 'verify']);
		expect(stepsFor('custom')).toEqual(['provider', 'connect', 'verify', 'review']);
	});

	test('a connection already open starts on its connect step', () => {
		const key = presetStepperState(
			{
				id: 'openai',
				templateId: 'openai',
				label: 'OpenAI',
				baseURL: 'https://api.openai.com/v1',
				variables: {}
			},
			templateOf()
		);
		expect(key.path).toBe('addKey');
		expect(key.targetProviderId).toBe('openai');

		const signIn = presetStepperState(
			{
				id: 'claude',
				templateId: 'claude',
				label: 'Claude',
				baseURL: '',
				variables: { region: 'kept' }
			},
			signInTemplate({
				variables: [
					{
						name: 'region',
						label: 'Region',
						placeholder: '',
						required: false,
						options: ['default']
					}
				]
			})
		);
		expect(signIn.path).toBe('signin');
		expect(signIn.targetProviderId).toBe('claude');
		expect(signIn.loginFlow).toBe('claude');
		expect(signIn.variables).toEqual({ region: 'kept' });
		expect(
			presetStepperState(
				{ id: 'gone', templateId: 'gone', label: 'Gone', baseURL: '', variables: {} },
				null
			).path
		).toBe('new');
	});

	test('a sign-in ends at verify, because the login already saved the connection', () => {
		expect(stepsFor('signin')).toEqual(['provider', 'connect', 'verify']);
		expect(stepAfter('signin', 'verify')).toBe('');
		expect(stepBefore('signin', 'verify')).toBe('connect');
		expect(canJumpTo('signin', 'verify', 'connect')).toBe(true);
	});

	test('moves forward and back, and stops at each end', () => {
		expect(stepAfter('new', 'connect')).toBe('verify');
		expect(stepAfter('new', 'review')).toBe('');
		expect(stepBefore('new', 'provider')).toBe('');
		expect(stepBefore('addKey', 'verify')).toBe('connect');
	});

	test('names the connect shape of each provider', () => {
		expect(connectKindOf(signInTemplate())).toBe('signin');
		expect(connectKindOf(templateOf({ id: 'amazon-bedrock', kind: 'cloud' }))).toBe('aws');
		expect(connectKindOf(templateOf({ id: 'google-vertex', kind: 'cloud' }))).toBe('gcp');
		expect(connectKindOf(templateOf({ id: 'azure-openai', kind: 'cloud' }))).toBe('azure');
		expect(connectKindOf(templateOf({ id: 'ollama', kind: 'local' }))).toBe('local');
		expect(connectKindOf(templateOf({ auth: 'none' }))).toBe('none');
		expect(connectKindOf(templateOf())).toBe('api_key');
		expect(connectKindOf(null)).toBe('custom');
	});

	test('a key provider needs a key', () => {
		const problems = validateConnect('api_key', stateOf(), templateOf());
		expect(Object.keys(problems)).toEqual(['credential.secret']);
		expect(
			validateConnect(
				'api_key',
				stateOf({ credential: { ...emptyStepperState().credential, secret: 'sk-1' } }),
				templateOf()
			)
		).toEqual({});
	});

	test('bedrock needs a region and one of its two credentials', () => {
		const template = templateOf({
			id: 'amazon-bedrock',
			kind: 'cloud',
			variables: [{ name: 'region', label: 'Region', placeholder: 'us-east-1', required: true }]
		});
		expect(Object.keys(validateConnect('aws', stateOf(), template)).sort()).toEqual([
			'credential.secret',
			'variables.region'
		]);
		const withKey = stateOf({
			variables: { region: 'us-east-1' },
			credential: { ...emptyStepperState().credential, secret: 'bedrock-key' }
		});
		expect(validateConnect('aws', withKey, template)).toEqual({});
		const withKeys = stateOf({
			variables: { region: 'us-east-1' },
			credential: {
				...emptyStepperState().credential,
				useAccessKeys: true,
				accessKeyId: 'AKIA',
				secretAccessKey: 'secret'
			}
		});
		expect(validateConnect('aws', withKeys, template)).toEqual({});
	});

	test('vertex needs a project, a location and a credential', () => {
		const template = templateOf({
			id: 'google-vertex',
			kind: 'cloud',
			variables: [
				{ name: 'project', label: 'Project', placeholder: 'p', required: true },
				{ name: 'location', label: 'Location', placeholder: 'l', required: true }
			]
		});
		expect(Object.keys(validateConnect('gcp', stateOf(), template)).sort()).toEqual([
			'credential.secret',
			'variables.location',
			'variables.project'
		]);
		const withServiceAccount = stateOf({
			variables: { project: 'p', location: 'us-central1' },
			credential: {
				...emptyStepperState().credential,
				useServiceAccount: true,
				serviceAccount: '{"type":"service_account"}'
			}
		});
		expect(validateConnect('gcp', withServiceAccount, template)).toEqual({});
	});

	test('azure needs a resource, a key and one deployment', () => {
		const template = templateOf({
			id: 'azure-openai',
			kind: 'cloud',
			variables: [{ name: 'resource', label: 'Resource', placeholder: 'r', required: true }]
		});
		expect(Object.keys(validateConnect('azure', stateOf(), template)).sort()).toEqual([
			'credential.secret',
			'deployments',
			'variables.resource'
		]);
		const ready = stateOf({
			variables: { resource: 'team' },
			credential: { ...emptyStepperState().credential, secret: 'azure-key' },
			deployments: [{ id: 'row1', modelId: 'gpt-4o', pricedAs: 'openai/gpt-4o' }]
		});
		expect(validateConnect('azure', ready, template)).toEqual({});
	});

	test('a custom endpoint needs an id, a label and an address', () => {
		expect(Object.keys(validateConnect('custom', stateOf(), null)).sort()).toEqual([
			'custom.baseURL',
			'custom.id',
			'custom.label'
		]);
		const bad = stateOf({ custom: { ...emptyStepperState().custom, id: 'Bad Id' } });
		expect(validateConnect('custom', bad, null)['custom.id']).toBe(
			'ui.pages.providersPage.add.idInvalid'
		);
	});

	test('a local preset needs an address', () => {
		expect(Object.keys(validateConnect('local', stateOf(), templateOf({ kind: 'local' })))).toEqual(
			['baseURL']
		);
	});

	test('a connection with no model list needs at least one id', () => {
		const vertex = templateOf({
			id: 'google-vertex',
			kind: 'cloud',
			variables: [
				{ name: 'project', label: 'Project', placeholder: 'p', required: true },
				{ name: 'location', label: 'Location', placeholder: 'l', required: true }
			],
			models_source: 'manual',
			models_format: 'none'
		});
		const empty = stateOf({
			variables: { project: 'p', location: 'us-central1' },
			credential: { ...emptyStepperState().credential, secret: 'vertex-key' }
		});
		expect(Object.keys(validateConnect('gcp', empty, vertex))).toEqual(['deployments']);
		const typed = stateOf({
			variables: { project: 'p', location: 'us-central1' },
			credential: { ...emptyStepperState().credential, secret: 'vertex-key' },
			deployments: [{ id: 'row2', modelId: 'gemini-3-pro', pricedAs: '' }]
		});
		expect(validateConnect('gcp', typed, vertex)).toEqual({});
	});

	test('a probe names the template it tests', () => {
		const body = buildProbeBody(
			stateOf({
				templateId: 'openai',
				credential: { ...emptyStepperState().credential, secret: 'sk-1' }
			}),
			templateOf(),
			'api_key'
		);
		expect(body.template_id).toBe('openai');
		expect(body.credential).toEqual({ kind: 'api_key', secret: 'sk-1' });
		expect(body.provider_id).toBeUndefined();
	});

	test('a probe aimed at an existing provider names that provider', () => {
		const body = buildProbeBody(
			stateOf({
				path: 'addKey',
				targetProviderId: 'openai-2',
				credential: { ...emptyStepperState().credential, secret: 'sk-2' }
			}),
			templateOf(),
			'api_key'
		);
		expect(body.provider_id).toBe('openai-2');
		expect(body.template_id).toBeUndefined();
	});

	test('a custom probe carries its own shape', () => {
		const body = buildProbeBody(
			stateOf({
				path: 'custom',
				custom: {
					id: 'lab',
					label: 'Lab',
					apiFormat: 'openai-chat',
					baseURL: 'https://lab/v1',
					keyHeader: 'x-api-key',
					modelsFormat: 'openai'
				}
			}),
			null,
			'custom'
		);
		expect(body.custom_id).toBe('lab');
		expect(body.key_header).toBe('x-api-key');
		expect(body.base_url).toBe('https://lab/v1');
	});

	test('an AWS probe sends the access keys as the JSON the daemon signs with', () => {
		const body = buildProbeBody(
			stateOf({
				credential: {
					...emptyStepperState().credential,
					useAccessKeys: true,
					accessKeyId: 'AKIA',
					secretAccessKey: 'secret',
					sessionToken: 'token'
				}
			}),
			templateOf({ id: 'amazon-bedrock', kind: 'cloud', auth: 'aws' }),
			'aws'
		);
		const credential = body.credential as { kind: string; secret: string };
		expect(credential.kind).toBe('aws_keys');
		expect(JSON.parse(credential.secret)).toEqual({
			access_key_id: 'AKIA',
			secret_access_key: 'secret',
			session_token: 'token'
		});
	});

	test('a Vertex probe sends the service account as it was pasted', () => {
		const body = buildProbeBody(
			stateOf({
				credential: {
					...emptyStepperState().credential,
					useServiceAccount: true,
					serviceAccount: '{"type":"service_account"}'
				}
			}),
			templateOf({ id: 'google-vertex', kind: 'cloud', auth: 'gcp' }),
			'gcp'
		);
		expect(body.credential).toEqual({
			kind: 'gcp_service_account',
			secret: '{"type":"service_account"}'
		});
	});

	test('an Azure probe carries the deployments and their prices', () => {
		const body = buildProbeBody(
			stateOf({
				deployments: [
					{ id: 'row3', modelId: 'gpt-4o', pricedAs: 'openai/gpt-4o' },
					{ id: 'row4', modelId: '', pricedAs: 'x/y' }
				]
			}),
			templateOf({
				id: 'azure-openai',
				kind: 'cloud',
				models_source: 'manual',
				models_format: 'none'
			}),
			'azure'
		);
		expect(body.manual_models).toEqual([{ model_id: 'gpt-4o', priced_as: 'openai/gpt-4o' }]);
	});

	test('a connection with no model list takes its ids by hand', () => {
		const vertex = templateOf({
			id: 'google-vertex',
			kind: 'cloud',
			models_source: 'manual',
			models_format: 'none'
		});
		const body = buildProbeBody(
			stateOf({ deployments: [{ id: 'row5', modelId: 'gemini-3-pro', pricedAs: '' }] }),
			vertex,
			'gcp'
		);
		expect(body.manual_models).toEqual([{ model_id: 'gemini-3-pro', priced_as: '' }]);
	});

	test('a connection that publishes a list sends no ids of its own', () => {
		const body = buildProbeBody(
			stateOf({ deployments: [{ id: 'row6', modelId: 'typed-by-hand', pricedAs: '' }] }),
			templateOf({ id: 'groq', models_source: 'listing', models_format: 'openai' }),
			'api_key'
		);
		expect(body.manual_models).toBeUndefined();
	});

	test('a commit names the provider, the label and the models left off', () => {
		expect(
			commitBody(stateOf({ accountLabel: 'work' }), {
				providerId: 'openai-2',
				label: 'OpenAI 2',
				disabled: ['gpt-4o']
			})
		).toEqual({
			provider_id: 'openai-2',
			label: 'OpenAI 2',
			account_label: 'work',
			disabled_models: ['gpt-4o']
		});
	});

	test('a commit without an account label generates one', () => {
		const body = commitBody(stateOf(), { providerId: 'openai', label: 'OpenAI', disabled: [] });
		expect(String(body.account_label)).toMatch(/^[A-Z][a-z]+ [A-Z][a-z]+$/);
	});

	test('a format switch follows the previous option defaults', () => {
		const chat = { default_base_url: 'https://api.teamorouter.cn/v1', key_header: 'bearer' };
		const messages = { default_base_url: 'https://api.teamorouter.cn/v1', key_header: 'x-api-key' };
		expect(
			followFormatDefaults(chat, messages, {
				baseURL: chat.default_base_url,
				keyHeader: chat.key_header
			})
		).toEqual({
			baseURL: messages.default_base_url,
			keyHeader: messages.key_header
		});
		expect(
			followFormatDefaults(chat, messages, {
				baseURL: 'https://proxy.example/v1',
				keyHeader: 'api-key'
			})
		).toEqual({
			baseURL: 'https://proxy.example/v1',
			keyHeader: 'api-key'
		});
	});

	test('nothing typed is not dirty, and anything typed is', () => {
		expect(isDirty(stateOf())).toBe(false);
		expect(
			isDirty(stateOf({ credential: { ...emptyStepperState().credential, secret: 'sk-1' } }))
		).toBe(true);
		expect(isDirty(stateOf({ accountLabel: 'work' }))).toBe(true);
		expect(
			isDirty(stateOf({ deployments: [{ id: 'row7', modelId: 'gpt-4o', pricedAs: '' }] }))
		).toBe(true);
		expect(isDirty(stateOf({ custom: { ...emptyStepperState().custom, id: 'lab' } }))).toBe(true);
	});
});
