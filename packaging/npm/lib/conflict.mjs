// Finds another Relo command on PATH. The binary is one product now: a daemon
// with its tray icon, built the same way for the npm package and the
// installers. Two installs would still fight over the name, the home
// directory, and the ports, so the install refuses when a command this
// package did not install answers PATH.
import { accessSync, constants, existsSync, realpathSync } from 'node:fs';
import { delimiter, join, resolve, sep } from 'node:path';
import { homedir } from 'node:os';
import { execFileSync } from 'node:child_process';

// candidates lists the relo commands a PATH resolves, in order. Windows
// installs the app's command as relo.cmd, a shim that starts the GUI
// relo.exe, and npm's bin as its own shim; both spellings are candidates
// there, while POSIX installs the bare command.
export function candidates({ pathValue = '', nodePlatform = 'linux', name = 'relo' } = {}) {
	const executables = nodePlatform === 'win32' ? [`${name}.exe`, `${name}.cmd`] : [name];
	const found = [];
	for (const dir of pathValue.split(delimiter).filter(Boolean)) {
		for (const file of executables) {
			const candidate = join(dir, file);
			try {
				accessSync(candidate, constants.X_OK);
				found.push(candidate);
			} catch {
				// Not there, or not executable: keep looking.
			}
		}
	}
	return found;
}

// findConflictingRelo returns the path of the first relo command on PATH
// this package did not install, or null. Commands inside selfDir are this
// package's own install and are skipped, so a reinstall is not a conflict.
// selfDir, PATH, and the platform are parameters so the search can be tested
// against fixture directories.
export function findConflictingRelo({
	env = process.env,
	nodePlatform = process.platform,
	selfDir = null
} = {}) {
	const pathValue = env.PATH ?? env.Path ?? '';
	for (const candidate of candidates({ pathValue, nodePlatform })) {
		if (selfDir && insideDir(candidate, selfDir)) {
			continue;
		}
		return candidate;
	}
	return null;
}

// appBundles lists where the macOS Relo.app may live. The bundle installs
// no command on PATH, so the PATH search below never sees it, and a stacked
// install would fight it over the home directory and the ports.
export function appBundles({ home = homedir(), applications = '/Applications' } = {}) {
	return [join(applications, 'Relo.app'), join(home, 'Applications', 'Relo.app')];
}

// findReloApp returns the first Relo.app bundle on this Mac, or null. The
// locations and the existence check are parameters so the search can be
// tested against fixture directories.
export function findReloApp({
	nodePlatform = process.platform,
	home,
	applications,
	exists = existsSync
} = {}) {
	if (nodePlatform !== 'darwin') return null;
	for (const app of appBundles({ home, applications })) {
		if (exists(app)) return app;
	}
	return null;
}

const UNINSTALL_KEYS = [
	'HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Relo',
	'HKLM\\Software\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Relo',
	'HKLM\\Software\\WOW6432Node\\Microsoft\\Windows\\CurrentVersion\\Uninstall\\Relo'
];

// findSetupInstall reports whether Relo-Setup registered an install. The
// installer normally adds its shim directory to PATH, which the search above
// already sees; the registry covers the install that skipped the PATH edit.
// A registry that cannot be read reads as no install.
export function findSetupInstall({ nodePlatform = process.platform, query = queryUninstallKey } = {}) {
	if (nodePlatform !== 'win32') return false;
	return UNINSTALL_KEYS.some((key) => query(key));
}

function queryUninstallKey(key) {
	try {
		execFileSync('reg', ['query', key], { stdio: 'ignore' });
		return true;
	} catch {
		return false;
	}
}

// insideDir reports whether candidate resolves inside dir, following a link
// the way the npm bin shim points into the installed package. Both sides
// resolve, because either path may itself sit under a link. A path that
// resolves nowhere compares by its raw form instead.
function insideDir(candidate, dir) {
	const prefix = existingPath(dir) + sep;
	const resolved = existingPath(candidate);
	return resolved === prefix.slice(0, -1) || resolved.startsWith(prefix);
}

// existingPath resolves a path through any links, or returns its raw form
// when nothing is there to resolve.
function existingPath(path) {
	try {
		return realpathSync(path);
	} catch {
		return resolve(path);
	}
}
