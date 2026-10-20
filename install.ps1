# Relo installer for Windows: the daemon, from the latest release.
#
#   irm https://relo.ifelse.one/install.ps1 | iex
#
# With arguments (PowerShell cannot pass them through a pipe):
#
#   & ([scriptblock]::Create((irm https://relo.ifelse.one/install.ps1))) -NoStart
#
# The script installs exactly one binary. An installation that belongs to
# another channel -- the Relo-Setup installer or npm -- is refused instead of
# stacked, because two Relos fight over the same state directory and ports.
[CmdletBinding()]
param(
	[string] $Version = $env:RELO_VERSION,
	[string] $Dir = $env:RELO_INSTALL_DIR,
	[Alias('Home')]
	[string] $ReloHome = $env:RELO_HOME,
	[switch] $NoStart,
	[switch] $NoAutostart,
	[switch] $ForcePorts,
	[switch] $Yes,
	[switch] $Help
)

Set-StrictMode -Version 1.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Repo = 'jonaskahn/relo'
$Releases = if ($env:RELO_RELEASE_BASE) { $env:RELO_RELEASE_BASE } else { "https://github.com/$Repo/releases" }
$Receipt = '.relo-install'
$DefaultPort = 10101
$script:ConsoleReady = $false
$script:Terminal = $false
try { $script:Terminal = -not [Console]::IsOutputRedirected } catch { }

if ($env:RELO_INSTALL_YES -eq '1') { $Yes = $true }

# ---------------------------------------------------------------- output ----

$script:Color = $false
try { $script:Color = [bool]$Host.UI.SupportsVirtualTerminal } catch { $script:Color = $false }
if ($env:NO_COLOR) { $script:Color = $false }
if (-not $script:Terminal) { $script:Color = $false }

# One screen, like the npm package draws: a framed card with the logo, the
# release, and the steps inside the same border.
$script:Inner = 76
$script:BarWidth = 16
$script:Framed = $false
$script:FirstSection = $true
$script:Visual = $script:Terminal
$script:Esc = [char]27
$script:Logo = @(
	'██████╗ ███████╗██╗      ██████╗ ',
	'██╔══██╗██╔════╝██║     ██╔═══██╗',
	'██████╔╝█████╗  ██║     ██║   ██║',
	'██╔══██╗██╔══╝  ██║     ██║   ██║',
	'██║  ██║███████╗███████╗╚██████╔╝',
	'╚═╝  ╚═╝╚══════╝╚══════╝ ╚═════╝ '
)

function Paint([string] $Text, [string] $Tone) {
	if (-not $script:Color) { return $Text }
	# The four shades of a Game Boy screen.
	switch ($Tone) {
		'red' { return "$($script:Esc)[38;2;155;188;15m$Text$($script:Esc)[0m" }
		'green' { return "$($script:Esc)[38;2;155;188;15m$Text$($script:Esc)[0m" }
		'rose' { return "$($script:Esc)[38;2;139;172;15m$Text$($script:Esc)[0m" }
		'amber' { return "$($script:Esc)[38;2;139;172;15m$Text$($script:Esc)[0m" }
		'slate' { return "$($script:Esc)[38;2;48;98;48m$Text$($script:Esc)[0m" }
		'dim' { return "$($script:Esc)[38;2;48;98;48m$Text$($script:Esc)[0m" }
		'bold' { return "$($script:Esc)[1m$Text$($script:Esc)[0m" }
		default { return $Text }
	}
}

function Get-Visible([string] $Text) {
	return ($Text -replace "$($script:Esc)\[[0-9;]*m", '')
}

function Clip-Text([string] $Text) {
	$out = ''
	$vis = 0
	$i = 0
	while ($i -lt $Text.Length -and $vis -lt ($script:Inner - 1)) {
		if ($Text[$i] -eq $script:Esc) {
			$j = $Text.IndexOf('m', $i)
			if ($j -lt 0) { break }
			$out += $Text.Substring($i, $j - $i + 1)
			$i = $j + 1
			continue
		}
		$out += $Text[$i]
		$vis++
		$i++
	}
	return $out + [char]0x2026
}

