// Maps the host onto a release asset name. The assets are the ones
// make build-all writes, so this translates Node's platform and arch into the
// Go names the release job already publishes rather than keeping a second
// table of its own.
import { arch, platform } from 'node:os';

const GOOS = { darwin: 'darwin', linux: 'linux', win32: 'windows' };
const GOARCH = { arm64: 'arm64', x64: 'amd64' };

const SUPPORTED = 'darwin and linux on amd64 and arm64, windows on amd64 and arm64.';

// target describes the daemon binary a host installs. The platform and arch
// default to this machine and take arguments so every host the package claims
// to support can be tested from one machine. It throws with a message naming
// the host when the release has no asset for it, so a failed install says
// which platform is missing instead of what a 404 means.
export function target({ platform: nodePlatform = platform(), arch: nodeArch = arch() } = {}) {
	const os = GOOS[nodePlatform];
	const cpu = GOARCH[nodeArch];
	if (!os || !cpu) {
		throw new Error(`relo publishes no binary for ${nodePlatform}-${nodeArch}. Supported: ${SUPPORTED}`);
	}
	return { os, arch: cpu, name: `relo-${os}-${cpu}${os === 'windows' ? '.exe' : ''}` };
}
