import { describe, expect, it } from 'vitest';

import { daemonActionPath, daemonRestartPath, daemonShutdownPath } from './daemon-lifecycle';

describe('daemonRestartPath', () => {
	it('names the graceful restart', () => {
		expect(daemonRestartPath('restart')).toBe('/daemon/restart');
	});

	it('names the force restart', () => {
		expect(daemonRestartPath('force-restart')).toBe('/daemon/force-restart');
	});
});

describe('daemonShutdownPath', () => {
	it('names the shutdown', () => {
		expect(daemonShutdownPath()).toBe('/daemon/shutdown');
	});
});

describe('daemonActionPath', () => {
	it('names each daemon action', () => {
		expect(daemonActionPath('restart')).toBe('/daemon/restart');
		expect(daemonActionPath('force-restart')).toBe('/daemon/force-restart');
		expect(daemonActionPath('shutdown')).toBe('/daemon/shutdown');
	});
});
