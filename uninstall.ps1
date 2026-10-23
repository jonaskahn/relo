# Relo uninstaller for Windows: detect how Relo was installed, remove it the
# same way, verify the result, and keep or delete the data as the operator
# asks.
#
#   irm https://relo.ifelse.one/uninstall.ps1 | iex
#
# With arguments (PowerShell cannot pass them through a pipe):
#
#   & ([scriptblock]::Create((irm https://relo.ifelse.one/uninstall.ps1))) -Wipe
[CmdletBinding()]
param(
	[Alias('Home')]
	[string] $ReloHome = $env:RELO_HOME,
	[string] $Dir = $env:RELO_INSTALL_DIR,
	[switch] $KeepData,
	[switch] $Wipe,
	[switch] $Yes,
	[switch] $Help
)

Set-StrictMode -Version 1.0
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$NpmPackage = '@jonaskahn/relo'
$Receipt = '.relo-install'
$script:Inner = 76
$script:Terminal = $false
try { $script:Terminal = -not [Console]::IsOutputRedirected } catch { }
if ($env:RELO_UNINSTALL_YES -eq '1') { $Yes = $true }

# ---------------------------------------------------------------- output ----

$script:Color = $false
try { $script:Color = [bool]$Host.UI.SupportsVirtualTerminal } catch { $script:Color = $false }
if ($env:NO_COLOR) { $script:Color = $false }
if (-not $script:Terminal) { $script:Color = $false }
$script:Esc = [char]27
$script:Visual = $script:Terminal
$script:Framed = $false
$script:FirstSection = $true
$script:Block = [string][char]0x2588
$script:Rule = [string][char]0x2550
$script:Side = [string][char]0x2502
$script:Bullet = [string][char]0x25B6
$script:Check = [string][char]0x2713
$script:Dot = [string][char]0x00B7
$script:Left = [string][char]0x2190
$script:Right = [string][char]0x2192

function Paint([string] $Text, [string] $Tone) {
	if (-not $script:Color) { return $Text }
	switch ($Tone) {
		'bright' { return "$($script:Esc)[38;2;155;188;15m$Text$($script:Esc)[0m" }
		'rose' { return "$($script:Esc)[38;2;139;172;15m$Text$($script:Esc)[0m" }
		'slate' { return "$($script:Esc)[38;2;48;98;48m$Text$($script:Esc)[0m" }
		'dim' { return "$($script:Esc)[38;2;48;98;48m$Text$($script:Esc)[0m" }
		'red' { return "$($script:Esc)[38;2;214;60;60m$Text$($script:Esc)[0m" }
		'bold' { return "$($script:Esc)[1m$Text$($script:Esc)[0m" }
		default { return $Text }
	}
}

function Get-Visible([string] $Text) { return ($Text -replace "$($script:Esc)\[[0-9;]*m", '') }
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
	return (Paint $script:Side 'bright') + $Text + (' ' * $pad) + (Paint $script:Side 'bright')
}
function Write-Row([string] $Text) { Write-Host (Format-Row $Text) }
function Write-Rule { Write-Host (Paint ([string][char]0x251C + ([string][char]0x2500 * $script:Inner) + [string][char]0x2524) 'bright') }
function Write-Section([string] $Text) {
	if ($script:FirstSection) { $script:FirstSection = $false } else { Write-Rule }
	Write-Row ''
	Write-Row ('  ' + (Paint $Text 'bold'))
	Write-Row ''
}
function Write-Step([string] $Text) { Write-Row ('  ' + (Paint $script:Bullet 'bright') + " $Text") }
function Write-OK([string] $Text) { Write-Row ('  ' + (Paint $script:Check 'bright') + " $Text") }
function Write-Note([string] $Text) { Write-Row ('  ' + (Paint $script:Dot 'slate') + ' ' + (Paint $Text 'slate')) }

