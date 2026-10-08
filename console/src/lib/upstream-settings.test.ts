import { describe, expect, it } from 'vitest';

import {
	CALL_WAIT_PRESETS,
	DEFAULT_CALL_WAIT,
	DEFAULT_FAILOVER_COOLDOWN,
	DEFAULT_RETRY_BACKOFF,
	FAILOVER_COOLDOWN_PRESETS,
	callWaitLabel,
	cloneWindows,
	failoverCooldownLabel,
	normalizeRetryBackoff,
	retryBackoffLabel,
	retryBackoffValid
} from './upstream-settings';

describe('call wait labels', () => {
	it('renders seconds up to a minute and whole minutes above', () => {
		expect(callWaitLabel(30)).toBe('30s');
		expect(callWaitLabel(90)).toBe('90s');
		expect(callWaitLabel(60)).toBe('1m');
		expect(callWaitLabel(180)).toBe('3m');
		expect(callWaitLabel(300)).toBe('5m');
		expect(callWaitLabel(600)).toBe('10m');
		expect(callWaitLabel(1800)).toBe('30m');
	});

	it('offers the five minute presets and ships the five-minute default', () => {
		expect(CALL_WAIT_PRESETS).toEqual([180, 300, 600, 900, 1800]);
		expect(CALL_WAIT_PRESETS).toContain(DEFAULT_CALL_WAIT);
		expect(DEFAULT_CALL_WAIT).toBe(300);
	});
});

describe('failover cooldown labels', () => {
	it('renders minutes up to an hour and whole hours above', () => {
		expect(failoverCooldownLabel(180)).toBe('3m');
		expect(failoverCooldownLabel(300)).toBe('5m');
		expect(failoverCooldownLabel(900)).toBe('15m');
		expect(failoverCooldownLabel(1800)).toBe('30m');
		expect(failoverCooldownLabel(3600)).toBe('1h');
		expect(failoverCooldownLabel(18000)).toBe('5h');
	});

	it('offers the none start plus the six waits and ships no first wait by default', () => {
		expect(FAILOVER_COOLDOWN_PRESETS).toEqual([0, 180, 300, 900, 1800, 3600, 18000]);
		expect(FAILOVER_COOLDOWN_PRESETS).toContain(DEFAULT_FAILOVER_COOLDOWN);
		expect(DEFAULT_FAILOVER_COOLDOWN).toBe(0);
	});
});

describe('retry backoff windows', () => {
	it('labels three windows in order', () => {
		expect(retryBackoffLabel(DEFAULT_RETRY_BACKOFF)).toBe('1–3s, 3–5s, 5–10s');
	});

	it('accepts three ordered ranges and rejects anything else', () => {
		expect(retryBackoffValid(DEFAULT_RETRY_BACKOFF)).toBe(true);
		expect(
			retryBackoffValid([
				[1, 3],
				[3, 5]
			])
		).toBe(false);
		expect(
			retryBackoffValid([
				[1, 3],
				[5, 3],
				[5, 10]
			])
		).toBe(false);
		expect(
			retryBackoffValid([
				[0, 0],
				[3, 5],
				[5, 10]
			])
		).toBe(false);
		expect(
			retryBackoffValid([
				[1, 3],
				[3, 5],
				[5, 601]
			])
		).toBe(false);
	});

	it('falls back to the shipped windows for a value the daemon cannot answer', () => {
		expect(normalizeRetryBackoff(undefined)).toEqual(DEFAULT_RETRY_BACKOFF);
		expect(normalizeRetryBackoff([])).toEqual(DEFAULT_RETRY_BACKOFF);
		expect(
			normalizeRetryBackoff([
				[1, 2],
				[3, 4],
				[5, 6]
			])
		).toEqual([
			[1, 2],
			[3, 4],
			[5, 6]
		]);
	});

	it('falls back when one window is malformed', () => {
		expect(normalizeRetryBackoff([[1, 2], [3, 4], [5]])).toEqual(DEFAULT_RETRY_BACKOFF);
		expect(
			normalizeRetryBackoff([
				[1, 2],
				[3, 4],
				['a', 6]
			])
		).toEqual(DEFAULT_RETRY_BACKOFF);
		expect(
			normalizeRetryBackoff([
				[1, 2],
				[3, 4],
				[-1, 6]
			])
		).toEqual(DEFAULT_RETRY_BACKOFF);
		expect(
			normalizeRetryBackoff([
				[1, 2],
				[3, 4],
				[5, 5]
			])
		).toEqual(DEFAULT_RETRY_BACKOFF);
		expect(
			normalizeRetryBackoff([
				[1, 2],
				[3, 4],
				[5, 601]
			])
		).toEqual(DEFAULT_RETRY_BACKOFF);
	});

	it('copies windows so a draft cannot change the source', () => {
		const copied = cloneWindows(DEFAULT_RETRY_BACKOFF);
		copied[0][0] = 99;
		expect(DEFAULT_RETRY_BACKOFF[0][0]).toBe(1);
	});
});
