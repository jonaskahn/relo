import { describe, expect, test } from 'vitest';

import { canDeleteExpired, groupKeys, keyStatusPill } from './access-keys';
import type { AccessKey } from './types';

function key(id: string, status: string, owner = '', expiresAtMs = 0): AccessKey {
	return {
		id,
		name: id,
		kind: 'agent',
		client: 'codex',
		token_hint: 'abcd…yz',
		status,
		generation: 1,
		created_at_ms: 0,
		last_used_at_ms: 0,
		expires_at_ms: expiresAtMs,
		owner
	};
}

const keys = [
	key('live-1', 'active'),
	key('gone', 'revoked'),
	key('lapsed', 'expired'),
	key('live-2', 'active')
];

describe('the client key groups', () => {
	test('the unfiltered list holds live keys, then expired, then revoked', () => {
		const groups = groupKeys(keys, 'all');
		expect(groups.active.map((entry) => entry.id)).toEqual(['live-1', 'live-2']);
		expect(groups.expired.map((entry) => entry.id)).toEqual(['lapsed']);
		expect(groups.revoked.map((entry) => entry.id)).toEqual(['gone']);
	});

	test('a filter keeps one group and drops the others', () => {
		expect(groupKeys(keys, 'active').expired).toEqual([]);
		expect(groupKeys(keys, 'active').revoked).toEqual([]);
		expect(groupKeys(keys, 'inactive').active).toEqual([]);
		expect(groupKeys(keys, 'active').active.map((entry) => entry.id)).toEqual(['live-1', 'live-2']);
		expect(groupKeys(keys, 'inactive').expired.map((entry) => entry.id)).toEqual(['lapsed']);
		expect(groupKeys(keys, 'inactive').revoked.map((entry) => entry.id)).toEqual(['gone']);
	});

	test('an empty list groups nothing', () => {
		const groups = groupKeys([], 'all');
		expect(groups.active).toEqual([]);
		expect(groups.expired).toEqual([]);
		expect(groups.revoked).toEqual([]);
	});

	test('only an expired operator key may be deleted', () => {
		expect(canDeleteExpired(key('lapsed', 'expired'))).toBe(true);
		expect(canDeleteExpired(key('owned', 'expired', 'codex'))).toBe(false);
		expect(canDeleteExpired(key('gone', 'revoked'))).toBe(false);
		expect(canDeleteExpired(key('live', 'active'))).toBe(false);
	});
});

describe('the status pill', () => {
	const now = 1_700_000_000_000;

	test('a live key without an expiry reads Active', () => {
		expect(keyStatusPill(key('live', 'active'), now)).toEqual({
			kind: 'pass',
			labelKey: 'ui.status.badge.active'
		});
	});

	test('a live key that lapses says when, rather than only that it is active', () => {
		const pill = keyStatusPill(key('live', 'active', '', now + 4 * 3_600_000 + 49 * 60_000), now);
		expect(pill.kind).toBe('pass');
		expect(pill.labelKey).toBe('ui.pages.keysPage.expiresIn');
		expect(pill.timeLeft).toBe('4h 49m');
	});

	test('a lapsed key is warn and a revoked one is danger', () => {
		expect(keyStatusPill(key('lapsed', 'expired'), now).kind).toBe('warn');
		expect(keyStatusPill(key('lapsed', 'expired'), now).labelKey).toBe('ui.status.badge.expired');
		expect(keyStatusPill(key('gone', 'revoked'), now).kind).toBe('fail');
		expect(keyStatusPill(key('gone', 'revoked'), now).labelKey).toBe('ui.status.badge.revoked');
	});

	test('a key whose expiry passed while the daemon still calls it active reads its own status', () => {
		expect(keyStatusPill(key('stale', 'active', '', now - 1000), now).labelKey).toBe(
			'ui.status.badge.active'
		);
	});
});
