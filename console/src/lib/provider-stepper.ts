import type { ProviderTemplate, TemplateFormatOption } from '$lib/types';

import { defaultAccountName } from './account-name';

// The add flow as plain state, so every step, every refusal and the exact
// body the daemon probes are decided here rather than in markup. A component
// reads this and renders; it never decides what a step means.

/** Which add flow an operator is in, which decides the steps the flow walks. */
export type StepperPath = 'new' | 'addKey' | 'signin' | 'custom';
/** One page of the add-connection flow. */
export type StepperStep = 'provider' | 'connect' | 'verify' | 'review';

/** A provider that signs in has no credential to test, so its connect step starts a login
 *  instead of a probe. Everything else probes. */
export type ConnectKind =
	'signin' | 'api_key' | 'aws' | 'gcp' | 'azure' | 'local' | 'none' | 'custom';

/** The sign-in form as an operator is filling it. */
export interface CredentialDraft {
	// kind is what the daemon stores the credential as, and what decides
	// which fields below are read.
	kind: 'api_key' | 'aws_keys' | 'gcp_service_account' | 'none' | string;
	secret: string;
	accessKeyId: string;
	secretAccessKey: string;
	sessionToken: string;
	serviceAccount: string;
	// useAccessKeys picks AWS access keys over a Bedrock API key, and
	// useServiceAccount picks a service account over a Vertex API key.
	useAccessKeys: boolean;
	useServiceAccount: boolean;
}

/** Where a template says the service runs.
 *  The id names the row an editor is showing rather than the deployment: it is dropped again by
 *  typedModels, so nothing here travels to the daemon. */
export interface Deployment {
	id: string;
	modelId: string;
	pricedAs: string;
}

/** A template's own field list, for a provider Relo cannot describe in its standard shapes. */
export interface CustomForm {
	id: string;
	label: string;
	apiFormat: string;
	baseURL: string;
	keyHeader: string;
	modelsFormat: string;
}

/** Everything the add-connection flow holds at one moment. */
export interface StepperState {
	path: StepperPath;
	templateId: string;
	// targetProviderId is set when the operator adds a key to a provider that
	// already exists, which is what makes the probe carry provider_id rather
	// than a template.
	targetProviderId: string;
	label: string;
	accountLabel: string;
	apiFormat: string;
	baseURL: string;
	headers: Record<string, string>;
	variables: Record<string, string>;
	credential: CredentialDraft;
	deployments: Deployment[];
	loginFlow: string;
	custom: CustomForm;
}

/** Builds the blank sign-in form. */
export function emptyCredential(): CredentialDraft {
	return {
		kind: 'api_key',
		secret: '',
		accessKeyId: '',
		secretAccessKey: '',
		sessionToken: '',
		serviceAccount: '',
		useAccessKeys: false,
		useServiceAccount: false
	};
}

/** Builds a flow at its first step. */
export function emptyStepperState(): StepperState {
	return {
		path: 'new',
		templateId: '',
		targetProviderId: '',
		label: '',
		accountLabel: '',
		apiFormat: '',
		baseURL: '',
		headers: {},
		variables: {},
		credential: emptyCredential(),
		deployments: [],
		loginFlow: '',
		custom: {
			id: '',
			label: '',
			apiFormat: 'openai-chat',
			baseURL: '',
			keyHeader: 'bearer',
			modelsFormat: 'openai'
		}
	};
}

/** Names the shape of the connect step, which decides the fields it shows and the checks it
 *  runs. */
export function connectKindOf(template: ProviderTemplate | null): ConnectKind {
	if (template === null) return 'custom';
	switch (template.id) {
		case 'amazon-bedrock':
			return 'aws';
		case 'google-vertex':
			return 'gcp';
		case 'azure-openai':
			return 'azure';
	}
	switch (template.kind) {
		case 'signin':
			return 'signin';
		case 'local':
			return 'local';
	}
	if (template.auth === 'none') return 'none';
	return 'api_key';
}

