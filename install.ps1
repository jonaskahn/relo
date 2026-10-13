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
$DefaultPorts = @(10201, 10202, 10203)
$BarWidth = 20
$script:ConsoleReady = $false
$script:Terminal = $false
try { $script:Terminal = -not [Console]::IsOutputRedirected } catch { }

if ($env:RELO_INSTALL_YES -eq '1') { $Yes = $true }

# ---------------------------------------------------------------- output ----

$script:Color = $false
try { $script:Color = [bool]$Host.UI.SupportsVirtualTerminal } catch { $script:Color = $false }
if ($env:NO_COLOR) { $script:Color = $false }
if (-not $script:Terminal) { $script:Color = $false }

function Write-Step([string] $Text) { Write-Host ("  " + (Paint '>' 'red') + " $Text") }
function Write-OK([string] $Text) { Write-Host ("  " + (Paint '+' 'green') + " $Text") }
function Write-Note([string] $Text) { Write-Host ("  " + (Paint 'i' 'slate') + " $Text") }
function Write-Oops([string] $Text) { Write-Host ("  " + (Paint '!' 'red') + " $Text") }
function Write-Rule { Write-Host (Paint '  ----------------------------------------' 'dim') }
function Write-Section([string] $Number, [string] $Text) {
	Write-Host ''
	Write-Rule
	Write-Host ("  " + (Paint "$Number / 04" 'red') + "  " + (Paint $Text 'bold'))
	Write-Host ''
}

function Paint([string] $Text, [string] $Tone) {
	if (-not $script:Color) { return $Text }
	switch ($Tone) {
		'red' { return "$([char]27)[38;2;200;16;46m$Text$([char]27)[0m" }
		'rose' { return "$([char]27)[38;2;240;90;110m$Text$([char]27)[0m" }
		'green' { return "$([char]27)[38;2;80;200;140m$Text$([char]27)[0m" }
		'amber' { return "$([char]27)[38;2;230;170;60m$Text$([char]27)[0m" }
		'slate' { return "$([char]27)[38;2;140;140;155m$Text$([char]27)[0m" }
		'dim' { return "$([char]27)[2m$Text$([char]27)[0m" }
		'bold' { return "$([char]27)[1m$Text$([char]27)[0m" }
		default { return $Text }
	}
}

function Write-Banner {
	Write-Host ''
	if ($script:Terminal) {
		$block = [string][char]0x2588
		Write-Host ((Paint ('  ######  '.Replace('#', $block)) 'red') + (Paint ('####### ##       ######'.Replace('#', $block)) 'bold'))
		Write-Host ((Paint ('  ##   ## '.Replace('#', $block)) 'red') + (Paint ('##      ##      ##    ##'.Replace('#', $block)) 'bold'))
		Write-Host ((Paint ('  ######  '.Replace('#', $block)) 'red') + (Paint ('#####   ##      ##    ##'.Replace('#', $block)) 'bold'))
		Write-Host ((Paint ('  ##  ##  '.Replace('#', $block)) 'red') + (Paint ('##      ##      ##    ##'.Replace('#', $block)) 'bold'))
		Write-Host ((Paint ('  ##   ## '.Replace('#', $block)) 'red') + (Paint ('####### #######  ######'.Replace('#', $block)) 'bold'))
	} else {
		Write-Host '  RELO'
	}
	Write-Host ''
	Write-Host (Paint '  One daemon. Your tools. Every provider.' 'bold')
	Write-Host (Paint "  Let's get Relo ready on windows/$ArchName." 'dim')
	Write-Host ''
}

function Fail([string] $Message, [string[]] $Detail) {
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
	$filled = [int][Math]::Floor($Percent * $BarWidth / 100)
	$full = '#' * $filled
	$empty = '-' * ($BarWidth - $filled)
	return "$full$empty"
}

