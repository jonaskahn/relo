// Finds another Relo command on PATH. The binary is one product now: a daemon
// with its tray icon, built the same way for the npm package and the
// installers. Two installs would still fight over the name, the home
// directory, and the ports, so the install refuses when a command this
// package did not install answers PATH.
import { accessSync, constants, realpathSync } from 'node:fs';
import { delimiter, join, resolve, sep } from 'node:path';

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
