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

function Paint([string] $Text, [string] $Tone) {
	if (-not $script:Color) { return $Text }
	switch ($Tone) {
		'bright' { return "$($script:Esc)[38;2;155;188;15m$Text$($script:Esc)[0m" }
		'rose' { return "$($script:Esc)[38;2;139;172;15m$Text$($script:Esc)[0m" }
		'slate' { return "$($script:Esc)[38;2;48;98;48m$Text$($script:Esc)[0m" }
		'bold' { return "$($script:Esc)[1m$Text$($script:Esc)[0m" }
		default { return $Text }
	}
}

function Get-Visible([string] $Text) { return ($Text -replace "$($script:Esc)\[[0-9;]*m", '') }
function Format-Row([string] $Text) {
	if (-not $script:Framed) { return $Text }
	$visible = (Get-Visible $Text).Length
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

function Read-DataChoice {
	if ($KeepData -or $Wipe) { return }
	if ($Yes -or -not $script:Terminal) {
		Write-Note 'Keeping the Relo data. Pass -Wipe to remove it.'
		return
	}
	$answer = Read-Host (Paint '  > Remove your Relo data (configuration, credentials, logs)? [y/N]' 'bright')
	if ($answer -match '^(?i)y(es)?$') { $script:Wipe = $true }
}

function Stop-Relo {
	if (-not $script:Binary -or -not (Test-Path $script:Binary)) { return }
	Write-Step 'Stopping Relo and settling its data'
	$arguments = @('--home', $script:ReloHome, 'uninstall', '--prepare')
	if ($script:Wipe) { $arguments += '--wipe' }
	& $script:Binary @arguments 2>$null | Out-Null
	if ($LASTEXITCODE -eq 0) {
		if ($script:Wipe) { Write-OK 'Service stopped and data removed' } else { Write-OK 'Service stopped; data kept' }
		return
	}
	Write-Note 'The installed Relo could not stop itself; continuing with file removal.'
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
			& npm uninstall -g $NpmPackage
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
Read-DataChoice
Write-ScreenOpen ("UNINSTALLER  $([char]0x00B7)  $(Get-ChannelLabel)")
Write-Section 'Remove Relo'
Write-Row ('  ' + (Paint 'Channel' 'slate') + "   $(Get-ChannelLabel)")
if ($script:Binary) { Write-Row ('  ' + (Paint 'Binary' 'slate') + "    $script:Binary") }
Write-Row ('  ' + (Paint 'Data' 'slate') + "      $script:ReloHome")
Write-Row ''
Stop-Relo
Remove-Program
Write-Summary