function Format-Row([string] $Text) {
	if (-not $script:Framed) { return $Text }
	$visible = (Get-Visible $Text).Length
	if ($visible -gt $script:Inner) {
		$Text = Clip-Text $Text
		$visible = (Get-Visible $Text).Length
	}
	$pad = [Math]::Max(0, $script:Inner - $visible)
	return (Paint '│' 'bright') + $Text + (' ' * $pad) + (Paint '│' 'bright')
}

function Write-Row([string] $Text) { Write-Host (Format-Row $Text) }
function Clear-Line { if ($script:Visual) { Write-Host ("`r" + $script:Esc + "[2K") -NoNewline } }

function Write-Separator([string] $Left, [string] $Right) {
	$dash = [string][char]0x2500
	Write-Host (Paint ($Left + ($dash * $script:Inner) + $Right) 'bright')
}

function Write-Center([string] $Text, [string] $Tone) {
	$visible = (Get-Visible $Text).Length
	$left = [Math]::Max(0, [int](($script:Inner - $visible) / 2))
	Write-Row ((' ' * $left) + (Paint $Text $Tone))
}

function Write-ScreenOpen([string] $VersionLabel) {
	if (-not $script:Visual) {
		Write-Host ''
		Write-Host '  RELO INSTALLER'
		Write-Host "  $VersionLabel  windows/$ArchName"
		Write-Host ''
		return
	}
	$script:Framed = $true
	Write-Separator ([string][char]0x250C) ([string][char]0x2510)
	Write-Row ''
	foreach ($line in $script:Logo) { Write-Center $line 'bright' }
	Write-Row ''
	Write-Center 'I N S T A L L E R' 'bright'
	Write-Row ''
	Write-Center $VersionLabel 'rose'
	$platform = ("windows · {0}" -f $ArchName).ToUpper()
	Write-Center $platform 'rose'
	Write-Row ''
	Write-Separator ([string][char]0x251C) ([string][char]0x2524)
}

function Write-ScreenClose {
	if (-not $script:Framed) { return }
	$script:Framed = $false
	Write-Separator ([string][char]0x2514) ([string][char]0x2518)
}

function Write-Step([string] $Text) {
	if ($script:Visual) {
		Write-Host (Format-Row ("  " + (Paint '▶' 'bright') + " $Text")) -NoNewline
	} else {
		Write-Host "  > $Text"
	}
}

function Write-OK([string] $Text) { Clear-Line; Write-Row ("  " + (Paint '✓' 'bright') + " $Text") }
function Write-Note([string] $Text) { Clear-Line; Write-Row ("  " + (Paint '·' 'slate') + " " + (Paint $Text 'slate')) }
function Write-Oops([string] $Text) { Clear-Line; Write-Row ("  " + (Paint '!' 'bright') + " $Text") }
function Write-Rule { Write-Separator ([string][char]0x251C) ([string][char]0x2524) }
function Write-Lead([string] $Text) { Write-Row ("  " + (Paint '▶' 'bright') + " $Text") }

function Write-Section([string] $Number, [string] $Text) {
	if ($script:FirstSection) { $script:FirstSection = $false } else { Write-Rule }
	Write-Row ''
	Write-Row ("  " + (Paint $Number 'bright') + "  " + (Paint $Text 'bold'))
	Write-Row ''
}

function Get-Chip([string] $Text) {
	if (-not $script:Color) { return " ▶ $Text " }
	return "$($script:Esc)[48;2;155;188;15m$($script:Esc)[38;2;15;56;15m ▶ $Text $($script:Esc)[0m"
}

function Fail([string] $Message, [string[]] $Detail) {
	Clear-Line
	Write-ScreenClose
	Write-Host ''
	Write-Oops $Message
	foreach ($line in $Detail) { Write-Host "    $line" }
	exit 1
}

# --------------------------------------------------------------- options ----

