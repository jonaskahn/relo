import { describe, expect, it } from 'vitest';

import {
	CALL_WAIT_PRESETS,
	DEFAULT_CALL_WAIT,
	DEFAULT_FAILOVER_COOLDOWN,
	DEFAULT_RETRY_ATTEMPTS,
	FAILOVER_COOLDOWN_PRESETS,
	RETRY_ATTEMPT_PRESETS,
	callWaitLabel,
	failoverCooldownLabel,
	retryAttemptsLabel
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

describe('retry attempts', () => {
	it('labels a count as its multiple', () => {
		expect(retryAttemptsLabel(1)).toBe('1×');
		expect(retryAttemptsLabel(3)).toBe('3×');
	});

	it('offers one, three, and five retries and ships three', () => {
		expect(RETRY_ATTEMPT_PRESETS).toEqual([1, 3, 5]);
		expect(RETRY_ATTEMPT_PRESETS).toContain(DEFAULT_RETRY_ATTEMPTS);
		expect(DEFAULT_RETRY_ATTEMPTS).toBe(3);
	});
});