/** Reports whether a connection publishes no model list of its own, which is the only case where
 *  a model id is the operator's to type: the ids are then the whole roster, and a catalog only
 *  describes them. */
export function declaresNoList(state: StepperState, template: ProviderTemplate | null): boolean {
	if (template === null) return state.custom.modelsFormat === 'none';
	return template.models_source === 'manual' || template.models_format === 'none';
}

/** Lists the steps of one path. Adding a key to a provider that already exists has no Review
 *  step: there is no new provider to name and no roster to decide about, because the provider
 *  keeps the one it has.
 *  A sign-in has none either: the login already stored the connection and the account, so there
 *  is nothing left to name or to save. */
export function stepsFor(path: StepperPath): StepperStep[] {
	if (path === 'addKey' || path === 'signin') return ['provider', 'connect', 'verify'];
	return ['provider', 'connect', 'verify', 'review'];
}

/** Where one step sits in the path a provider's sign-in takes. */
export function stepIndex(path: StepperPath, step: StepperStep): number {
	return stepsFor(path).indexOf(step);
}

/** Returns the next step, or an empty string at the end of the path. */
export function stepAfter(path: StepperPath, step: StepperStep): StepperStep | '' {
	const steps = stepsFor(path);
	const index = steps.indexOf(step);
	if (index < 0 || index + 1 >= steps.length) return '';
	return steps[index + 1];
}

/** Returns the previous step, or an empty string at the start. */
export function stepBefore(path: StepperPath, step: StepperStep): StepperStep | '' {
	const steps = stepsFor(path);
	const index = steps.indexOf(step);
	if (index <= 0) return '';
	return steps[index - 1];
}

/** Reports whether a finished step is one the operator may return to, which every step behind
 *  the current one is. */
export function canJumpTo(path: StepperPath, current: StepperStep, target: StepperStep): boolean {
	return stepIndex(path, target) < stepIndex(path, current);
}

/** Returns the fields that still need a value, named the way the form marks them.
 *  An empty object means the step may continue. */
