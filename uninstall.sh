#!/bin/sh
# Relo uninstaller: detect how Relo was installed, remove it the same way,
# verify the result, and keep or delete the data as the operator asks.
#
#   curl -fsSL https://relo.ifelse.one/uninstall.sh | sh
#   curl -fsSL https://relo.ifelse.one/uninstall.sh | sh -s -- --wipe
set -eu

NPM_PACKAGE="@jonaskahn/relo"
RECEIPT=".relo-install"
INNER=76
TERMINAL=0
[ ! -t 1 ] || TERMINAL=1
INTERACTIVE=0
WIPE=0
KEEP=0
ASSUME_YES="${RELO_UNINSTALL_YES:-0}"
HOME_OPT="${RELO_HOME:-}"
DIR_OPT="${RELO_INSTALL_DIR:-}"

# ---------------------------------------------------------------- output ----

ESC="$(printf '\033')"
if [ -n "${NO_COLOR:-}" ] || [ "$TERMINAL" = 0 ]; then
	COLOR=0
else
	COLOR=1
fi
if [ "$COLOR" = 1 ]; then
	OFF="$(printf '\033[0m')"
	BRIGHT="$(printf '\033[38;2;155;188;15m')"
	LIGHT="$(printf '\033[38;2;139;172;15m')"
	MID="$(printf '\033[38;2;48;98;48m')"
	DANGER="$(printf '\033[38;2;214;60;60m')"
	BOLD="$(printf '\033[1m')"
else
	OFF=""; BRIGHT=""; LIGHT=""; MID=""; DANGER=""; BOLD=""
fi

UTF8=0
_probe="█"
if [ "$TERMINAL" = 1 ] && [ "${#_probe}" -eq 1 ]; then UTF8=1; fi

DASHES=""
_i=0
while [ "$_i" -lt "$INNER" ]; do DASHES="${DASHES}─"; _i=$((_i + 1)); done

FRAME=0
FIRST_SECTION=1

vislen() {
	_s="$(printf '%s' "$1" | sed "s/${ESC}\\[[0-9;]*m//g")"
	printf '%s' "${#_s}"
}

clip() {
	_rest="$1"
	_out=""
	_vis=0
	while [ -n "$_rest" ] && [ "$_vis" -lt $((INNER - 1)) ]; do
		case "$_rest" in
		"${ESC}"*)
			_out="$_out${_rest%%m*}m"
			_rest="${_rest#*m}"
			;;
		*)
			_head="${_rest#?}"
			_out="$_out${_rest%"$_head"}"
			_rest="$_head"
			_vis=$((_vis + 1))
			;;
		esac
	done
	printf '%s…' "$_out"
}

framed() {
	[ "$FRAME" = 1 ] || { printf '%s' "$1"; return 0; }
	_len="$(vislen "$1")"
	if [ "$_len" -gt "$INNER" ]; then _text="$(clip "$1")"; else _text="$1"; fi
	_len="$(vislen "$_text")"
	_pad=$((INNER - _len))
	[ "$_pad" -ge 0 ] || _pad=0
	printf '%s%s%*s%s' "${BRIGHT}│${OFF}" "$_text" "$_pad" '' "${BRIGHT}│${OFF}"
}

row() { printf '%s\n' "$(framed "$1")"; }
hline() { printf '%s%s%s\n' "${BRIGHT}$1${OFF}" "${BRIGHT}${DASHES}${OFF}" "${BRIGHT}$2${OFF}"; }
center_row() {
	_len="$(vislen "$1")"
	_left=$(((INNER - _len) / 2))
	[ "$_left" -ge 0 ] || _left=0
	row "$(printf '%*s%s' "$_left" '' "$1")"
}
say_plain() { printf '%s\n' "$*"; }
rule() { hline '├' '┤'; }
step() { row "  ${BRIGHT}▶${OFF} $*"; }
done_() { row "  ${BRIGHT}✓${OFF} $*"; }
note() { row "  ${MID}·${OFF} ${MID}$*${OFF}"; }
section() {
	if [ "$FIRST_SECTION" = 1 ]; then FIRST_SECTION=0; else rule; fi
	row ""
	row "  ${BOLD}$1${OFF}"
	row ""
}