function Save-Remote([string] $Url, [string] $Destination, [string] $Label) {
	Write-Step "Downloading $Label"
	$total = if ($script:Terminal) { Get-RemoteLength $Url } else { 0 }
	$request = [System.Net.HttpWebRequest]::Create($Url)
	$request.UserAgent = 'relo-installer'
	$request.AllowAutoRedirect = $true
	$request.Timeout = 60000

	$response = $request.GetResponse()
	$stream = $response.GetResponseStream()
	$out = [System.IO.File]::Create($Destination)
	$buffer = New-Object byte[] 65536
	$written = 0
	$watch = [System.Diagnostics.Stopwatch]::StartNew()
	$lastDraw = [System.Diagnostics.Stopwatch]::StartNew()

	try {
		while (($read = $stream.Read($buffer, 0, $buffer.Length)) -gt 0) {
			$out.Write($buffer, 0, $read)
			$written += $read
			if (-not $script:Terminal) { continue }
			if ($lastDraw.ElapsedMilliseconds -lt 150) { continue }
			$lastDraw.Restart()
			if ($total -gt 0) {
				$percent = [int]($written * 100 / $total)
				if ($percent -gt 100) { $percent = 100 }
				$seconds = [Math]::Max($watch.Elapsed.TotalSeconds, 0.1)
				$rate = [int]($written / 1024 / $seconds)
				Write-Host ("`r    " + (Paint ("[" + (Get-Bar $percent) + "]") 'rose') +
					(" {0,3}%  {1}/{2} MB  {3} KB/s   " -f $percent, [int]($written / 1MB), [int]($total / 1MB), $rate)) -NoNewline
			} else {
				Write-Host ("`r    " + ("{0} MB downloaded   " -f [int]($written / 1MB))) -NoNewline
			}
		}
	} finally {
		$out.Close()
		$stream.Close()
		$response.Close()
	}

	if (-not $script:Terminal) {
		Write-OK "Downloaded $Label"
	} elseif ($total -gt 0) {
		Write-Host ("`r    " + (Paint ("[" + (Get-Bar 100) + "]") 'green') +
			(" 100%  {0}/{1} MB                " -f [int]($total / 1MB), [int]($total / 1MB)))
	} else {
		Write-Host ("`r    " + (Paint 'Download complete.' 'green') + '                    ')
	}
}