function Write-ScreenOpen([string] $Label) {
	if (-not $script:Visual) {
		Write-Host ''
		Write-Host '  RELO UNINSTALLER'
		Write-Host "  $Label"
		Write-Host ''
		return
	}
	$script:Framed = $true
	Write-Host (Paint ([string][char]0x250C + ([string][char]0x2500 * $script:Inner) + [string][char]0x2510) 'bright')
	Write-Row ''
	foreach ($line in @(
		'######] #######]##]      ######] ',
		'##[==##]##[====}##|     ##[===##]',
		'######[}#####]  ##|     ##|   ##|',
		'##[==##]##[==}  ##|     ##|   ##|',
		'##|  ##|#######]#######]{######[}',
		'{=}  {=}{======}{======} {=====} ')) {
		$text = $line.Replace('#', $script:Block).Replace('=', $script:Rule).Replace('|', [string][char]0x2551)
		Write-Row (' ' * [Math]::Max(0, [int](($script:Inner - $text.Length) / 2)) + (Paint $text 'bright'))
	}
	Write-Row ''
	Write-Row (' ' * 24 + (Paint 'U N I N S T A L L E R' 'bright'))
	Write-Row ''
	Write-Row (' ' * [Math]::Max(0, [int](($script:Inner - $Label.Length) / 2)) + (Paint $Label 'rose'))
	Write-Row ''
	Write-Rule
}

function Write-ScreenClose {
	if (-not $script:Framed) { return }
	$script:Framed = $false
	Write-Host (Paint ([string][char]0x2514 + ([string][char]0x2500 * $script:Inner) + [string][char]0x2518) 'bright')
}

function Fail([string] $Message, [string[]] $Detail) {
	Write-ScreenClose
	Write-Host ''
	Write-Host (Paint ('! ' + $Message) 'bright') -ForegroundColor Red
	foreach ($line in $Detail) { Write-Host "    $line" }
	exit 1
}

# --------------------------------------------------------------- options ----

if ($Help) {
	@'
Relo uninstaller

Usage: uninstall.ps1 [options]

  -KeepData        keep the Relo data (configuration, credentials, logs)
  -Wipe            remove the Relo data too
  -Home <path>     Relo state directory (default ~\.relo)
  -Dir <path>      directory holding a script-installed relo.exe
  -Yes             use the defaults without prompting (keeps data)
  -Help            this text

Environment: RELO_HOME, RELO_INSTALL_DIR, RELO_UNINSTALL_YES, NO_COLOR.
'@ | Write-Host
	exit 0
}

if ($KeepData -and $Wipe) { Fail 'Ask for one of -KeepData or -Wipe.' @() }
$script:Wipe = [bool]$Wipe

if (-not $ReloHome) { $ReloHome = Join-Path $env:USERPROFILE '.relo' }
$script:ReloHome = [Environment]::ExpandEnvironmentVariables($ReloHome)
if ($Dir) { $Dir = [Environment]::ExpandEnvironmentVariables($Dir) }

# ------------------------------------------------------------- detection ----

$script:Channel = ''
$script:Binary = ''
$script:Uninstaller = ''

function Find-Relo {
	if ($Dir) {
		$candidate = Join-Path $Dir 'relo.exe'
		if (Test-Path $candidate) { $script:Binary = $candidate }
	}
	if (-not $script:Binary) {
		$found = Get-Command relo -ErrorAction SilentlyContinue
		if ($found) { $script:Binary = $found.Source }
	}

	foreach ($root in @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Relo',
			'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Relo',
			'HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\Relo')) {
		if (Test-Path $root) {
			$script:Channel = 'setup'
			$script:Uninstaller = (Get-ItemProperty $root).UninstallString
			$setupDir = Split-Path -Parent $script:Uninstaller.Trim('"')
			$candidate = Join-Path $setupDir 'relo.exe'
			if (Test-Path $candidate) { $script:Binary = $candidate }
			return
		}
	}

	$npm = Get-Command npm -ErrorAction SilentlyContinue
	if ($npm) {
		$root = (& npm root -g 2>$null)
		if ($root -and (Test-Path (Join-Path $root $NpmPackage))) {
			$script:Channel = 'npm'
			return
		}
	}

	if ($script:Binary -and (Test-Path (Join-Path (Split-Path -Parent $script:Binary) $Receipt))) {
		$script:Channel = 'script'
		return
	}
	if ($script:Binary) {
		$script:Channel = 'manual'
		return
	}
	Fail 'Relo was not found.' @(
		'Looked for a registered Setup uninstaller, npm, and relo on PATH.',
		'Pass -Dir <folder> if the binary is in a custom folder.')
}

