/** The two ways a daemon can be restarted. */
export type DaemonRestartKind = 'restart' | 'force-restart';
/** Every lifecycle call the console can make. */
export type DaemonActionKind = DaemonRestartKind | 'shutdown';

/** The management route one restart answers on. */
export function daemonRestartPath(kind: DaemonRestartKind): string {
	return kind === 'force-restart' ? '/daemon/force-restart' : '/daemon/restart';
}

/** The management route a shutdown answers on. */
export function daemonShutdownPath(): string {
	return '/daemon/shutdown';
}

/** The management route one lifecycle call answers on. */
export function daemonActionPath(kind: DaemonActionKind): string {
	return kind === 'shutdown' ? daemonShutdownPath() : daemonRestartPath(kind);
}
