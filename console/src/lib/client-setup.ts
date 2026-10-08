// The client keys page mints a key once and then has to walk the operator
// through pointing their coding client at the daemon. These helpers hold
// the client catalogue and the setup snippets the reveal step renders.

/** One client the key setup sheet can walk an operator through. */
export interface ClientOption {
	id: string;
	label: string;
	kind: 'agent' | 'shared';
	// protocol is the data-plane surface the client speaks, which decides
	// which base URL the setup snippets show.
	protocol: 'openai' | 'anthropic';
	// logoId names a provider logo when one exists; otherwise the card
	// draws a monogram.
	logoId?: string;
}

/** What the agents page and the key setup sheet offer. A logoId names the mark to draw, and it is
 *  the agent's own mark where one exists: a Codex row wears the Codex mark rather than OpenAI's,
 *  because the operator is choosing an agent and not a vendor. A client no icon set publishes
 *  carries no logoId and falls back to a monogram. */
export const CLIENT_OPTIONS: ClientOption[] = [
	{ id: 'codex', label: 'Codex', kind: 'agent', protocol: 'openai', logoId: 'codex' },
	{
		id: 'claude-code',
		label: 'Claude Code',
		kind: 'agent',
		protocol: 'anthropic',
		logoId: 'claude-code'
	},
	// Claude Desktop has no mark of its own and wears Anthropic's.
	{
		id: 'claude-desktop',
		label: 'Claude Desktop',
		kind: 'agent',
		protocol: 'anthropic',
		logoId: 'anthropic'
	},
	{ id: 'cursor', label: 'Cursor', kind: 'agent', protocol: 'openai', logoId: 'cursor' },
	{
		id: 'grok-build',
		label: 'Grok Build',
		kind: 'agent',
		protocol: 'openai',
		logoId: 'grok-build'
	},
	// The clients Relo writes a provider block for. Their labels are the title
	// the console shows, so an operator reads the same name the daemon reports.
	{ id: 'opencode', label: 'Opencode', kind: 'agent', protocol: 'openai', logoId: 'opencode' },
	{ id: 'cline', label: 'Cline', kind: 'agent', protocol: 'openai', logoId: 'cline' },
	{ id: 'pi', label: 'Pi', kind: 'agent', protocol: 'openai', logoId: 'pi' },
	{ id: 'omp', label: 'OMP', kind: 'agent', protocol: 'openai', logoId: 'omp' },
	{ id: 'hermes', label: 'Hermes', kind: 'agent', protocol: 'openai', logoId: 'hermes' },
	{ id: 'openclaw', label: 'Openclaw', kind: 'agent', protocol: 'openai', logoId: 'openclaw' },
	// Kimi Code has no mark under its own id, so it wears the published Kimi one.
	{ id: 'kimi', label: 'Kimi', kind: 'agent', protocol: 'openai', logoId: 'kimi-code-plan-cn' },
	{ id: 'gajae', label: 'Gajae', kind: 'agent', protocol: 'openai' },
	{ id: 'dsh', label: 'DSH', kind: 'agent', protocol: 'openai', logoId: 'dsh' },
	{ id: 'mcode', label: 'Mcode', kind: 'agent', protocol: 'openai', logoId: 'mcode' },
	{ id: 'zcode', label: 'Zcode', kind: 'agent', protocol: 'openai' },
	{ id: 'prime', label: 'Prime', kind: 'agent', protocol: 'openai' },
	{ id: 'aside', label: 'Aside', kind: 'agent', protocol: 'openai' },
	{ id: 'raycast', label: 'Raycast', kind: 'agent', protocol: 'openai' },
	{ id: 'omo', label: 'OMO', kind: 'agent', protocol: 'openai' },
	// The last two are not clients at all: "Other" stands in for a client the
	// operator names by hand, and "Shared" is a key reusable across clients. Both
	// take a mark so the row reads as a choice rather than a gap in the list.
	{ id: 'other', label: 'Other', kind: 'agent', protocol: 'openai', logoId: 'other' },
	{ id: 'shared', label: 'Shared', kind: 'shared', protocol: 'openai', logoId: 'shared' }
];

/** The address a client points at, per protocol. */
export interface DataPlaneURLs {
	openai: string;
	anthropic: string;
}

/** The paste-ready setup for one client. */
export interface SetupSnippet {
	language: 'toml' | 'shell';
	code: string;
}

/** Renders the configuration one client needs. Codex reads a config.toml block and
 *  RELO_CODEX_API_KEY.
 *  The Anthropic surface is only a base URL: an auth token overrides a claude.ai login, and the
 *  Claude Code integration is what detects login versus proxy.
 *  Any other client gets the generic base URL and key pair. */
export function setupSnippet(clientId: string, urls: DataPlaneURLs, token: string): SetupSnippet {
	if (clientId === 'codex') {
		return {
			language: 'toml',
			code:
				'model_provider = "relo"\n' +
				'\n[model_providers.relo]\n' +
				'name = "Relo"\n' +
				'base_url = "' +
				urls.openai +
				'/v1"\n' +
				'env_key = "RELO_CODEX_API_KEY"\n' +
				'\n# shell\n' +
				'export RELO_CODEX_API_KEY="' +
				token +
				'"'
		};
	}
	const option = CLIENT_OPTIONS.find((entry) => entry.id === clientId);
	const protocol = option?.protocol ?? 'openai';
	if (protocol === 'anthropic') {
		return {
			language: 'shell',
			code: 'export ANTHROPIC_BASE_URL=' + urls.anthropic
		};
	}
	return {
		language: 'shell',
		code: 'export OPENAI_BASE_URL=' + urls.openai + '/v1\n' + 'export OPENAI_API_KEY=' + token
	};
}