function Test-Checksum([string] $File, [string] $Sums, [string] $Name) {
	if (-not (Test-Path $Sums)) {
		Write-Note 'This release publishes no checksums.txt'
		return
	}
	$want = $null
	foreach ($line in Get-Content $Sums) {
		$parts = $line -split '\s+', 2
		if ($parts.Count -eq 2 -and $parts[1].Trim() -eq $Name) { $want = $parts[0].Trim() }
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
				elseif ($section -eq 'server.data_plane' -and $key -in @('openai', 'anthropic', 'gemini')) { $script:CheckPorts += $value }
			}
		}
	}
	if ($CheckPorts.Count -eq 0) {
		$script:CheckPorts = @($DefaultPort) + $DefaultPorts
		Write-Note 'No config.toml yet; using the default ports'
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
	Write-Step 'Checking the ports Relo serves'
	$foreign = @()
	foreach ($port in $CheckPorts) {
		if ($port -le 0) { continue }
		$holders = Get-PortHolders $port
		if ($holders.Count -eq 0) {
			Write-Host ("  " + (Paint 'i' 'slate') + (" {0,-6} available" -f $port))
			continue
		}
		foreach ($holder in $holders) {
			if ($holder.Name -match '^(?i)relo(-desktop)?$') {
				Write-Host ("  " + (Paint '+' 'green') + (" {0,-6} stopping {1} (pid {2})" -f $port, $holder.Name, $holder.Pid))
				Stop-Holder $holder
			} else {
				Write-Host ("  " + (Paint '!' 'amber') + (" {0,-6} held by {1} (pid {2})" -f $port, $holder.Name, $holder.Pid))
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
			Write-Host ''
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
	if ($LASTEXITCODE -ne 0) { Write-Note 'The daemon did not start on its own' }

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
	Write-Host ''
	Write-Rule
	Write-OK (Paint "Relo $ResolvedVersion is installed." 'bold')
	Write-Host ''
	Write-Host ("  " + (Paint 'Binary' 'dim') + "   $Bin")
	Write-Host ("  " + (Paint 'Data' 'dim') + "     $ReloHome")
	Write-Host ''
	if ($script:ConsoleReady) {
		Write-Host (Paint '  Your next step' 'bold')
		Write-Host '  Open the console and add your first connection:'
	} else {
		Write-Host (Paint '  Ready when you are' 'bold')
		Write-Host '  Start Relo, then open the console:'
		Write-Host "    & `"$Bin`" daemon start"
	}
	Write-Host (Paint "  http://127.0.0.1:$port" 'red')
	Write-Host ''
	Write-Host (Paint '  Useful commands' 'dim')
	Write-Host '    relo daemon status       Check how Relo is doing'
	Write-Host '    relo daemon restart      Restart Relo'
	Write-Host '    relo autostart disable   Turn off start at login'
	Write-Host ''
	Write-Note 'To update, run this installer again.'
	Write-Rule
	Write-Host ''
}

# -------------------------------------------------------------- the prompt --

function Read-Setup([string] $Prompt) {
	$value = Read-Host (Paint "  > $Prompt" 'red')
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
	Write-Banner
	Write-Section '01' 'Make it yours'
	Write-Host "  You're one Enter away. Want to make a change first?"
	Write-Host ''
	while ($true) {
		$startLabel = if ($NoStart) { Paint '[OFF]' 'slate' } else { Paint '[ON ]' 'green' }
		$autoLabel = if ($NoAutostart) { Paint '[OFF]' 'slate' } else { Paint '[ON ]' 'green' }
		if ($NoStart) { $autoLabel = Paint '[ --]' 'amber' }
		$versionLabel = if ($Version) { $Version } else { 'Latest stable' }
		$versionHint = if ($Version) { '(pinned release)' } else { '(recommended)' }
		Write-Host (Paint '  INSTALLATION' 'dim')
		Write-Rule
		Write-Host ("  " + (Paint '[1]' 'red') + ' ' + (Paint 'Release' 'bold'))
		Write-Host ("      $versionLabel " + (Paint $versionHint 'dim'))
		Write-Host ''
		Write-Host ("  " + (Paint '[2]' 'red') + ' ' + (Paint 'Install folder' 'bold'))
		Write-Host "      $Dir"
		Write-Host ''
		Write-Host (Paint '  ON YOUR SCHEDULE' 'dim')
		Write-Rule
		Write-Host ("  " + (Paint '[3]' 'red') + " $startLabel " + (Paint 'Start after install' 'bold'))
		Write-Host (Paint "      Have the console ready as soon as we're done." 'dim')
		Write-Host ''
		Write-Host ("  " + (Paint '[4]' 'red') + " $autoLabel " + (Paint 'Start at login' 'bold'))
		if ($NoStart) {
			Write-Host (Paint '      Not changed (start after install is off)' 'amber')
		} else {
			Write-Host (Paint '      Keep Relo ready whenever you sign in.' 'dim')
		}
		Write-Host ''
		Write-Host (Paint (Paint '  >>>  Press Enter to continue installation' 'bold') 'red')
		Write-Host (Paint '       1-4 to adjust a setting / q to cancel' 'dim')
		Write-Host ''
		$choice = Read-Setup 'What would you like to do?'
		Write-Host ''
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
		Write-Host ''
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

if ($interactive) {
	Invoke-Options
} else {
	Write-Banner
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
$asset = "relo-windows-$ArchName.exe"

try {
	Save-Remote "$Base/$asset" (Join-Path $temp 'relo.exe') $asset
	try {
		Save-Remote "$Base/checksums.txt" (Join-Path $temp 'checksums.txt') 'checksums.txt'
		Test-Checksum (Join-Path $temp 'relo.exe') (Join-Path $temp 'checksums.txt') $asset
	} catch {
		Write-Note 'This release publishes no checksums.txt'
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
} finally {
	Remove-Item -Recurse -Force $temp -ErrorAction SilentlyContinue
}
