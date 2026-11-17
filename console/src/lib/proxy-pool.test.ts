import { describe, expect, it } from 'vitest';

import { parseProxyLine, parseProxyPaste, proxyStateLabel, redactProxy } from './proxy-pool';

describe('proxy paste parsing', () => {
	it('accepts a full URL, the quartet, and SOCKS5', () => {
		expect(parseProxyLine('http://user:pass@127.0.0.1:7890')).toMatchObject({
			scheme: 'http',
			host: '127.0.0.1',
			port: 7890,
			username: 'user',
			password: 'pass'
		});
		expect(parseProxyLine('127.0.0.1:7890:user:pass')).toMatchObject({
			scheme: 'http',
			username: 'user',
			password: 'pass'
		});
		expect(parseProxyLine('socks5h://127.0.0.1:1080')).toMatchObject({ scheme: 'socks5' });
	});

	it('rejects schemes and addresses the daemon refuses', () => {
		expect(parseProxyLine('ftp://127.0.0.1:21')).toBeNull();
		expect(parseProxyLine('not a proxy')).toBeNull();
		expect(parseProxyLine('http://127.0.0.1')).toBeNull();
	});

	it('ignores comments and blanks and reports duplicates', () => {
		const lines = parseProxyPaste(
			'# comment\n\nhttp://127.0.0.1:7890\nhttp://127.0.0.1:7890\nftp://bad\n',
			['http://127.0.0.1:7891']
		);
		expect(lines.map((line) => line.status)).toEqual(['ready', 'duplicate', 'invalid']);
	});
});

describe('proxy rows', () => {
	it('redacts a stored password and keeps a credential-free address plain', () => {
		expect(
			redactProxy({
				scheme: 'http',
				host: '127.0.0.1',
				port: 7890,
				username: 'user',
				hasPassword: true
			})
		).toBe('http://user:••••@127.0.0.1:7890');
		expect(redactProxy({ scheme: 'socks5', host: '127.0.0.1', port: 1080, username: '' })).toBe(
			'socks5://127.0.0.1:1080'
		);
	});

	it('names every health state, including a cooling deadline', () => {
		expect(proxyStateLabel({ state: 'ready' })).toBe('ready');
		expect(proxyStateLabel({ state: 'unreachable' })).toBe('unreachable');
		expect(proxyStateLabel({ state: 'unverified' })).toBe('unverified');
		expect(proxyStateLabel({ state: 'cooling', cooling_until: '2026-11-17T12:04:00Z' })).toBe(
			'cooling_until:2026-11-17T12:04:00Z'
		);
	});
});
