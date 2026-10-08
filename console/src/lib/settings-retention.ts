/** The request ledger's floor, mirroring the daemon's own rule: a budget of 0 keeps everything,
 *  and any other budget has to leave at least this many UTC days.
 *  The console refuses a shorter window before the write. */
export const MIN_USAGE_DAYS = 3;

/** Reports whether a retention window is one the daemon accepts. */
export function validUsageDays(days: number): boolean {
	return Number.isInteger(days) && (days === 0 || days >= MIN_USAGE_DAYS);
}
