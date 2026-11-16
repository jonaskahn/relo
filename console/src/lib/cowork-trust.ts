// What the Route Cowork control reads from the daemon's answer: which reason copy to show,
// whether the "do it yourself" steps apply, and the prompt an operator hands to their own
// coding agent. These are pure, so the control and its tests share one reading.

import type { DesktopPickerManual } from './types';

const KEY = 'ui.pages.integrationsPage.desktopPicker.cowork.';

/** The refusals and states the daemon names for Cowork routing. */
export const COWORK_REASONS = new Set([
	'cowork_needs_picker',
	'cowork_trust_missing',
	'cowork_restart_required',
	'cowork_active',
	'cowork_trust_declined',
	'cowork_trust_unavailable',
	'cowork_trust_remains'
]);

/** The message key for a Cowork reason. Anything else reads as "off". */
export function coworkReasonKey(reason: string): string {
	switch (reason) {
		case 'cowork_active':
			return KEY + 'reasonActive';
		case 'cowork_restart_required':
			return KEY + 'reasonRestart';
		case 'cowork_needs_picker':
			return KEY + 'reasonNeedsPicker';
		case 'cowork_trust_missing':
			return KEY + 'reasonTrustMissing';
		case 'cowork_trust_declined':
			return KEY + 'reasonTrustDeclined';
		case 'cowork_trust_unavailable':
			return KEY + 'reasonTrustUnavailable';
		case 'cowork_trust_remains':
			return KEY + 'reasonTrustRemains';
		default:
			return KEY + 'reasonOff';
	}
}

/** Which "do it yourself" steps apply: install when the system trust could not be added, remove
 *  after a removal that failed, and none otherwise. */
export type ManualMode = 'install' | 'remove' | null;

const INSTALL_REASONS = new Set([
	'cowork_trust_declined',
	'cowork_trust_unavailable',
	'cowork_trust_missing'
]);

export function coworkManualMode(reason: string, removeFailed: boolean): ManualMode {
	if (removeFailed) return 'remove';
	return INSTALL_REASONS.has(reason) ? 'install' : null;
}

/** The operating system as the prompt names it. */
export function osName(os: string): string {
	switch (os) {
		case 'darwin':
			return 'macOS';
		case 'windows':
			return 'Windows';
		case 'linux':
			return 'Linux';
		default:
			return os;
	}
}

/** Formats one message with values, which is what the console's translator does. */
export type Format = (key: string, values: Record<string, string>) => string;

/** The prompt an operator pastes into their own coding agent to install or remove the
 *  certificate. Its prose is the console's language; every value in it, the path, the
 *  fingerprint and the commands, comes from the daemon and is not rewritten here. */
export function buildAgentPrompt(
	format: Format,
	manual: DesktopPickerManual,
	mode: 'install' | 'remove'
): string {
	return format(KEY + (mode === 'install' ? 'agentPromptInstall' : 'agentPromptRemove'), {
		path: manual.cert_path,
		sha256: manual.sha256,
		os: osName(manual.os),
		commands: mode === 'install' ? manual.install : manual.remove,
		check: manual.check
	});
}
