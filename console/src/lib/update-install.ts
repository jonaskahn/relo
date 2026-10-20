import type { UpdateInstaller } from './updates';

/** One install method the update dialog can teach. */
export type InstallMethod = 'shell' | 'powershell' | 'npm' | 'brew';

/** One upgrade command for an install method. */
export type InstallGuide = {
	readonly method: InstallMethod;
	readonly labelKey: string;
	readonly command: string;
};

const INSTALL_SCRIPT = 'https://relo.ifelse.one/install.sh';
const INSTALL_POWERSHELL = 'https://relo.ifelse.one/install.ps1';

/** Reports whether a version string carries a prerelease suffix. */
export function isPrerelease(version: string): boolean {
	return bareVersion(version).split('+')[0].includes('-');
}

/** Returns the commands that upgrade Relo for the detected installer, or all
 *  four when the daemon could not name the install method. */
export function installGuides(
	installer: UpdateInstaller | undefined,
	latest: string
): InstallGuide[] {
	const version = bareVersion(latest);
	const beta = isPrerelease(version);
	const guides = [
		shellGuide(version, beta),
		powershellGuide(version, beta),
		npmGuide(version, beta),
		brewGuide(beta)
	];
	switch (installer) {
		case 'shell':
			return [guides[0]];
		case 'powershell':
			return [guides[1]];
		case 'npm':
			return [guides[2]];
		case 'brew':
			return [guides[3]];
		case 'sparkle':
		case 'installer':
			return [];
		default:
			return guides;
	}
}

function bareVersion(version: string): string {
	return version.trim().replace(/^v/, '');
}

function shellGuide(version: string, beta: boolean): InstallGuide {
	return {
		method: 'shell',
		labelKey: 'ui.updateDialog.methodShell',
		command: beta
			? `curl -fsSL ${INSTALL_SCRIPT} | sh -s -- --version v${version}`
			: `curl -fsSL ${INSTALL_SCRIPT} | sh`
	};
}

function powershellGuide(version: string, beta: boolean): InstallGuide {
	return {
		method: 'powershell',
		labelKey: 'ui.updateDialog.methodPowerShell',
		command: beta
			? `& ([scriptblock]::Create((irm ${INSTALL_POWERSHELL}))) -Version v${version}`
			: `irm ${INSTALL_POWERSHELL} | iex`
	};
}

function npmGuide(version: string, beta: boolean): InstallGuide {
	return {
		method: 'npm',
		labelKey: 'ui.updateDialog.methodNpm',
		command: beta
			? `npm i -g @jonaskahn/relo@${version} --foreground-scripts`
			: 'npm i -g @jonaskahn/relo --foreground-scripts'
	};
}

function brewGuide(beta: boolean): InstallGuide {
	return {
		method: 'brew',
		labelKey: 'ui.updateDialog.methodBrew',
		command: beta ? 'brew upgrade relo-beta' : 'brew upgrade relo'
	};
}
