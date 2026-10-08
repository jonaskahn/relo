# Writes the development state directory once, with its own console port, its
# own data plane ports, and no login-item registration, so a run started from
# an editor never collides with the daemon on ~/.relo. An existing
# configuration is left alone: a developer's edits outrank this script.
# Mirrors scripts/dev-setup.sh for Windows contributors without bash/make.
param([int]$Port = 10201)
$ErrorActionPreference = "Stop"

$root = if ($PSScriptRoot) { (Split-Path -Parent $PSScriptRoot) } else { (Get-Location).Path }
$homeDir = if ($env:RELO_HOME) { $env:RELO_HOME } else { Join-Path $root ".dev/relo" }
$config = Join-Path $homeDir "config.toml"

New-Item -ItemType Directory -Force -Path $homeDir | Out-Null

if (Test-Path $config) {
    Write-Host "dev: $config is already in place"
    exit 0
}

$openai = $Port + 100
$anthropic = $Port + 101
$gemini = $Port + 102
$text = @"
# The development state directory, written once by scripts/dev-setup.ps1.

# No login item: a run started from an editor owns its own daemon.
[system]
autostart = false

[system.logging]
level = "debug"

[server]
bind = "127.0.0.1"
port = $Port

# The data plane sits above the console port, so a development run never
# competes with a daemon on the default addresses.
[server.data_plane]
openai = $openai
anthropic = $anthropic
gemini = $gemini

[secrets]
keychain = false

[ui]
language = "auto"
"@
# No BOM: the Unix script writes plain UTF-8 and the TOML reader expects it.
[System.IO.File]::WriteAllText($config, $text, (New-Object System.Text.UTF8Encoding $false))

Write-Host "dev: wrote $config (port $Port)"