if ($Help) {
	@'
Relo installer

Usage: install.ps1 [options]

  -Version <tag>   install a pinned release, for example -Version v0.1.0
  -Dir <path>      where the binary goes (default %LOCALAPPDATA%\Relo\bin)
  -Home <path>     Relo state directory to read ports from (default ~\.relo)
  -NoStart         do not start the daemon after installing
  -NoAutostart     do not register Relo to start at login
  -ForcePorts      take a Relo port from another program without asking
  -Yes             use the selected settings without prompting
  -Help            this text

Environment: RELO_VERSION, RELO_INSTALL_DIR, RELO_HOME, RELO_INSTALL_YES,
NO_COLOR. RELO_RELEASE_BASE overrides the download host.
'@ | Write-Host
	exit 0
}

$CheckPorts = @()
$ArchName = ''

function Get-Platform {
	if (-not [Environment]::Is64BitOperatingSystem) {
		Fail 'Relo needs a 64-bit Windows.' @()
	}
	switch -Regex ($env:PROCESSOR_ARCHITECTURE) {
		'^(AMD64|x86)$' { $script:ArchName = 'amd64' }
		'^ARM64$' { $script:ArchName = 'arm64' }
		default { Fail "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" @() }
	}
}

function Resolve-Paths {
	if (-not $Dir -or $Dir -eq '') {
		$script:Dir = Join-Path $env:LOCALAPPDATA 'Relo\bin'
	}
	$script:Dir = [Environment]::ExpandEnvironmentVariables($Dir)
	if (-not $ReloHome -or $ReloHome -eq '') {
		$script:ReloHome = Join-Path $env:USERPROFILE '.relo'
	}
	$script:Bin = Join-Path $Dir 'relo.exe'
}

# -------------------------------------------------------- foreign check -----

