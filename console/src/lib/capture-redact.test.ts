import { describe, expect, it } from 'vitest';

import { maskCredential, redactHeaders, sensitiveHeader } from './capture-redact';

describe('maskCredential', () => {
	it('keeps the scheme a credential was sent with', () => {
		expect(maskCredential('Bearer sk-test-123')).toBe('Bearer ••••');
		expect(maskCredential('Basic dXNlcjpwYXNz')).toBe('Basic ••••');
	});

	it('masks a bare value whole', () => {
		expect(maskCredential('rlo_ak_ace6d4259537986f46c61f9d9788a3ff_4FiMdVHp')).toBe('••••');
		expect(maskCredential('__cf_bm=a3YZenUYyyNCEInk')).toBe('••••');
	});

	it('leaves an empty value alone', () => {
		expect(maskCredential('')).toBe('');
	});
});

describe('redactHeaders', () => {
	it('masks the headers that name a credential', () => {
		const redacted = redactHeaders({
			Authorization: ['Bearer eyJ0eXAiOiJhdCtqd3Q'],
			'X-Api-Key': ['rlo_ak_ace6d4259537986f46c61f9d9788a3ff_secret'],
			'X-Xai-Token-Auth': ['xai-grok-cli'],
			Cookie: ['session=abc'],
			'Set-Cookie': ['__cf_bm=a3YZenU']
		});
		expect(redacted.Authorization).toEqual(['Bearer ••••']);
		expect(redacted['X-Api-Key']).toEqual(['••••']);
		expect(redacted['X-Xai-Token-Auth']).toEqual(['••••']);
		expect(redacted.Cookie).toEqual(['••••']);
		expect(redacted['Set-Cookie']).toEqual(['••••']);
	});

	it('leaves a header that names no credential untouched', () => {
		const redacted = redactHeaders({
			'Content-Type': ['application/json'],
			'X-Ratelimit-Remaining-Tokens': ['21'],
			'User-Agent': ['relo-grok/1.0.13']
		});
		expect(redacted).toEqual({
			'Content-Type': ['application/json'],
			'X-Ratelimit-Remaining-Tokens': ['21'],
			'User-Agent': ['relo-grok/1.0.13']
		});
	});

	it('copies one value per header, so the capture never aliases the log', () => {
		const redacted = redactHeaders({ 'Content-Type': ['application/json'] });
		expect(redacted['Content-Type']).not.toBe(undefined);
	});
});

describe('sensitiveHeader', () => {
	it('reads a credential name however it is spelled', () => {
		for (const name of [
			'Authorization',
			'proxy-authorization',
			'X-Api-Key',
			'x_goog_api_key',
			'X-Xai-Token-Auth',
			'X-Amz-Security-Token',
			'Cookie',
			'Set-Cookie',
			'x-vendor-secret'
		]) {
			expect(sensitiveHeader(name), name).toBe(true);
		}
	});

	it('leaves a name that only mentions tokens alone', () => {
		for (const name of [
			'Content-Type',
			'X-Ratelimit-Remaining-Tokens',
			'X-Ratelimit-Limit-Tokens',
			'User-Agent'
		]) {
			expect(sensitiveHeader(name), name).toBe(false);
		}
	});
});
