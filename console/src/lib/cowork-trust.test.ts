import { describe, expect, it } from 'vitest';

import {
	buildAgentPrompt,
	COWORK_REASONS,
	coworkManualMode,
	coworkReasonKey,
	osName
} from './cowork-trust';
import type { DesktopPickerManual } from './types';

const manual: DesktopPickerManual = {
	os: 'darwin',
	cert_path: '/Users/x/.relo/ca/ca.pem',
	sha1: 'AA',
	sha256: 'BB'.repeat(32),
	docs_url: 'https://example.test/guide',
	install: 'sudo security add-trusted-cert -d /Users/x/.relo/ca/ca.pem',
	remove: 'sudo security remove-trusted-cert -d /Users/x/.relo/ca/ca.pem',
	check: 'security dump-trust-settings -d | grep "Relo Local CA"'
};

describe('Cowork reasons', () => {
	it('names a message for every reason the daemon sends and reads anything else as off', () => {
		for (const reason of COWORK_REASONS) {
			expect(coworkReasonKey(reason)).not.toMatch(/reasonOff$/);
		}
		expect(coworkReasonKey('off')).toMatch(/reasonOff$/);
		expect(coworkReasonKey('something_new')).toMatch(/reasonOff$/);
	});
});

describe('the do-it-yourself steps', () => {
	it('apply to a failed install, and to a failed removal once one was tried', () => {
		for (const reason of [
			'cowork_trust_declined',
			'cowork_trust_unavailable',
			'cowork_trust_missing'
		]) {
			expect(coworkManualMode(reason, false)).toBe('install');
		}
		expect(coworkManualMode('cowork_trust_remains', true)).toBe('remove');
	});

	it('stay out of the way while nothing has failed', () => {
		for (const reason of [
			'off',
			'cowork_active',
			'cowork_restart_required',
			'cowork_trust_remains'
		]) {
			expect(coworkManualMode(reason, false)).toBeNull();
		}
	});
});

describe('the prompt for the operator’s own coding agent', () => {
	const format = (key: string, values: Record<string, string>) =>
		`${key}|${Object.entries(values)
			.map(([name, value]) => `${name}=${value}`)
			.join('|')}`;

	it('carries the file, the fingerprint and the daemon’s own install commands', () => {
		const prompt = buildAgentPrompt(format, manual, 'install');
		expect(prompt).toContain('agentPromptInstall');
		expect(prompt).toContain('path=/Users/x/.relo/ca/ca.pem');
		expect(prompt).toContain('sha256=' + manual.sha256);
		expect(prompt).toContain('os=macOS');
		expect(prompt).toContain('commands=' + manual.install);
		expect(prompt).toContain('check=' + manual.check);
		expect(prompt).not.toContain(manual.remove);
	});

	it('carries the removal commands for a removal and nothing it was not given', () => {
		const prompt = buildAgentPrompt(format, manual, 'remove');
		expect(prompt).toContain('agentPromptRemove');
		expect(prompt).toContain('commands=' + manual.remove);
		expect(prompt).not.toContain(manual.install);
	});

	it('names the operating systems it knows', () => {
		expect([osName('darwin'), osName('windows'), osName('linux'), osName('plan9')]).toEqual([
			'macOS',
			'Windows',
			'Linux',
			'plan9'
		]);
	});
});
