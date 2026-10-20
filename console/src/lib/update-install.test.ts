import { describe, expect, it } from 'vitest';

import { installGuides, isPrerelease } from './update-install';

describe('installGuides', () => {
	it('uses the shell one-liner for a stable release', () => {
		expect(installGuides('shell', '0.2.0')).toEqual([
			{
				method: 'shell',
				labelKey: 'ui.updateDialog.methodShell',
				command: 'curl -fsSL https://relo.ifelse.one/install.sh | sh'
			}
		]);
	});

	it('pins the beta tag for the shell installer', () => {
		const [guide] = installGuides('shell', '0.1.4-beta.28.1a2b3c4d');
		expect(guide.command).toBe(
			'curl -fsSL https://relo.ifelse.one/install.sh | sh -s -- --version v0.1.4-beta.28.1a2b3c4d'
		);
	});

	it('uses the PowerShell one-liner for a stable release', () => {
		const [guide] = installGuides('powershell', '0.2.0');
		expect(guide.command).toBe('irm https://relo.ifelse.one/install.ps1 | iex');
	});

	it('pins the beta tag for PowerShell', () => {
		const [guide] = installGuides('powershell', '0.1.4-beta.28.1a2b3c4d');
		expect(guide.command).toBe(
			'& ([scriptblock]::Create((irm https://relo.ifelse.one/install.ps1))) -Version v0.1.4-beta.28.1a2b3c4d'
		);
	});

	it('uses the npm package for a stable release', () => {
		const [guide] = installGuides('npm', '0.2.0');
		expect(guide.command).toBe('npm i -g @jonaskahn/relo --foreground-scripts');
	});

	it('pins the npm version for a beta', () => {
		const [guide] = installGuides('npm', '0.1.4-beta.28.1a2b3c4d');
		expect(guide.command).toBe(
			'npm i -g @jonaskahn/relo@0.1.4-beta.28.1a2b3c4d --foreground-scripts'
		);
	});

	it('upgrades the stable formula', () => {
		const [guide] = installGuides('brew', '0.2.0');
		expect(guide.command).toBe('brew upgrade relo');
	});

	it('upgrades the beta formula for a beta release', () => {
		const [guide] = installGuides('brew', '0.1.4-beta.28.1a2b3c4d');
		expect(guide.command).toBe('brew upgrade relo-beta');
	});

	it('has no command for the app or an installer package', () => {
		expect(installGuides('sparkle', '0.2.0')).toEqual([]);
		expect(installGuides('installer', '0.2.0')).toEqual([]);
	});

	it('shows every method when the installer is unknown', () => {
		const guides = installGuides('unknown', '0.1.4-beta.28.1a2b3c4d');
		expect(guides.map((guide) => guide.method)).toEqual(['shell', 'powershell', 'npm', 'brew']);
		expect(guides[3].command).toBe('brew upgrade relo-beta');
	});

	it('shows every method when the daemon names no installer', () => {
		expect(installGuides(undefined, '0.2.0')).toHaveLength(4);
	});

	it('does not double a leading v', () => {
		const [guide] = installGuides('shell', 'v0.1.4-beta.28.1a2b3c4d');
		expect(guide.command).toContain('--version v0.1.4-beta.28.1a2b3c4d');
		expect(guide.command).not.toContain('vv');
	});
});

describe('isPrerelease', () => {
	it.each([
		['0.1.4-beta.1', true],
		['v0.1.4-beta.1', true],
		['0.1.4', false],
		['v0.1.4', false],
		['0.1.4+build', false],
		['0.1.4+build-x', false]
	])('%s -> %s', (version, expected) => {
		expect(isPrerelease(version)).toBe(expected);
	});
});