screen_open() {
	if [ "$UTF8" = 0 ]; then
		say_plain ""
		say_plain "  RELO UNINSTALLER"
		say_plain "  $1"
		say_plain ""
		return 0
	fi
	FRAME=1
	hline '┌' '┐'
	row ''
	center_row "${BRIGHT}██████╗ ███████╗██╗      ██████╗ ${OFF}"
	center_row "${BRIGHT}██╔══██╗██╔════╝██║     ██╔═══██╗${OFF}"
	center_row "${BRIGHT}██████╔╝█████╗  ██║     ██║   ██║${OFF}"
	center_row "${BRIGHT}██╔══██╗██╔══╝  ██║     ██║   ██║${OFF}"
	center_row "${BRIGHT}██║  ██║███████╗███████╗╚██████╔╝${OFF}"
	center_row "${BRIGHT}╚═╝  ╚═╝╚══════╝╚══════╝ ╚═════╝ ${OFF}"
	row ''
	center_row "${BRIGHT}U N I N S T A L L E R${OFF}"
	row ''
	center_row "${LIGHT}$1${OFF}"
	row ''
	hline '├' '┤'
}

screen_close() {
	[ "$FRAME" = 1 ] || return 0
	FRAME=0
	hline '└' '┘'
}

error_box() {
	_title="$1"
	shift
	if [ "$UTF8" = 0 ]; then
		printf '  ! %s\n' "$_title" >&2
		for line in "$@"; do printf '    %s\n' "$line" >&2; done
		return 0
	fi
	FRAME=0
	printf '\n' >&2
	printf '%s%s%s\n' "${DANGER}┌${OFF}" "${DANGER}${DASHES}${OFF}" "${DANGER}┐${OFF}" >&2
	box_row ''
	box_row "  ${DANGER}!${OFF} ${BOLD}$_title${OFF}"
	box_row ''
	for line in "$@"; do box_row "    $line"; done
	box_row ''
	printf '%s%s%s\n' "${DANGER}└${OFF}" "${DANGER}${DASHES}${OFF}" "${DANGER}┘${OFF}" >&2
}

box_row() {
	_len="$(vislen "$1")"
	_pad=$((INNER - _len))
	[ "$_pad" -ge 0 ] || _pad=0
	printf '%s%s%*s%s\n' "${DANGER}│${OFF}" "$1" "$_pad" '' "${DANGER}│${OFF}" >&2
}

die() {
	screen_close
	error_box "$@"
	exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }
quote() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

# --------------------------------------------------------------- options ----

usage() {
	cat <<'EOF'
Relo uninstaller

Usage: uninstall.sh [options]

  --keep-data       keep the Relo data (configuration, credentials, logs)
  --wipe            remove the Relo data too
  --home <path>     Relo state directory (default ~/.relo)
  --dir <path>      directory holding a script-installed relo binary
  -y, --yes         use the defaults without prompting (keeps data)
  --help            this text

Environment: RELO_HOME, RELO_INSTALL_DIR, RELO_UNINSTALL_YES, NO_COLOR.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--keep-data) KEEP=1; WIPE=0; shift ;;
	--wipe) WIPE=1; KEEP=0; shift ;;
	--home) HOME_OPT="${2:?--home needs a value}"; shift 2 ;;
	--home=*) HOME_OPT="${1#*=}"; shift ;;
	--dir) DIR_OPT="${2:?--dir needs a value}"; shift 2 ;;
	--dir=*) DIR_OPT="${1#*=}"; shift ;;
	-y|--yes) ASSUME_YES=1; shift ;;
	-h|--help) usage; exit 0 ;;
	*) die "unknown option: $1" "run uninstall.sh --help" ;;
	esac
done

if [ "$KEEP" = 1 ] && [ "$WIPE" = 1 ]; then
	die "ask for one of --keep-data or --wipe"
fi

if [ "$TERMINAL" = 1 ] && [ "$ASSUME_YES" != 1 ] && ( : </dev/tty ) 2>/dev/null; then
	INTERACTIVE=1
fi

# ------------------------------------------------------------- detection ----

HOME_DIR="${HOME_OPT:-$HOME/.relo}"
APP=""
BIN=""
CHANNEL=""
FORMULA=""
NPM_ROOT=""

