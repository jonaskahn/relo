// The Desktop shortcut a Windows install leaves, the same one Relo-Setup
// creates: it restarts the daemon through the GUI-subsystem relo.exe, so
// opening it shows no terminal. Other platforms have no such shortcut.
import { spawnSync } from 'node:child_process';

const NAME = 'Relo.lnk';

// run executes a PowerShell script with the values passed through the
// environment, so a path never needs quoting.
function run(script, env) {
	const result = spawnSync('powershell', ['-NoProfile', '-NonInteractive', '-Command', script], {
		env: { ...process.env, ...env },
		stdio: 'ignore'
	});
	return result.status === 0;
}

const DESKTOP = "$desktop = [Environment]::GetFolderPath('Desktop'); $link = Join-Path $desktop '" + NAME + "';";

// createDesktopShortcut writes the shortcut for binary and reports whether it
// exists afterwards. The working directory is the profile, because the
// install directory is not a place to write.
export function createDesktopShortcut(binary, { nodePlatform = process.platform } = {}) {
	if (nodePlatform !== 'win32') return false;
	return run(
		DESKTOP +
			'$s = (New-Object -ComObject WScript.Shell).CreateShortcut($link);' +
			'$s.TargetPath = $env:RELO_BINARY;' +
			"$s.Arguments = 'daemon restart --force';" +
			'$s.IconLocation = $env:RELO_BINARY;' +
			'$s.WorkingDirectory = $env:USERPROFILE;' +
			'$s.Save()',
		{ RELO_BINARY: binary }
	);
}

// removeDesktopShortcut deletes the shortcut; a missing one is fine.
export function removeDesktopShortcut({ nodePlatform = process.platform } = {}) {
	if (nodePlatform !== 'win32') return false;
	return run(DESKTOP + 'Remove-Item -LiteralPath $link -Force -ErrorAction SilentlyContinue', {});
}
