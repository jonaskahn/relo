#!/usr/bin/env node
// Entry point npm links as `relo`. It execs the platform binary the postinstall
// placed beside it, passing through arguments, streams, and exit status.
import { spawn } from 'node:child_process';
import { accessSync, constants } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { target } from '../lib/platform.mjs';

const binary = resolve(dirname(fileURLToPath(import.meta.url)), target().name);

try {
	accessSync(binary, constants.X_OK);
} catch {
	console.error(
		'relo: this install has no daemon binary. Reinstall without --ignore-scripts, or run\n' +
			'  node node_modules/relo/lib/install.mjs'
	);
	process.exit(1);
}

const child = spawn(binary, process.argv.slice(2), { stdio: 'inherit' });

// A terminal interrupt already reaches the child through the process group.
// These cover a service manager or a shell that signals only the launcher.
if (process.platform !== 'win32') {
	for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) {
		process.on(signal, () => child.kill(signal));
	}
}

child.on('error', (error) => {
	console.error(`relo: could not start ${binary}: ${error.message}`);
	process.exit(1);
});

child.on('exit', (code, signal) => {
	// A signalled child reports no code; pass the shell convention through so a
	// caller waiting on relo still sees a failure.
	process.exit(code ?? (signal === 'SIGINT' ? 130 : 1));
});