detect() {
	# A package that owns the relo command is found even when the command is
	# not on PATH, which is how a package manager can be asked to remove it.
	if have brew; then
		if brew list --formula relo >/dev/null 2>&1; then
			CHANNEL="brew"; FORMULA="relo"; return 0
		fi
		if brew list --formula relo-beta >/dev/null 2>&1; then
			CHANNEL="brew"; FORMULA="relo-beta"; return 0
		fi
	fi
	if have rpm && rpm -q relo >/dev/null 2>&1; then CHANNEL="rpm"; return 0; fi
	if have dpkg && dpkg -s relo >/dev/null 2>&1; then CHANNEL="deb"; return 0; fi
	if have npm; then
		NPM_ROOT="$(npm root -g 2>/dev/null || true)"
		if [ -n "$NPM_ROOT" ] && [ -d "$NPM_ROOT/$NPM_PACKAGE" ]; then
			CHANNEL="npm"; return 0
		fi
	fi

	for app in "/Applications/Relo.app" "$HOME/Applications/Relo.app"; do
		if [ -d "$app" ]; then APP="$app"; break; fi
	done
	if [ -n "$APP" ]; then
		CHANNEL="app"
		if [ -x "$APP/Contents/MacOS/Relo" ]; then BIN="$APP/Contents/MacOS/Relo"; fi
		return 0
	fi

	if [ -n "$DIR_OPT" ] && [ -e "$DIR_OPT/relo" ]; then
		BIN="$DIR_OPT/relo"
	else
		BIN="$(command -v relo 2>/dev/null || true)"
	fi

	if [ -n "$BIN" ] && [ -f "$(dirname "$BIN")/$RECEIPT" ]; then CHANNEL="script"; return 0; fi
	if [ -n "$BIN" ]; then CHANNEL="manual"; return 0; fi

	die "Relo was not found" \
		"Looked for Relo.app, brew/npm packages, apt/rpm packages, and relo on PATH." \
		"Pass --dir <folder> if the binary is in a custom folder."
}

channel_label() {
	case "$1" in
	app) printf 'macOS app bundle' ;;
	brew) printf 'Homebrew (%s)' "$FORMULA" ;;
	npm) printf 'npm (%s)' "$NPM_PACKAGE" ;;
	script) printf 'installer script' ;;
	deb) printf 'Debian package' ;;
	rpm) printf 'RPM package' ;;
	manual) printf 'manual install' ;;
	esac
}

command_label() {
	case "$1" in
	npm) printf 'npm uninstall -g %s' "$NPM_PACKAGE" ;;
	brew) printf 'brew uninstall %s' "$FORMULA" ;;
	app) printf 'rm -rf %s' "$(quote "$APP")" ;;
	script) printf 'rm -f %s %s' "$(quote "$(dirname "$BIN")/$RECEIPT")" "$(quote "$BIN")" ;;
	deb) printf 'sudo apt remove relo' ;;
	rpm) printf 'sudo dnf remove relo' ;;
	*) printf 'rm -f %s' "$(quote "$BIN")" ;;
	esac
}

ask_data() {
	[ "$KEEP" = 1 ] && return 0
	[ "$WIPE" = 1 ] && return 0
	if [ "$ASSUME_YES" = 1 ] || [ "$INTERACTIVE" != 1 ]; then
		note "Keeping the Relo data. Pass --wipe to remove it."
		return 0
	fi
	printf '%s  %s▶%s Remove your Relo data (configuration, credentials, logs)? [y/N] ' "${BRIGHT}│${OFF}" "$BRIGHT" "$OFF" >/dev/tty
	IFS= read -r answer </dev/tty || answer=""
	case "$(printf '%s' "$answer" | tr '[:upper:]' '[:lower:]' | tr -d ' ')" in
	y|yes) WIPE=1 ;;
	*) WIPE=0 ;;
	esac
}

settle_data() {
	if [ -n "$BIN" ] && [ -x "$BIN" ]; then
		step "Stopping Relo and settling its data"
		if [ "$WIPE" = 1 ]; then
			if "$BIN" --home "$HOME_DIR" uninstall --prepare --wipe >/dev/null 2>&1; then
				done_ "Service stopped and data removed"
				return 0
			fi
		else
			if "$BIN" --home "$HOME_DIR" uninstall --prepare >/dev/null 2>&1; then
				done_ "Service stopped; data kept"
				return 0
			fi
		fi
		note "The installed Relo could not stop itself; continuing with file removal."
	fi
	if [ "$WIPE" = 1 ]; then
		if [ -d "$HOME_DIR" ]; then
			rm -rf "$HOME_DIR" || die "Could not remove $HOME_DIR"
			done_ "Removed $HOME_DIR"
			note "Keychain entries were not removed because the Relo binary could not do it."
		fi
	else
		note "Keeping the Relo data in $HOME_DIR"
	fi
}

