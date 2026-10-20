/** One channel that installed the running build. */
export type UpdateInstaller =
	'sparkle' | 'brew' | 'npm' | 'shell' | 'powershell' | 'installer' | 'unknown';

/** A newer release the daemon reports. */
export type UpdateNotice = {
	current: string;
	latest: string;
	available: boolean;
	url: string;
	method?: 'sparkle' | 'download';
	installer?: UpdateInstaller;
};

/** Reports whether a notice is one to act on. */
export function updateAvailable(notice: UpdateNotice | null | undefined): boolean {
	return Boolean(notice?.available && notice.url);
}
