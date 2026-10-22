import { isPrerelease } from './update-install';

/** One channel that installed the running build. */
export type UpdateInstaller =
	'sparkle' | 'brew' | 'npm' | 'shell' | 'powershell' | 'installer' | 'unknown';

/** One file the newer release carries for this machine. */
export type UpdateDownload = {
	readonly name: string;
	readonly url: string;
	readonly size?: number;
	readonly arch?: string;
	readonly format?: string;
};

/** A newer release the daemon reports. */
export type UpdateNotice = {
	current: string;
	latest: string;
	available: boolean;
	url: string;
	method?: 'sparkle' | 'download';
	installer?: UpdateInstaller;
	notes?: string;
	page?: string;
	downloads?: readonly UpdateDownload[];
};

/** Reports whether a notice is one to act on. */
export function updateAvailable(notice: UpdateNotice | null | undefined): boolean {
	return Boolean(notice?.available && notice.url);
}

/** The newest published beta a beta build names beside its own version. A newer
 *  beta is already offered by the update control, so the label stays out of
 *  its way. */
export function latestBetaVersion(
	current: string,
	notice: UpdateNotice | null | undefined
): string {
	if (!notice || updateAvailable(notice) || !isPrerelease(current)) return '';
	return notice.latest;
}