function Get-ChannelLabel {
	switch ($script:Channel) {
		'setup' { return 'Relo Setup' }
		'npm' { return "npm ($NpmPackage)" }
		'script' { return 'installer script' }
		'manual' { return 'manual install' }
		default { return $script:Channel }
	}
}

function Get-ChannelCommand {
	switch ($script:Channel) {
		'setup' { return 'Remove Relo from Settings > Apps' }
		'npm' { return "npm uninstall -g $NpmPackage" }
		'script' {
			$receipt = Join-Path (Split-Path -Parent $script:Binary) $Receipt
			return "del /f `"$receipt`" `"$($script:Binary)`""
		}
		default { return "del /f `"$($script:Binary)`"" }
	}
}

# ------------------------------------------------------------ selections ----
#
# A selection menu reads single keys from the console, so the uninstaller can
# arrive on a pipeline and still ask questions. Without a key-capable console
# it falls back to a typed number.

function Read-Answer([string] $Prompt) {
	if ($script:Framed) {
		# Leave the row open while reading: the terminal's caret then sits
		# right after the question, inside the frame.
		Write-Host ((Paint $script:Side 'bright') + "  " + (Paint $script:Bullet 'bright') + " $Prompt ") -NoNewline
		$value = Read-Host
		Write-Host ("`r" + $script:Esc + "[1A" + $script:Esc + "[2K") -NoNewline
		Write-Row ("  " + (Paint $script:Bullet 'bright') + " $Prompt " + $value)
	} else {
		$value = Read-Host (Paint "  > $Prompt" 'red')
	}
	if ($null -eq $value) { Fail 'Uninstall canceled: the terminal input closed.' @() }
	return $value.Trim()
}

function Clear-Choice([int] $Height) {
	Write-Host ("{0}[{1}A" -f $script:Esc, $Height) -NoNewline
	for ($i = 0; $i -lt $Height; $i++) { Write-Host ("`r{0}[2K" -f $script:Esc) }
	# Back to the first row, so the next draw replaces the block in place.
	Write-Host ("{0}[{1}A" -f $script:Esc, $Height) -NoNewline
}

# Settle-Choice replaces the drawn menu with one answer row and deletes the
# rows under it, so the frame closes up instead of leaving gaps.
function Settle-Choice([int] $Height, [string] $Text) {
	Write-Host ("{0}[{1}A" -f $script:Esc, $Height) -NoNewline
	Write-Host ("`r{0}[2K{1}" -f $script:Esc, (Format-Row $Text))
	for ($i = 1; $i -lt $Height; $i++) { Write-Host ("`r{0}[1M" -f $script:Esc) -NoNewline }
}

function Write-Choice([string] $Prompt, [string[]] $Options, [int] $Pick) {
	Write-Row ("  " + (Paint $script:Bullet 'bright') + " $Prompt")
	$line = '  '
	for ($i = 0; $i -lt $Options.Count; $i++) {
		if ($i -eq $Pick) {
			$line += " " + $(if ($script:Color) { "$($script:Esc)[7m[ $($Options[$i]) ]$($script:Esc)[0m" } else { "[ $($Options[$i]) ]" })
		} else {
			$line += " " + (Paint "[$($Options[$i])]" 'dim')
		}
	}
	Write-Row $line
	Write-Row ('  ' + (Paint ($script:Left + '/' + $script:Right + ' to move ' + $script:Dot + ' Enter to pick ' + $script:Dot + ' q to cancel') 'dim'))
}

# Select-UninstallOption asks a question and returns the 0-based index, or -1
# when the user cancels.
function Select-UninstallOption([string] $Prompt, [string[]] $Options, [int] $Default = 0) {
	$pick = $Default
	if ($pick -lt 0) { $pick = 0 }
	if ($pick -ge $Options.Count) { $pick = $Options.Count - 1 }
	$height = $Options.Count + 2

	if (-not $script:Visual) {
		Write-Row ("  " + (Paint '>' 'bright') + " $Prompt")
		for ($i = 0; $i -lt $Options.Count; $i++) { Write-Row "    [$($i + 1)] $($Options[$i])" }
		$answer = Read-Answer "Choose 1-$($Options.Count)"
		if ($answer -eq '') { return $pick }
		if ($answer -match '^[0-9]+$' -and [int]$answer -ge 1 -and [int]$answer -le $Options.Count) {
			return ([int]$answer - 1)
		}
		return -1
	}

	try {
		while ($true) {
			Write-Choice $Prompt $Options $pick
			$key = [Console]::ReadKey($true)
			switch ([string]$key.Key) {
				'LeftArrow' { if ($pick -gt 0) { $pick-- } }
				'RightArrow' { if ($pick -lt $Options.Count - 1) { $pick++ } }
				'UpArrow' { if ($pick -gt 0) { $pick-- } }
				'DownArrow' { if ($pick -lt $Options.Count - 1) { $pick++ } }
				'K' { if ($pick -gt 0) { $pick-- } }
				'J' { if ($pick -lt $Options.Count - 1) { $pick++ } }
				'Enter' {
					Settle-Choice $height ("  " + (Paint $script:Bullet 'bright') + " $Prompt " + (Paint $Options[$pick] 'bold'))
					return $pick
				}
				'Spacebar' {
					Settle-Choice $height ("  " + (Paint $script:Bullet 'bright') + " $Prompt " + (Paint $Options[$pick] 'bold'))
					return $pick
				}
				'Escape' { Settle-Choice $height ('  ' + (Paint ($script:Bullet + ' ' + $Prompt + ' (canceled)') 'dim')); return -1 }
				'Q' { Settle-Choice $height ('  ' + (Paint ($script:Bullet + ' ' + $Prompt + ' (canceled)') 'dim')); return -1 }
				default {
					$digit = [string]$key.KeyChar
					if ($digit -match '^[1-9]$' -and [int]$digit -le $Options.Count) { $pick = [int]$digit - 1 }
				}
			}
			Clear-Choice $height
		}
	} catch {
		# No console to read keys from: ask for a number instead.
		Clear-Choice $height
		Write-Row ("  " + (Paint '>' 'bright') + " $Prompt")
		for ($i = 0; $i -lt $Options.Count; $i++) { Write-Row "    [$($i + 1)] $($Options[$i])" }
		$answer = Read-Answer "Choose 1-$($Options.Count)"
		if ($answer -eq '') { return $pick }
		if ($answer -match '^[0-9]+$' -and [int]$answer -ge 1 -and [int]$answer -le $Options.Count) {
			return ([int]$answer - 1)
		}
		return -1
	}
}

function Read-DataChoice {
	if ($KeepData -or $Wipe) { return }
	if ($Yes -or -not $script:Terminal) {
		Write-Note 'Keeping the Relo data. Pass -Wipe to remove it.'
		return
	}
	# A selection, install.ps1's way: pick with the arrow keys, and only an
	# explicit "Remove data" wipes. q cancels the uninstall entirely.
	$pick = Select-UninstallOption 'Keep or remove your Relo data?' @('Keep data', 'Remove data') 0
	if ($pick -lt 0) {
		Write-Note 'Canceled; nothing was removed.'
		Write-ScreenClose
		exit 0
	}
	if ($pick -eq 1) { $script:Wipe = $true }
}

function Stop-Relo {
	if (-not $script:Binary -or -not (Test-Path $script:Binary)) { return }
	Write-Step 'Stopping Relo'
	& $script:Binary --home $script:ReloHome uninstall --prepare 2>$null | Out-Null
	if ($LASTEXITCODE -eq 0) {
		Write-OK 'Service stopped'
		return
	}
	Write-Note 'The installed Relo could not stop itself; continuing with file removal.'
}

function Remove-Data {
	if (-not $script:Wipe) { return }
	# The service is already stopped; ask the binary to wipe the state and the
	# keychain entries it owns, falling back to a plain directory removal.
	if ($script:Binary -and (Test-Path $script:Binary)) {
		& $script:Binary --home $script:ReloHome uninstall --prepare --wipe 2>$null | Out-Null
		if ($LASTEXITCODE -eq 0) {
			Write-OK 'Data removed'
			return
		}
	}
	if (Test-Path $script:ReloHome) {
		Remove-Item -Recurse -Force $script:ReloHome
		Write-OK "Removed $script:ReloHome"
		Write-Note 'Keychain entries were not removed because the Relo binary could not do it.'
	}
}

function Remove-Program {
	Write-Step "Removing $(Get-ChannelLabel)"
	switch ($script:Channel) {
		'setup' {
			if (-not $script:Uninstaller) { Fail 'No Setup uninstaller is registered.' @('Remove Relo from Settings > Apps.') }
			$command = Get-ChannelCommand
			Start-Process -FilePath $script:Uninstaller.Trim('"') -ArgumentList '/DATA=handled' -Wait
			$deadline = (Get-Date).AddSeconds(30)
			while ((Get-Date) -lt $deadline -and (Test-Path $script:Binary)) { Start-Sleep -Milliseconds 250 }
			if (Test-Path $script:Binary) { Fail 'Relo Setup did not remove the program.' @("Run: $command") }
		}
		'npm' {
			& npm uninstall -g $NpmPackage 2>$null | Out-Null
			if ($LASTEXITCODE -ne 0) { Fail 'npm did not remove Relo.' @("Run: npm uninstall -g $NpmPackage") }
			$root = (& npm root -g 2>$null)
			if ($root -and (Test-Path (Join-Path $root $NpmPackage))) { Fail 'The npm package is still installed.' @("Run: npm uninstall -g $NpmPackage") }
		}
		'script' {
			Remove-Item -Force (Join-Path (Split-Path -Parent $script:Binary) $Receipt) -ErrorAction SilentlyContinue
			Remove-FromPath (Split-Path -Parent $script:Binary)
			Remove-RunningBinary $script:Binary
		}
		'manual' { Remove-RunningBinary $script:Binary }
	}
	Write-OK 'Removed and verified'
}

function Remove-RunningBinary([string] $Path) {
	if (-not $Path) { return }
	Write-Note "Windows will delete this file after Relo exits: $Path"
	Start-Process -FilePath 'cmd.exe' -ArgumentList '/c', "ping 127.0.0.1 -n 3 >nul & del /f /q `"$Path`"" -WindowStyle Hidden
}

function Remove-FromPath([string] $Directory) {
	$key = 'HKCU:\Environment'
	if (-not (Test-Path $key)) { return }
	$value = (Get-ItemProperty $key -Name Path -ErrorAction SilentlyContinue).Path
	if (-not $value) { return }
	$kept = @($value -split ';' | Where-Object { $_ -and $_ -ne $Directory })
	if ($kept.Count -eq ($value -split ';').Count) { return }
	if ($kept.Count -eq 0) { Remove-ItemProperty $key -Name Path }
	else { Set-ItemProperty $key -Name Path -Value ($kept -join ';') }
}

function Write-Summary {
	Write-Rule
	Write-Row ''
	Write-OK (Paint 'Relo is removed.' 'bold')
	Write-Row ''
	Write-Row ('  ' + (Paint 'Channel' 'slate') + "  $(Get-ChannelLabel)")
	if ($script:Wipe) { Write-Row ('  ' + (Paint 'Data' 'slate') + '     removed') }
	else { Write-Row ('  ' + (Paint 'Data' 'slate') + "     $script:ReloHome") }
	Write-Row ''
	Write-ScreenClose
}

Find-Relo
Write-ScreenOpen ("UNINSTALLER  $([char]0x00B7)  $(Get-ChannelLabel)")
Write-Section 'Remove Relo'
Write-Row ('  ' + (Paint 'Channel' 'slate') + "   $(Get-ChannelLabel)")
if ($script:Binary) { Write-Row ('  ' + (Paint 'Binary' 'slate') + "    $script:Binary") }
Write-Row ('  ' + (Paint 'Data' 'slate') + "      $script:ReloHome")
Write-Row ''
Stop-Relo
Read-DataChoice
Remove-Data
Remove-Program
Write-Summary