export function validateConnect(
	kind: ConnectKind,
	state: StepperState,
	template: ProviderTemplate | null
): Record<string, string> {
	const problems: Record<string, string> = {};
	const needs = (_field: string) => 'ui.pages.providersPage.verify.fieldRequired';

	switch (kind) {
		case 'custom': {
			if (state.custom.id.trim() === '') problems['custom.id'] = needs('custom.id');
			else if (!/^[a-z0-9][a-z0-9._-]{0,63}$/.test(state.custom.id.trim())) {
				problems['custom.id'] = 'ui.pages.providersPage.add.idInvalid';
			}
			if (state.custom.label.trim() === '') problems['custom.label'] = needs('custom.label');
			if (state.custom.baseURL.trim() === '') problems['custom.baseURL'] = needs('custom.baseURL');
			if (state.custom.apiFormat.trim() === '')
				problems['custom.apiFormat'] = needs('custom.apiFormat');
			break;
		}
		case 'azure': {
			if (variableValue(state, 'resource').trim() === '') {
				problems['variables.resource'] = needs('resource');
			}
			if (state.credential.secret.trim() === '') problems['credential.secret'] = needs('key');
			if (state.deployments.filter((row) => row.modelId.trim() !== '').length === 0) {
				problems.deployments = 'ui.pages.providersPage.add.deploymentsRequired';
			}
			break;
		}
		case 'aws': {
			if (variableValue(state, 'region').trim() === '')
				problems['variables.region'] = needs('region');
			if (state.credential.useAccessKeys) {
				if (state.credential.accessKeyId.trim() === '') {
					problems['credential.accessKeyId'] = needs('accessKeyId');
				}
				if (state.credential.secretAccessKey.trim() === '') {
					problems['credential.secretAccessKey'] = needs('secretAccessKey');
				}
			} else if (state.credential.secret.trim() === '') {
				problems['credential.secret'] = needs('key');
			}
			break;
		}
		case 'gcp': {
			if (variableValue(state, 'project').trim() === '')
				problems['variables.project'] = needs('project');
			if (variableValue(state, 'location').trim() === '')
				problems['variables.location'] = needs('location');
			if (state.credential.useServiceAccount) {
				if (state.credential.serviceAccount.trim() === '') {
					problems['credential.serviceAccount'] = needs('serviceAccount');
				}
			} else if (state.credential.secret.trim() === '') {
				problems['credential.secret'] = needs('key');
			}
			break;
		}
		case 'signin': {
			if (state.loginFlow.trim() === '' && (template?.login_methods ?? []).length > 0) {
				problems.loginFlow = needs('flow');
			}
			break;
		}
		case 'none': {
			const missing = (template?.variables ?? []).filter(
				(variable) => variable.required && variableValue(state, variable.name).trim() === ''
			);
			for (const variable of missing) {
				problems['variables.' + variable.name] = needs(variable.name);
			}
			break;
		}
		case 'local': {
			if (state.baseURL.trim() === '') problems.baseURL = needs('baseURL');
			if (state.templateId === 'litellm' && state.credential.secret.trim() === '') {
				problems['credential.secret'] = needs('key');
			}
			break;
		}
		default: {
			if (state.credential.secret.trim() === '') problems['credential.secret'] = needs('key');
			break;
		}
	}

	const required = (template?.variables ?? []).filter((variable) => variable.required);
	for (const variable of required) {
		if (variableValue(state, variable.name).trim() === '') {
			problems['variables.' + variable.name] = needs(variable.name);
		}
	}
	// A connection that publishes no model list gets every id from the
	// operator, so the flow asks for at least one before it probes. Azure has
	// its own wording for the same requirement, and a sign-in adds its models
	// after the login.
	if (
		kind !== 'azure' &&
		kind !== 'signin' &&
		declaresNoList(state, template) &&
		state.deployments.filter((row) => row.modelId.trim() !== '').length === 0
	) {
		problems.deployments = 'ui.pages.providersPage.add.modelsRequired';
	}
	return problems;
}

/** Reads a variable an operator typed, falling back to the first option a template offers so a
 *  select never starts empty. */
export function variableValue(state: StepperState, name: string): string {
	return state.variables[name] ?? '';
}

/** Fills every variable with the first option a template offers, which is what makes a form
 *  continue-able without typing. */
export function defaultVariables(template: ProviderTemplate | null): Record<string, string> {
	const values: Record<string, string> = {};
	for (const variable of template?.variables ?? []) {
		values[variable.name] = variable.options?.[0] ?? '';
	}
	return values;
}

/** Names what a probe request tests. */
export type ProbeTarget = 'provider' | 'template' | 'custom';

/** The address and credential a verify step should probe. */
export function probeTarget(state: StepperState): ProbeTarget {
	if (state.path === 'custom') return 'custom';
	if (state.targetProviderId !== '') return 'provider';
	if (state.templateId === '') return 'custom';
	return 'template';
}

/** Renders the credential a probe carries. The daemon decides how to send it from the provider's
 *  own auth, so this only has to say what kind of thing the operator typed. */
export function probeCredential(state: StepperState, kind: ConnectKind) {
	switch (kind) {
		case 'signin':
			return { kind: 'oauth', secret: '' };
		case 'none':
			return { kind: 'none', secret: '' };
		case 'aws':
			if (state.credential.useAccessKeys) {
				return {
					kind: 'aws_keys',
					secret: JSON.stringify({
						access_key_id: state.credential.accessKeyId.trim(),
						secret_access_key: state.credential.secretAccessKey.trim(),
						session_token: state.credential.sessionToken.trim()
					})
				};
			}
			return { kind: 'api_key', secret: state.credential.secret.trim() };
		case 'gcp':
			if (state.credential.useServiceAccount) {
				return { kind: 'gcp_service_account', secret: state.credential.serviceAccount.trim() };
			}
			return { kind: 'api_key', secret: state.credential.secret.trim() };
		default:
			return { kind: 'api_key', secret: state.credential.secret.trim() };
	}
}

