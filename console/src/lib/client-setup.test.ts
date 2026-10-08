import { describe, expect, it } from 'vitest';
import { CLIENT_OPTIONS, setupSnippet } from './client-setup';
import { providerLogoSrc } from './provider-logo';

const urls = { openai: 'http://127.0.0.1:10201', anthropic: 'http://127.0.0.1:10202' };

describe('CLIENT_OPTIONS', () => {
	it('keeps one entry per client and the shared kind', () => {
		const ids = CLIENT_OPTIONS.map((option) => option.id);
		expect(new Set(ids).size).toBe(ids.length);
		expect(CLIENT_OPTIONS.find((option) => option.id === 'shared')?.kind).toBe('shared');
	});

	// A row that declares a logoId must resolve it, or the key sheet asks for a
	// mark that 404s and shows an empty tile rather than the monogram. A row
	// with no logoId is a client no icon set publishes, which falls back on
	// purpose, so only the declared ones are checked here.
	it('resolves every mark a row declares', () => {
		for (const option of CLIENT_OPTIONS) {
			expect(option.label.trim(), option.id).not.toBe('');
			if (!option.logoId) continue;
			expect(providerLogoSrc(option.logoId), option.id).not.toBeNull();
		}
	});
});

describe('setupSnippet', () => {
	it('writes the Codex config block against the OpenAI port', () => {
		const snippet = setupSnippet('codex', urls, 'relo-sk-1');
		expect(snippet.language).toBe('toml');
		expect(snippet.code).toContain('base_url = "http://127.0.0.1:10201/v1"');
		expect(snippet.code).toContain('env_key = "RELO_CODEX_API_KEY"');
		expect(snippet.code).toContain('export RELO_CODEX_API_KEY="relo-sk-1"');
		expect(snippet.code).not.toContain('RELO_TOKEN');
	});

	it('points the Anthropic surface at its base URL and does not set an auth token', () => {
		for (const clientId of ['claude-code', 'claude-desktop']) {
			const snippet = setupSnippet(clientId, urls, 'relo-sk-2');
			expect(snippet.language).toBe('shell');
			expect(snippet.code).toContain('ANTHROPIC_BASE_URL=http://127.0.0.1:10202');
			expect(snippet.code).not.toContain('ANTHROPIC_AUTH_TOKEN');
			expect(snippet.code).not.toContain('apiKey');
		}
	});

	it('falls back to OpenAI-shaped exports', () => {
		const snippet = setupSnippet('cursor', urls, 'relo-sk-3');
		expect(snippet.code).toContain('OPENAI_BASE_URL=http://127.0.0.1:10201/v1');
	});

	// A client nobody added a row for still has to produce a snippet rather
	// than an empty one: the key sheet can be showing a client from an older
	// config, and a blank block would tell the operator nothing.
	it('falls back to the OpenAI exports for a client it does not know', () => {
		const snippet = setupSnippet('a-client-from-the-future', urls, 'relo-sk-4');
		expect(snippet.language).toBe('shell');
		expect(snippet.code).toContain('OPENAI_BASE_URL=http://127.0.0.1:10201/v1');
	});

	it('gives every row a snippet that names the daemon', () => {
		for (const option of CLIENT_OPTIONS) {
			const snippet = setupSnippet(option.id, urls, 'relo-sk-5');
			expect(snippet.language, option.id).toBeTruthy();
			expect(snippet.code, option.id).toContain('127.0.0.1');
		}
	});
});
