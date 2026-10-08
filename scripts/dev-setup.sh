#!/usr/bin/env bash
# Writes the development state directory once, with its own console port, its
# own data plane ports, and no login-item registration, so a run started from
# an editor never collides with the daemon on ~/.relo. An existing
# configuration is left alone: a developer's edits outrank this script.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
home="${RELO_HOME:-$root/.dev/relo}"
port="${1:-10201}"
config="$home/config.toml"

mkdir -p "$home"
chmod 700 "$home"

if [ -f "$config" ]; then
  echo "dev: $config is already in place"
  exit 0
fi

cat >"$config" <<EOF
# The development state directory, written once by make dev-setup.

# No login item: a run started from an editor owns its own daemon.
[system]
autostart = false

[system.logging]
level = "debug"

[server]
bind = "127.0.0.1"
port = $port

# The data plane sits above the console port, so a development run never
# competes with a daemon on the default addresses.
[server.data_plane]
openai = $((port + 100))
anthropic = $((port + 101))
gemini = $((port + 102))

[secrets]
keychain = false

[ui]
language = "auto"
EOF

echo "dev: wrote $config (port $port)"