/** Renders the exact request the daemon is asked to test. */
export function buildProbeBody(
	state: StepperState,
	template: ProviderTemplate | null,
	kind: ConnectKind
): Record<string, unknown> {
	const body: Record<string, unknown> = {
		label: state.label.trim(),
		api_format: state.apiFormat,
		base_url: state.baseURL,
		headers: state.headers,
		variables: state.variables,
		credential: probeCredential(state, kind)
	};

	switch (probeTarget(state)) {
		case 'provider':
			body.provider_id = state.targetProviderId;
			break;
		case 'custom':
			body.custom_id = state.custom.id.trim();
			body.label = state.custom.label.trim() || state.custom.id.trim();
			body.api_format = state.custom.apiFormat;
			body.base_url = state.custom.baseURL;
			body.key_header = state.custom.keyHeader;
			body.models_format = state.custom.modelsFormat;
			break;
		default:
			body.template_id = state.templateId;
			break;
	}

	// The ids an operator typed are the roster of a connection that publishes
	// no model list, so they travel with the probe that would store them.
	if (declaresNoList(state, template) && kind !== 'signin') {
		body.manual_models = typedModels(state);
	}
	return body;
}

/** Renders the model ids an operator typed. Azure serves deployment names and Vertex AI and Kiro
 *  serve ids a list never returns, so the roster comes from the form and each id may be priced
 *  as a model Relo does know. */
export function typedModels(state: StepperState): { model_id: string; priced_as: string }[] {
	return state.deployments
		.filter((row) => row.modelId.trim() !== '')
		.map((row) => ({ model_id: row.modelId.trim(), priced_as: row.pricedAs.trim() }));
}

/** Renders the body that saves a probe. */
export function commitBody(
	state: StepperState,
	options: { providerId: string; label: string; disabled: string[] }
): Record<string, unknown> {
	return {
		provider_id: options.providerId.trim(),
		label: options.label.trim(),
		account_label: state.accountLabel.trim() || defaultAccountName(),
		disabled_models: options.disabled
	};
}

/** Reports whether the operator typed anything that closing the flow would throw away, which is
 *  what a discard confirmation reads. */
export function isDirty(state: StepperState): boolean {
	if (state.credential.secret.trim() !== '') return true;
	if (state.credential.accessKeyId.trim() !== '') return true;
	if (state.credential.secretAccessKey.trim() !== '') return true;
	if (state.credential.sessionToken.trim() !== '') return true;
	if (state.credential.serviceAccount.trim() !== '') return true;
	if (state.accountLabel.trim() !== '') return true;
	if (state.label.trim() !== '') return true;
	if (state.baseURL.trim() !== '') return true;
	if (state.deployments.some((row) => row.modelId.trim() !== '')) return true;
	if (Object.values(state.variables).some((value) => value.trim() !== '')) return true;
	if (Object.values(state.headers).some((value) => value.trim() !== '')) return true;
	return (
		state.custom.id.trim() !== '' ||
		state.custom.label.trim() !== '' ||
		state.custom.baseURL.trim() !== ''
	);
}

/** Moves the base URL and key header with a format choice when the operator has not written one
 *  of their own. */
export function followFormatDefaults(
	previous: Pick<TemplateFormatOption, 'default_base_url' | 'key_header'> | null,
	next: Pick<TemplateFormatOption, 'default_base_url' | 'key_header'>,
	current: { baseURL: string; keyHeader: string }
): { baseURL: string; keyHeader: string } {
	return {
		baseURL:
			previous !== null && current.baseURL === previous.default_base_url
				? next.default_base_url
				: current.baseURL,
		keyHeader:
			previous !== null && current.keyHeader === previous.key_header
				? next.key_header
				: current.keyHeader
	};
}