function Test-Foreign {
	foreach ($root in @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Relo',
			'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Relo',
			'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\Relo')) {
		if (Test-Path $root) {
			Fail 'Relo is already installed by Relo-Setup.' @(
				'This installer will not stack a second copy.',
				"Remove it first:  & `"$env:LOCALAPPDATA\Programs\Relo\uninstall.exe`" /S"
			)
		}
	}

	$npm = Get-Command npm -ErrorAction SilentlyContinue
	if ($npm) {
		$npmRoot = (& npm root -g 2>$null)
		if ($npmRoot -and (Test-Path (Join-Path $npmRoot '@jonaskahn\relo'))) {
			Fail 'Relo is already installed by npm.' @(
				'Remove it first:  npm uninstall -g @jonaskahn/relo'
			)
		}
	}

	$found = Get-Command relo -ErrorAction SilentlyContinue
	if ($found) {
		$here = Split-Path -Parent $found.Source
		if ((Test-Path (Join-Path $Dir $Receipt)) -and $here -eq $Dir) { return }
		if ($here -ne $Dir) {
			Fail "Another relo is already on PATH: $($found.Source)" @(
				"That one carries no $Receipt marker, so it came from another channel.",
				'Remove it, then run this installer again.'
			)
		}
	}
}

# --------------------------------------------------------------- download ---

function Get-RemoteLength([string] $Url) {
	try {
		$request = [System.Net.HttpWebRequest]::Create($Url)
		$request.Method = 'HEAD'
		$request.AllowAutoRedirect = $true
		$request.UserAgent = 'relo-installer'
		$response = $request.GetResponse()
		$length = $response.ContentLength
		$response.Close()
		return [int64]$length
	} catch {
		return 0
	}
}

function Get-Bar([int] $Percent) {
	$filled = [int][Math]::Floor($Percent * $script:BarWidth / 100)
	$full = [string][char]0x2588
	$empty = [string][char]0x2591
	return (Paint ($full * $filled) 'bright') + (Paint ($empty * ($script:BarWidth - $filled)) 'slate')
}

function Save-Remote([string] $Url, [string] $Destination, [string] $Label) {
	Write-Step "Downloading $Label"
	$request = [System.Net.HttpWebRequest]::Create($Url)
	$request.UserAgent = 'relo-installer'
	$request.AllowAutoRedirect = $true
	$request.Timeout = 60000

	$response = $request.GetResponse()
	$stream = $response.GetResponseStream()
	$out = [System.IO.File]::Create($Destination)
	$buffer = New-Object byte[] 65536
	$written = 0
	# The GET response carries the asset length where a HEAD may answer 0.
	$total = [int64]$response.ContentLength
	if ($total -le 0) { $total = Get-RemoteLength $Url }
	if ($total -lt 0) { $total = 0 }
	$watch = [System.Diagnostics.Stopwatch]::StartNew()
	$lastDraw = [System.Diagnostics.Stopwatch]::StartNew()

	try {
		while (($read = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
			$out.Write($buffer, 0, $read)
			$written += $read
			if (-not $script:Visual) { continue }
			if ($lastDraw.ElapsedMilliseconds -lt 150) { continue }
			$lastDraw.Restart()
			$seconds = [Math]::Max($watch.Elapsed.TotalSeconds, 0.1)
			$rate = [int]($written / 1024 / $seconds)
			if ($total -gt 0) {
				$percent = [int]($written * 100 / $total)
				if ($percent -gt 100) { $percent = 100 }
				$eta = ''
				if ($rate -gt 0 -and $written -lt $total) {
					$eta = "  ETA $([Math]::Max(1, [int](($total - $written) / ($rate * 1024))))s"
				}
				$line = "  " + (Get-Chip 'DOWNLOAD') + " [" + (Get-Bar $percent) + "] $percent%  $([int]($written / 1MB))/$([int]($total / 1MB)) MB  $rate KB/s$eta"
			} else {
				$line = "  " + (Get-Chip 'DOWNLOAD') + " $([int]($written / 1MB)) MB  $rate KB/s"
			}
			Write-Host ("`r" + $script:Esc + "[2K" + (Format-Row $line)) -NoNewline
		}
	} finally {
		$out.Close()
		$stream.Close()
		$response.Close()
	}

	Write-OK "Downloaded $Label ($([int]($written / 1MB)) MB)"
}

function Test-Checksum([string] $File, [string] $Sums, [string] $Name) {
	if (-not (Test-Path $Sums)) {
		Write-Note 'This release publishes no checksums.txt'
		return
	}
	$want = $null
	foreach ($line in Get-Content $Sums) {
		$parts = $line -split '\s+', 2
		if ($parts.Count -eq 2 -and $parts[1].Trim().TrimStart('*') -eq $Name) { $want = $parts[0].Trim() }
	}
	if (-not $want) {
		Write-Note "no checksum published for $Name; skipping verification"
		return
	}
	$got = (Get-FileHash $File -Algorithm SHA256).Hash.ToLower()
	if ($got -ne $want.ToLower()) {
		Fail "checksum mismatch for $Name" @(
			"expected  $want",
			"     got  $got",
			'The download was corrupted or tampered with. Nothing was installed.'
		)
	}
	Write-OK 'Checksum verified'
}

# -------------------------------------------------------------- the ports ---

function Read-Ports {
	$config = Join-Path $ReloHome 'config.toml'
	$script:CheckPorts = @()
	if (Test-Path $config) {
		$section = ''
		foreach ($line in Get-Content $config) {
			$trimmed = $line -replace '#.*$', ''
			if ($trimmed -match '^\s*\[([^\]]+)\]\s*$') {
				$section = $Matches[1].Trim()
				continue
			}
			if ($trimmed -match '^\s*([A-Za-z_]+)\s*=\s*"?([0-9]+)"?\s*$') {
				$key = $Matches[1]
				$value = [int]$Matches[2]
				if ($section -eq 'server' -and $key -eq 'port') { $script:CheckPorts += $value }
			}
		}
	}
	if ($CheckPorts.Count -eq 0) {
		$script:CheckPorts = @($DefaultPort)
		Write-Note 'No config.toml yet; using the default port'
	} else {
		$script:CheckPorts = $CheckPorts | Select-Object -Unique
		Write-Note "Ports read from $config"
	}
}

function Get-PortHolders([int] $Port) {
	$holders = @()
	try {
		$connections = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction Stop
		foreach ($connection in $connections) {
			$process = Get-Process -Id $connection.OwningProcess -ErrorAction SilentlyContinue
			if ($process) { $holders += [pscustomobject]@{ Pid = $process.Id; Name = $process.ProcessName } }
		}
	} catch {
		$netstat = & netstat -ano 2>$null | Select-String "LISTENING" | Select-String ":$Port\s"
		foreach ($match in $netstat) {
			$fields = ($match.ToString() -split '\s+') | Where-Object { $_ -ne '' }
			$ownerPid = [int]$fields[-1]
			$process = Get-Process -Id $ownerPid -ErrorAction SilentlyContinue
			if ($process) { $holders += [pscustomobject]@{ Pid = $process.Id; Name = $process.ProcessName } }
		}
	}
	return $holders
}

function Stop-Holder($Holder) {
	try { Stop-Process -Id $Holder.Pid -Force -ErrorAction SilentlyContinue } catch { }
}

function Stop-Daemon {
	$binary = Join-Path $Dir 'relo.exe'
	if (-not (Test-Path $binary)) {
		$found = Get-Command relo -ErrorAction SilentlyContinue
		if (-not $found) { return }
		$binary = $found.Source
	}
	Write-Step 'Stopping the running daemon'
	& $binary daemon stop --force 2>$null | Out-Null
	if ($LASTEXITCODE -eq 0) { Write-OK 'Daemon stopped' } else { Write-Note 'No daemon was running' }
	Get-Process -Name 'relo' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}

function Clear-Ports {
	Write-Lead 'Checking the ports Relo serves'
	$foreign = @()
	foreach ($port in $CheckPorts) {
		if ($port -le 0) { continue }
		$holders = Get-PortHolders $port
		if ($holders.Count -eq 0) {
			Write-Row ("  " + (Paint '·' 'slate') + (" $port available"))
			continue
		}
		foreach ($holder in $holders) {
			if ($holder.Name -match '^(?i)relo(-desktop)?$') {
				Write-Row ("  " + (Paint '✓' 'bright') + (" $port stopping $($holder.Name) (pid $($holder.Pid))"))
				Stop-Holder $holder
			} else {
				Write-Row ("  " + (Paint '!' 'bright') + (" $port held by $($holder.Name) (pid $($holder.Pid))"))
				$foreign += $port
			}
		}
	}

	if ($foreign.Count -gt 0) {
		$list = ' ' + (($foreign | Select-Object -Unique) -join ' ')
		if (-not $ForcePorts) {
			if ($Yes -or -not $interactive) {
				Fail "Ports$list are held by another program" @(
					'Nothing was installed.',
					'Rerun with -ForcePorts to take them, or free them yourself.'
				)
			}
			Write-Row ''
			Write-Note 'Stopping another program may interrupt its work.'
			if (-not (Confirm-Setup "Stop the program using ports$list?")) {
				Fail "Ports$list are held by another program" @('Nothing was installed.')
			}
		}
		foreach ($port in ($foreign | Select-Object -Unique)) {
			foreach ($holder in (Get-PortHolders $port)) {
				if ($holder.Name -notmatch '^(?i)relo(-desktop)?$') { Stop-Holder $holder }
			}
		}
	}

	Start-Sleep -Milliseconds 800
	foreach ($port in $CheckPorts) {
		if ($port -le 0) { continue }
		foreach ($holder in (Get-PortHolders $port)) { Stop-Holder $holder }
	}
	Write-OK 'Ports ready'
}

# ------------------------------------------------------------- the daemon ---

function Add-ToPath([string] $Directory) {
	$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
	if (-not $userPath) { $userPath = '' }
	$entries = $userPath -split ';' | Where-Object { $_ -ne '' }
	if ($entries -notcontains $Directory) {
		[Environment]::SetEnvironmentVariable('Path', (($entries + $Directory) -join ';'), 'User')
		Write-OK "Added $Directory to PATH"
	}
	if (($env:Path -split ';') -notcontains $Directory) {
		$env:Path = "$Directory;$env:Path"
	}
}

function Start-Daemon {
	if ($NoStart) {
		Write-Note 'Leaving the daemon stopped (-NoStart)'
		return
	}

	Write-Step 'Starting the daemon'
	& $Bin daemon start 2>$null | Out-Null
	if ($LASTEXITCODE -eq 0) { Write-OK 'Daemon started' } else { Write-Note 'The daemon did not start on its own' }

	if (-not $NoAutostart) {
		& $Bin autostart enable 2>$null | Out-Null
		if ($LASTEXITCODE -eq 0) { Write-OK 'Autostart enabled' } else { Write-Note 'Autostart was not changed' }
	}

	$port = if ($CheckPorts.Count -gt 0) { $CheckPorts[0] } else { $DefaultPort }
	Write-Step 'Waiting for the console to respond'
	for ($i = 0; $i -lt 20; $i++) {
		try {
			$body = Invoke-RestMethod "http://127.0.0.1:$port/healthz" -TimeoutSec 2
			$script:ConsoleReady = $true
			Write-OK "Console is ready at http://127.0.0.1:$port"
			if ($body.version) { Write-Note "Reported version: $($body.version)" }
			return
		} catch {
			Start-Sleep -Milliseconds 500
		}
	}
	Write-Note 'The console is not answering yet. Try: relo daemon status'
}

function Write-Summary {
	$port = if ($CheckPorts.Count -gt 0) { $CheckPorts[0] } else { $DefaultPort }
	Write-Rule
	Write-OK (Paint "Relo $ResolvedVersion is installed." 'bold')
	Write-Row ''
	Write-Row ("  " + (Paint 'Binary' 'slate') + "   $Bin")
	Write-Row ("  " + (Paint 'Data' 'slate') + "     $ReloHome")
	Write-Row ''
	if ($script:ConsoleReady) {
		Write-Row (Paint '  Your next step' 'bold')
		Write-Row '  Open the console and add your first connection:'
	} else {
		Write-Row (Paint '  Ready when you are' 'bold')
		Write-Row '  Start Relo, then open the console:'
		Write-Row "    & `"$Bin`" daemon start"
	}
	Write-Row ("  " + (Paint "http://127.0.0.1:$port" 'bright'))
	Write-Row ''
	Write-Row (Paint '  Useful commands' 'slate')
	Write-Row '    relo daemon status       Check how Relo is doing'
	Write-Row '    relo daemon restart      Restart Relo'
	Write-Row '    relo autostart disable   Turn off start at login'
	Write-Row ''
	Write-Note 'To update, run this installer again.'
}

# -------------------------------------------------------------- the prompt --

function Read-Setup([string] $Prompt) {
	if ($script:Framed) {
		# Leave the row open while reading: the terminal's caret then sits
		# right after the question, inside the frame.
		Write-Host ((Paint '│' 'bright') + "  " + (Paint '▶' 'bright') + " $Prompt ") -NoNewline
		$value = Read-Host
		Write-Host ("`r" + $script:Esc + "[1A" + $script:Esc + "[2K") -NoNewline
		Write-Row ("  " + (Paint '▶' 'bright') + " $Prompt " + $value)
	} else {
		$value = Read-Host (Paint "  > $Prompt" 'red')
	}
	if ($null -eq $value) { Fail 'Setup canceled: the terminal input closed.' @() }
	return $value.Trim()
}

function Confirm-Setup([string] $Prompt) {
	while ($true) {
		$answer = Read-Setup "$Prompt [y/N]"
		switch -Regex ($answer) {
			'^(y|yes)$' { return $true }
			'^(n|no)?$' { return $false }
			default { Write-Note 'Please type y for yes or n for no. Enter means no.' }
		}
	}
}

function Invoke-Options {
	Write-Section '01' 'Make it yours'
	Write-Row "  You're one Enter away. Want to make a change first?"
	Write-Row ''
	while ($true) {
		$startLabel = if ($NoStart) { Paint '[OFF]' 'slate' } else { Paint '[ON ]' 'green' }
		$autoLabel = if ($NoAutostart) { Paint '[OFF]' 'slate' } else { Paint '[ON ]' 'green' }
		if ($NoStart) { $autoLabel = Paint '[ --]' 'amber' }
		$versionLabel = if ($Version) { $Version } else { 'Latest stable' }
		$versionHint = if ($Version) { '(pinned release)' } else { '(recommended)' }
		Write-Row (Paint '  INSTALLATION' 'dim')
		Write-Rule
		Write-Row ("  " + (Paint '[1]' 'red') + ' ' + (Paint 'Release' 'bold'))
		Write-Row ("      $versionLabel " + (Paint $versionHint 'dim'))
		Write-Row ''
		Write-Row ("  " + (Paint '[2]' 'red') + ' ' + (Paint 'Install folder' 'bold'))
		Write-Row "      $Dir"
		Write-Row ''
		Write-Row (Paint '  ON YOUR SCHEDULE' 'dim')
		Write-Rule
		Write-Row ("  " + (Paint '[3]' 'red') + " $startLabel " + (Paint 'Start after install' 'bold'))
		Write-Row (Paint "      Have the console ready as soon as we're done." 'dim')
		Write-Row ''
		Write-Row ("  " + (Paint '[4]' 'red') + " $autoLabel " + (Paint 'Start at login' 'bold'))
		if ($NoStart) {
			Write-Row (Paint '      Not changed (start after install is off)' 'amber')
		} else {
			Write-Row (Paint '      Keep Relo ready whenever you sign in.' 'dim')
		}
		Write-Row ''
		Write-Row (Paint (Paint '  >>>  Press Enter to continue installation' 'bold') 'red')
		Write-Row (Paint '       1-4 to adjust a setting / q to cancel' 'dim')
		Write-Row ''
		$choice = Read-Setup 'What would you like to do?'
		Write-Row ''
		switch ($choice) {
			'' { }
			'1' {
				Write-Note 'Enter a release such as v0.1.0. Leave blank for latest stable.'
				$value = Read-Setup 'Which release?'
				$script:Version = $value
			}
			'2' {
				Write-Note "Press Enter to keep $Dir."
				$value = Read-Setup 'Where should Relo be installed?'
				if ($value) { $script:Dir = $value }
				Resolve-Paths
			}
			'3' { $script:NoStart = [switch](-not [bool]$NoStart) }
			'4' { $script:NoAutostart = [switch](-not [bool]$NoAutostart) }
			'q' { Write-Note 'Setup canceled. Nothing was changed.'; exit 0 }
			default { Write-Note 'Choose 1, 2, 3 or 4; Enter to install; q to cancel.' }
		}
		if ($choice -eq '') { break }
		Write-Row ''
	}
	Resolve-Paths
}

# ---------------------------------------------------------------- install ---

function Resolve-Version {
	if ($Version) {
		$tag = if ($Version.StartsWith('v')) { $Version } else { "v$Version" }
		$script:ResolvedVersion = $tag.Substring(1)
		$script:Base = "$Releases/download/$tag"
		return
	}
	$script:Base = "$Releases/latest/download"
	$script:ResolvedVersion = 'latest'
	try {
		$request = [System.Net.HttpWebRequest]::Create("$Releases/latest")
		$request.AllowAutoRedirect = $true
		$request.Method = 'HEAD'
		$response = $request.GetResponse()
		$effective = $response.ResponseUri.AbsoluteUri
		$response.Close()
		$tag = ($effective -replace '/+$', '') -replace '.*/', ''
		if ($tag) { $script:ResolvedVersion = $tag.TrimStart('v') }
	} catch { }
}

Get-Platform
Resolve-Paths

$interactive = $false
try { $interactive = $script:Terminal -and -not $Yes -and -not [Console]::IsInputRedirected } catch { $interactive = $false }

$versionLabel = if ($Version) { $Version } else { 'latest release' }
Write-ScreenOpen $versionLabel
if ($interactive) {
	Invoke-Options
} else {
	Write-Section '01' 'Use your selected settings'
	Write-Note 'Running without prompts. Use -Help to see setup options.'
}

Test-Foreign
Write-OK 'No other Relo installation is in the way'

Write-Section '02' 'Download Relo'
Resolve-Version
Write-Note "Release $ResolvedVersion"

$temp = Join-Path ([System.IO.Path]::GetTempPath()) ("relo-install." + [guid]::NewGuid().ToString('N').Substring(0, 8))
New-Item -ItemType Directory -Path $temp -Force | Out-Null
$rawAsset = "relo-windows-$ArchName.exe"

try {
	# One manifest lists every asset in the release. A release carrying the
	# raw binary uses it; one carrying only the versioned tarballs falls
	# back to this host's archive (same rule as the npm installer).
	try {
		Save-Remote "$Base/checksums.txt" (Join-Path $temp 'checksums.txt') 'checksums.txt'
	} catch {
		Fail 'no checksums.txt in the release' @(
			'No release may be published yet; rerun with -Version <tag> for a published tag.'
		)
	}
	$entries = @{}
	foreach ($line in Get-Content (Join-Path $temp 'checksums.txt')) {
		if ($line -match '^[0-9a-fA-F]{64}\s+\*?(.+?)\s*$') { $entries[$Matches[1].Trim()] = $true }
	}
	if ($entries.ContainsKey($rawAsset)) {
		$kind = 'raw'
		$asset = $rawAsset
	} else {
		$kind = 'tarball'
		$asset = $null
		foreach ($key in $entries.Keys) {
			if ($key -match ("^relo-(.+)-windows-" + [regex]::Escape($ArchName) + "\.tar\.gz$")) {
				$asset = $key
				$ResolvedVersion = $Matches[1]
				break
			}
		}
		if (-not $asset) {
			Fail "no $rawAsset or versioned tarball in the release" @(
				'No release may be published yet; rerun with -Version <tag> for a published tag.'
			)
		}
	}

	if ($kind -eq 'raw') {
		Save-Remote "$Base/$asset" (Join-Path $temp 'relo.exe') $asset
		Test-Checksum (Join-Path $temp 'relo.exe') (Join-Path $temp 'checksums.txt') $asset
	} else {
		if (-not (Get-Command tar -ErrorAction SilentlyContinue)) {
			Fail 'tar is required to install Relo' @('Install tar, then run this installer again.')
		}
		Save-Remote "$Base/$asset" (Join-Path $temp 'bundle.tar.gz') $asset
		Test-Checksum (Join-Path $temp 'bundle.tar.gz') (Join-Path $temp 'checksums.txt') $asset
		Write-Lead "Extracting $asset"
		& tar -xzf (Join-Path $temp 'bundle.tar.gz') -C $temp
		if ($LASTEXITCODE -ne 0) { Fail "could not unpack $asset" @() }
		Write-OK "Extracted $asset"
		if (-not (Test-Path (Join-Path $temp 'relo.exe'))) { Fail 'the release archive holds no relo.exe' @() }
	}

	Write-Section '03' 'Install Relo'
	Stop-Daemon
	Read-Ports
	Clear-Ports

	Write-Step "Installing to $Dir"
	New-Item -ItemType Directory -Path $Dir -Force | Out-Null
	Move-Item -Force (Join-Path $temp 'relo.exe') $Bin
	Write-OK "Installed at $Bin"

	@(
		'channel=script'
		"version=$ResolvedVersion"
		'os=windows'
		"arch=$ArchName"
		"installed=$([DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'))"
	) | Set-Content -Path (Join-Path $Dir $Receipt) -Encoding ASCII

	Add-ToPath $Dir

	Write-Section '04' 'Get you ready'
	Start-Daemon
	Write-Summary
	Write-ScreenClose
} finally {
	Remove-Item -Recurse -Force $temp -ErrorAction SilentlyContinue
}
