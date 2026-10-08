/** A newer release the daemon reports. */
export type UpdateNotice = {
	current: string;
	latest: string;
	available: boolean;
	url: string;
	method?: 'sparkle' | 'download';
};

/** Reports whether a notice is one to act on. */
export function updateAvailable(notice: UpdateNotice | null | undefined): boolean {
	return Boolean(notice?.available && notice.url);
}
