// Runs before npm removes this package: the daemon is stopped, its ports are
// freed, and autostart is unregistered. The data is never touched here, because
// an npm removal keeps it -- `relo uninstall` is the way to wipe it.
//
// The work goes through the installed binary, which is the one thing that
// knows the configured ports and the login items. Without it the same quiesce
// runs in node, so a removal that lost the binary still frees what it can.
//
// Best effort by design: npm skips this with --ignore-scripts, and an
// uninstall must not fail over it.
import { existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

import { quiesce, reloHome } from './daemon.mjs';
import { target } from './platform.mjs';

const packageRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const binary = resolve(packageRoot, 'bin', target().name);
const home = reloHome();

try {
	if (existsSync(binary)) {
		spawnSync(binary, ['--home', home, 'uninstall', '--prepare'], { stdio: 'inherit' });
	} else {
		quiesce(home);
	}
} catch (error) {
	console.warn(`relo: could not stop the running daemon; ${error.message}`);
}