remove_program() {
	step "Removing $(channel_label "$CHANNEL")"
	case "$CHANNEL" in
	app)
		have osascript && osascript -e 'tell application id "com.relo.tray" to quit' >/dev/null 2>&1 || true
		rm -rf "$APP" || die "Could not remove $APP" "Run: rm -rf $(quote "$APP")"
		[ ! -d "$APP" ] || die "$APP is still present" "Run: rm -rf $(quote "$APP")"
		;;
	brew)
		if ! brew uninstall "$FORMULA"; then
			die "Homebrew did not remove $FORMULA" "Run: brew uninstall $FORMULA"
		fi
		if brew list --formula "$FORMULA" >/dev/null 2>&1; then
			die "$FORMULA is still installed" "Run: brew uninstall $FORMULA"
		fi
		;;
	npm)
		if ! npm uninstall -g "$NPM_PACKAGE"; then
			die "npm did not remove $NPM_PACKAGE" "Run: npm uninstall -g $NPM_PACKAGE"
		fi
		NPM_ROOT="$(npm root -g 2>/dev/null || true)"
		if [ -n "$NPM_ROOT" ] && [ -d "$NPM_ROOT/$NPM_PACKAGE" ]; then
			die "$NPM_PACKAGE is still installed" "Run: npm uninstall -g $NPM_PACKAGE"
		fi
		;;
	script)
		receipt="$(dirname "$BIN")/$RECEIPT"
		manual=$(command_label script)
		rm -f "$receipt" "$BIN" || die "Could not remove the script install" "Run: $manual"
		if [ -e "$BIN" ] || [ -e "$receipt" ]; then
			die "The script install is still present" "Run: $manual"
		fi
		;;
	manual)
		if ! rm -f "$BIN"; then
			die "Could not remove $BIN" "Run: rm -f $(quote "$BIN")"
		fi
		[ ! -e "$BIN" ] || die "$BIN is still present" "Run: rm -f $(quote "$BIN")"
		;;
	deb)
		note "Debian packages need an explicit privileged command."
		return 2
		;;
	rpm)
		note "RPM packages need an explicit privileged command."
		return 2
		;;
	esac
	done_ "Removed and verified"
}

summary_done() {
	rule
	row ''
	row "  ${BRIGHT}✓${OFF} ${BOLD}Relo is removed.${OFF}"
	row ''
	row "  ${MID}Channel${OFF}  $(channel_label "$CHANNEL")"
	if [ "$WIPE" = 1 ]; then
		row "  ${MID}Data${OFF}     removed"
	else
		row "  ${MID}Data${OFF}     $HOME_DIR"
	fi
	row ''
	screen_close
}

summary_pending() {
	rule
	row ''
	row "  ${BRIGHT}!${OFF} ${BOLD}Relo is still installed.${OFF}"
	row ''
	row "  ${MID}Channel${OFF}  $(channel_label "$CHANNEL")"
	if [ "$WIPE" = 1 ]; then
		row "  ${MID}Data${OFF}     removed"
	else
		row "  ${MID}Data${OFF}     $HOME_DIR"
	fi
	row ''
	row "  Run this to finish:"
	if [ "$CHANNEL" = "deb" ]; then
		row "    ${BOLD}sudo apt remove relo${OFF}"
		row "    ${MID}sudo apt purge relo   (also removes the data)${OFF}"
	else
		row "    ${BOLD}sudo dnf remove relo${OFF}"
	fi
	row ''
	screen_close
}

# ------------------------------------------------------------------ main ----

detect
LABEL="$(channel_label "$CHANNEL")"
screen_open "UNINSTALLER  ·  $LABEL"

section "Remove Relo"
row "  ${MID}Channel${OFF}   $LABEL"
if [ -n "$BIN" ]; then row "  ${MID}Binary${OFF}    $BIN"; fi
row "  ${MID}Data${OFF}      $HOME_DIR"
row ''

ask_data
settle_data

if [ "$CHANNEL" = "deb" ] || [ "$CHANNEL" = "rpm" ]; then
	remove_program || true
	summary_pending
	exit 0
fi

remove_program
summary_done
