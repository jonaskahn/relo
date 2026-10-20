#!/bin/sh
# Relo installer: the daemon, from the latest release, on macOS and Linux.
#
#   curl -fsSL https://relo.ifelse.one/install.sh | sh
#   curl -fsSL https://relo.ifelse.one/install.sh | sh -s -- --beta
#
# The script installs exactly one binary. An installation that belongs to
# another channel -- Homebrew, npm, the macOS app, a .deb or a .rpm -- is
# refused instead of stacked, because two Relos fight over the same state
# directory and the same ports.
set -eu

REPO="jonaskahn/relo"
RELEASES="${RELO_RELEASE_BASE:-https://github.com/$REPO/releases}"
DEFAULT_PORT=10101
RECEIPT=".relo-install"
INNER=76
BAR_WIDTH=16
TERMINAL=0
[ ! -t 1 ] || TERMINAL=1
CONSOLE_READY=0

# ---------------------------------------------------------------- output ----
#
# One screen, like the npm package draws: a framed card with the logo, the
# release, and the steps inside the same border. Rows pad by character count,
# so the frame needs a UTF-8 locale and a terminal; without either, the same
# lines print plainly.

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
	DARK="$(printf '\033[38;2;15;56;15m')"
	BOLD="$(printf '\033[1m')"
else
	OFF=""; BRIGHT=""; LIGHT=""; MID=""; DARK=""; BOLD=""
fi
# The older names keep painting the same screen.
RED="$BRIGHT"; GREEN="$BRIGHT"; AMBER="$LIGHT"; SLATE="$MID"; DIM="$MID"

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
row_tty() { framed "$1" >/dev/tty; printf '\n' >/dev/tty; }
hline() { printf '%s%s%s\n' "${BRIGHT}$1${OFF}" "${BRIGHT}${DASHES}${OFF}" "${BRIGHT}$2${OFF}"; }

center_row() {
	_len="$(vislen "$1")"
	_left=$(((INNER - _len) / 2))
	[ "$_left" -ge 0 ] || _left=0
	row "$(printf '%*s%s' "$_left" '' "$1")"
}

chip() {
	if [ "$COLOR" = 1 ]; then
		printf '\033[48;2;155;188;15m%s ▶ %s \033[0m' "$DARK" "$1"
	else
		printf ' ▶ %s ' "$1"
	fi
}

say() { row "$*"; }
say_plain() { printf '%s\n' "$*"; }
rule() { hline '├' '┤'; }
clear_line() { [ "$UTF8" = 1 ] && printf '\r%s' "${ESC}[2K"; }

step() {
	if [ "$UTF8" = 1 ]; then
		printf '%s' "$(framed "  ${BRIGHT}▶${OFF} $*")"
	else
		printf '  %s>%s %s\n' "$BRIGHT" "$OFF" "$*"
	fi
}

lead() { row "  ${BRIGHT}▶${OFF} $*"; }
done_() { clear_line; row "  ${BRIGHT}✓${OFF} $*"; }
note() { clear_line; row "  ${MID}·${OFF} ${MID}$*${OFF}"; }
oops() { clear_line; row "  ${BRIGHT}!${OFF} $*"; }

section() {
	if [ "$FIRST_SECTION" = 1 ]; then FIRST_SECTION=0; else hline '├' '┤'; fi
	row ""
	row "  ${BOLD}$1${OFF}"
	row ""
}

screen_open() {
	if [ "$UTF8" = 0 ]; then
		say_plain ""
		say_plain "  RELO INSTALLER"
		say_plain "  $VERSION_LINE  $OS/$ARCH"
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
	center_row "${BRIGHT}I N S T A L L E R${OFF}"
	row ''
	center_row "${LIGHT}$VERSION_LINE${OFF}"
	center_row "${LIGHT}$(printf '%s · %s' "$OS" "$ARCH" | tr '[:lower:]' '[:upper:]')${OFF}"
	row ''
	hline '├' '┤'
}

screen_close() {
	[ "$FRAME" = 1 ] || return 0
	FRAME=0
	hline '└' '┘'
}

die() {
	clear_line
	screen_close
	printf '  %s!%s %s\n' "$BRIGHT" "$OFF" "$1" >&2
	shift
	for line in "$@"; do printf '    %s\n' "$line" >&2; done
	exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

# --------------------------------------------------------------- options ----

VERSION="${RELO_VERSION:-}"
CHANNEL="stable"
INSTALL_DIR="${RELO_INSTALL_DIR:-}"
RELO_HOME_OPT="${RELO_HOME:-}"
START_AFTER=1
AUTOSTART=1
START_SET=0
AUTOSTART_SET=0
ASSUME_YES="${RELO_INSTALL_YES:-0}"
FORCE_PORTS=0
INTERACTIVE=0

usage() {
	cat <<'EOF'
Relo installer

Usage: install.sh [options]

  --version <tag>   install a pinned release, for example --version v0.1.0
  --beta            install the newest beta instead of the latest stable
  --dir <path>      where the binary goes (default ~/.local/bin)
  --home <path>     Relo state directory to read ports from (default ~/.relo)
  --no-start        do not start the daemon after installing
  --no-autostart    do not register Relo to start at login
  --force-ports     take a Relo port from another program without asking
  -y, --yes         use the selected settings without prompting
  --help            this text

Environment: RELO_VERSION, RELO_INSTALL_DIR, RELO_HOME, RELO_INSTALL_YES,
NO_COLOR. RELO_RELEASE_BASE overrides the download host.
EOF
}

while [ $# -gt 0 ]; do
	case "$1" in
	--version) VERSION="${2:?--version needs a value}"; shift 2 ;;
	--version=*) VERSION="${1#*=}"; shift ;;
	--beta) CHANNEL="beta"; shift ;;
	--dir) INSTALL_DIR="${2:?--dir needs a value}"; shift 2 ;;
	--dir=*) INSTALL_DIR="${1#*=}"; shift ;;
	--home) RELO_HOME_OPT="${2:?--home needs a value}"; shift 2 ;;
	--home=*) RELO_HOME_OPT="${1#*=}"; shift ;;
	--no-start) START_AFTER=0; START_SET=1; shift ;;
	--no-autostart) AUTOSTART=0; AUTOSTART_SET=1; shift ;;
	--force-ports) FORCE_PORTS=1; shift ;;
	-y | --yes) ASSUME_YES=1; shift ;;
	-h | --help) usage; exit 0 ;;
	*) die "unknown option: $1" "run install.sh --help" ;;
	esac
done

# The script usually arrives on stdin, so prompts come from the terminal.
if [ "$TERMINAL" = 1 ] && [ "$ASSUME_YES" = 0 ] && ( : </dev/tty ) 2>/dev/null; then
	INTERACTIVE=1
fi

ask() {
	# ask <prompt> <fallback>
	if [ "$UTF8" = 1 ] && [ "$FRAME" = 1 ]; then
		# Leave the row open while reading: the terminal's caret then sits
		# right after the question, inside the frame.
		printf '%s  %s▶%s %s ' "${BRIGHT}│${OFF}" "$BRIGHT" "$OFF" "$1" >/dev/tty
		IFS= read -r answer </dev/tty || die "Setup canceled: the terminal input closed."
		printf '%s' "${ESC}[1A${ESC}[2K" >/dev/tty
		row_tty "  ${BRIGHT}▶${OFF} $1 ${answer:-$2}"
	else
		printf '  %s> %s%s ' "$BRIGHT" "$1" "$OFF" >/dev/tty
		IFS= read -r answer </dev/tty || die "Setup canceled: the terminal input closed."
	fi
	[ -n "$answer" ] || answer="$2"
	printf '%s' "$answer"
}

# ------------------------------------------------------------- selections --
#
# A selection menu reads single keys from /dev/tty, so the installer can be
# piped in on stdin and still ask questions on the terminal. Without a
# terminal it falls back to a typed number.

MENU_TTY_STATE=""
MENU_PICK=1

menu_enter() {
	[ "$INTERACTIVE" = 1 ] && [ "$UTF8" = 1 ] || return 1
	MENU_TTY_STATE="$(stty -g </dev/tty 2>/dev/null || true)"
	[ -n "$MENU_TTY_STATE" ] || return 1
	stty -echo -icanon min 1 time 0 </dev/tty 2>/dev/null || return 1
	trap 'stty "$MENU_TTY_STATE" </dev/tty 2>/dev/null || true; exit 130' INT TERM HUP
}

menu_leave() {
	if [ -n "$MENU_TTY_STATE" ]; then
		stty "$MENU_TTY_STATE" </dev/tty 2>/dev/null || true
		MENU_TTY_STATE=""
	fi
	trap - INT TERM HUP
}

# Keys are read as byte values: a character captured through $(...) loses a
# trailing newline, and Enter is exactly that byte.
read_key() {
	_code="$(dd if=/dev/tty bs=1 count=1 2>/dev/null | od -An -tu1 | tr -d ' ')"
	[ -n "$_code" ] || return 1
	printf '%s' "$_code"
}

# read_key_timed prints nothing when no key follows: it tells a lone Escape
# from the bracket that opens an arrow key sequence.
read_key_timed() {
	stty min 0 time 1 </dev/tty 2>/dev/null || true
	_code="$(dd if=/dev/tty bs=1 count=1 2>/dev/null | od -An -tu1 | tr -d ' ')"
	stty min 1 time 0 </dev/tty 2>/dev/null || true
	printf '%s' "$_code"
}

MENU_HEIGHT=3

menu_rewind() {
	[ "$1" -gt 0 ] && printf '%s[%dA' "$ESC" "$1" >/dev/tty
}

# menu_row paints the button line: the chosen button reversed, the rest plain.
menu_row() {
	_line="  "
	_i=1
	for _item in "$@"; do
		if [ "$_i" = "$MENU_PICK" ]; then
			_line="$_line ${MENU_REV}[ $_item ]${MENU_OFF}"
		else
			_line="$_line ${DIM}[$_item]${OFF}"
		fi
		_i=$((_i + 1))
	done
	row_tty "$_line"
}

menu_draw() {
	_prompt="$1"
	shift
	row_tty "  ${BRIGHT}▶${OFF} $_prompt"
	menu_row "$@"
	row_tty "  ${DIM}←/→ to move · Enter to pick · q to cancel${OFF}"
}

# menu_settle replaces the drawn menu with one answer row. The rows under it
# are deleted, not blanked, so the frame closes up instead of leaving gaps.
menu_settle() {
	menu_rewind "$MENU_HEIGHT"
	printf '\r%s[2K%s\n' "$ESC" "$(framed "$1")" >/dev/tty
	_i=1
	while [ "$_i" -lt "$MENU_HEIGHT" ]; do
		printf '\r%s[M' "$ESC" >/dev/tty
		_i=$((_i + 1))
	done
}

menu_done() {
	menu_settle "  ${BRIGHT}▶${OFF} $MENU_PROMPT ${BOLD}${MENU_ANSWER}${OFF}"
}

menu_stop() {
	menu_settle "  ${DIM}▶ $MENU_PROMPT (canceled)${OFF}"
	menu_leave
}

# menu_select <prompt> <default> <option>...; prints the chosen option and
# returns 0. Returns 1 when the user cancels.
menu_select() {
	MENU_PROMPT="$1"
	MENU_PICK="$2"
	shift 2
	_count=$#
	[ "$MENU_PICK" -ge 1 ] || MENU_PICK=1
	[ "$MENU_PICK" -le "$_count" ] || MENU_PICK="$_count"
	if [ "$COLOR" = 1 ]; then
		MENU_REV="$(printf '\033[7m')"
		MENU_OFF="$(printf '\033[0m')"
	else
		MENU_REV=""
		MENU_OFF=""
	fi

	if ! menu_enter; then
		printf '  %s>%s %s\n' "$BRIGHT" "$OFF" "$MENU_PROMPT" >/dev/tty
		_i=1
		for _item in "$@"; do
			printf '    [%s] %s\n' "$_i" "$_item" >/dev/tty
			_i=$((_i + 1))
		done
		printf '  %s>%s Choose 1-%s: ' "$BRIGHT" "$OFF" "$_count" >/dev/tty
		IFS= read -r _answer </dev/tty || return 1
		case "$_answer" in
		'') ;;
		*[!0-9]*) return 1 ;;
		*)
			[ "$_answer" -ge 1 ] && [ "$_answer" -le "$_count" ] || return 1
			MENU_PICK="$_answer"
			;;
		esac
	else
		while :; do
			menu_draw "$MENU_PROMPT" "$@"
			_key="$(read_key)" || {
				menu_stop
				return 1
			}
			case "$_key" in
			10 | 13 | 32) break ;;
			113 | 81) menu_stop; return 1 ;;
			104 | 107) [ "$MENU_PICK" -gt 1 ] && MENU_PICK=$((MENU_PICK - 1)) ;;
			106 | 108) [ "$MENU_PICK" -lt "$_count" ] && MENU_PICK=$((MENU_PICK + 1)) ;;
			27)
				_code="$(read_key_timed)"
				if [ "$_code" = 91 ] || [ "$_code" = 79 ]; then
					_code="$(read_key_timed)"
					case "$_code" in
					65 | 68) [ "$MENU_PICK" -gt 1 ] && MENU_PICK=$((MENU_PICK - 1)) ;;
					66 | 67) [ "$MENU_PICK" -lt "$_count" ] && MENU_PICK=$((MENU_PICK + 1)) ;;
					esac
				else
					menu_stop
					return 1
				fi
				;;
			esac
			if [ "$_key" -ge 49 ] && [ "$_key" -le 57 ]; then
				_digit=$((_key - 48))
				[ "$_digit" -le "$_count" ] && MENU_PICK="$_digit"
			fi
			menu_rewind "$MENU_HEIGHT"
		done
		_i=1
		for _item in "$@"; do
			[ "$_i" = "$MENU_PICK" ] && MENU_ANSWER="$_item"
			_i=$((_i + 1))
		done
		menu_done
		menu_leave
	fi

	_i=1
	for _item in "$@"; do
		[ "$_i" = "$MENU_PICK" ] && printf '%s' "$_item"
		_i=$((_i + 1))
	done
}

# choose_yes <prompt> <yes|no> prints yes or no, with that option preselected.
choose_yes() {
	_default=2
	[ "$2" = "yes" ] && _default=1
	menu_select "$1" "$_default" "Yes" "No" >/dev/null || { printf 'no'; return 0; }
	[ "$MENU_PICK" = 1 ] && printf 'yes' || printf 'no'
}

confirm() {
	# confirm <prompt>; true when the answer is yes
	[ "$INTERACTIVE" = 1 ] || return 1
	[ "$(choose_yes "$1" no)" = "yes" ]
}

# ------------------------------------------------------------- detection ----

OS=""
ARCH=""
detect_platform() {
	kernel="$(uname -s 2>/dev/null || echo unknown)"
	machine="$(uname -m 2>/dev/null || echo unknown)"
	case "$kernel" in
	Darwin) OS="darwin" ;;
	Linux) OS="linux" ;;
	*) die "unsupported system: $kernel" \
		"Relo ships for macOS and Linux. On Windows, run install.ps1 in PowerShell." ;;
	esac
	case "$machine" in
	x86_64 | amd64) ARCH="amd64" ;;
	arm64 | aarch64) ARCH="arm64" ;;
	*) die "unsupported architecture: $machine" ;;
	esac
}

resolve_paths() {
	if [ -n "$RELO_HOME_OPT" ]; then
		RELO_HOME_DIR="$RELO_HOME_OPT"
	else
		RELO_HOME_DIR="$HOME/.relo"
	fi
	if [ -z "$INSTALL_DIR" ]; then
		INSTALL_DIR="$HOME/.local/bin"
	fi
	case "$INSTALL_DIR" in
	"~"*) INSTALL_DIR="$HOME${INSTALL_DIR#\~}" ;;
	esac
	BIN="$INSTALL_DIR/relo"
}

# -------------------------------------------------------- foreign check -----

# One install, one channel. Each match prints its own removal command.
reject_foreign() {
	if have brew && brew list --formula relo >/dev/null 2>&1; then
		die "Relo is already installed by Homebrew." \
			"This installer will not stack a second copy." \
			"Remove it first:  brew uninstall relo"
	fi

	if have rpm && rpm -q relo >/dev/null 2>&1; then
		die "Relo is already installed by an .rpm package." \
			"Remove it first:  sudo dnf remove relo"
	fi

	if have dpkg && dpkg -s relo >/dev/null 2>&1; then
		die "Relo is already installed by a .deb package." \
			"Remove it first:  sudo apt remove relo"
	fi

	if have npm; then
		npm_root="$(npm root -g 2>/dev/null || true)"
		if [ -n "$npm_root" ] && [ -d "$npm_root/@jonaskahn/relo" ]; then
			die "Relo is already installed by npm." \
				"Remove it first:  npm uninstall -g @jonaskahn/relo"
		fi
	fi

	for app in "/Applications/Relo.app" "$HOME/Applications/Relo.app"; do
		if [ -d "$app" ]; then
			die "Relo.app is already installed." \
				"Remove it first:  rm -rf \"$app\""
		fi
	done

	found="$(command -v relo 2>/dev/null || true)"
	if [ -n "$found" ] && [ -f "$INSTALL_DIR/$RECEIPT" ]; then
		return 0
	fi
	if [ -n "$found" ]; then
		here="$(cd "$(dirname "$found")" && pwd -P)"
		mine="$(cd "$INSTALL_DIR" 2>/dev/null && pwd -P || printf '%s' "$INSTALL_DIR")"
		if [ "$here" != "$mine" ]; then
			die "Another relo is already on PATH: $here/relo" \
				"That one carries no $RECEIPT marker, so it came from another channel." \
				"Remove it, then run this installer again."
		fi
	fi
}

# --------------------------------------------------------------- download ---

TMP_DIR=""
cleanup() {
	[ -n "$TMP_DIR" ] && rm -rf "$TMP_DIR"
	return 0
}
trap cleanup EXIT INT TERM

fetch_file() {
	# fetch_file <url> <dest>
	if have curl; then
		curl -fsSL --retry 2 --connect-timeout 15 -o "$2" "$1"
	else
		wget -q -O "$2" "$1"
	fi
}

fetch_file_headers() {
	# fetch_file_headers <url> <dest> <headers>; the GET response carries the
	# length the HEAD request may omit.
	if have curl; then
		curl -fsSL --retry 2 --connect-timeout 15 -D "$3" -o "$2" "$1"
	else
		wget -q --server-response -O "$2" "$1" 2>"$3"
	fi
}

content_length() {
	# The last Content-Length wins: a redirect chain carries one per hop.
	if have curl; then
		curl -fsIL --connect-timeout 10 "$1" 2>/dev/null |
			awk 'tolower($1) == "content-length:" { n = $2 } END { sub(/\r$/, "", n); if (n ~ /^[0-9]+$/) print n }'
	else
		wget --server-response --spider "$1" 2>&1 |
			awk 'tolower($1) == "content-length:" { n = $2 } END { sub(/\r$/, "", n); if (n ~ /^[0-9]+$/) print n }'
	fi
}

bar() {
	# bar <percent>; a meter of BAR_WIDTH full blocks
	filled=$(( $1 * BAR_WIDTH / 100 ))
	full=""
	empty=""
	i=0
	while [ "$i" -lt "$BAR_WIDTH" ]; do
		if [ "$i" -lt "$filled" ]; then full="${full}█"; else empty="${empty}░"; fi
		i=$((i + 1))
	done
	printf '%s%s%s%s' "$BRIGHT" "$full" "$MID" "$empty"
}

fetch_with_progress() {
	# fetch_with_progress <url> <dest> <label>
	url="$1"; dest="$2"; label="$3"
	if [ "$UTF8" = 0 ]; then
		step "Downloading $label"
		fetch_file "$url" "$dest"
		done_ "Downloaded $label"
		return 0
	fi
	total="$(content_length "$url" || true)"
	case "$total" in
	'' | *[!0-9]*) total=0;; # unknown length: still show bytes and rate
	esac

	step "Downloading $label"
	hdr="$TMP_DIR/.headers.$$"
	( fetch_file_headers "$url" "$dest" "$hdr" ) &
	pid=$!
	started="$(date +%s)"
	while kill -0 "$pid" 2>/dev/null; do
		now=0
		if [ -f "$dest" ]; then
			now="$(wc -c <"$dest" | tr -d ' ')"
			case "$now" in '' | *[!0-9]*) now=0;; esac
		fi
		if [ "$total" -lt 1 ] && [ -f "$hdr" ]; then
			total="$(awk 'tolower($1) == "content-length:" { n = $2 } END { sub(/\r$/, "", n); if (n ~ /^[0-9]+$/) print n }' "$hdr")"
			case "$total" in '' | *[!0-9]*) total=0;; esac
		fi
		elapsed=$(( $(date +%s) - started ))
		[ "$elapsed" -gt 0 ] || elapsed=1
		rate=$((now / 1024 / elapsed))
		if [ "$total" -gt 0 ]; then
			percent=$((now * 100 / total))
			[ "$percent" -gt 100 ] && percent=100
			eta=""
			if [ "$rate" -gt 0 ] && [ "$now" -lt "$total" ]; then
				eta="  ETA $(((total - now) / (rate * 1024)))s"
			fi
			line="  $(chip DOWNLOAD) [$(bar "$percent")] ${percent}%  $((now / 1048576))/$((total / 1048576)) MB  ${rate} KB/s${eta}"
		else
			line="  $(chip DOWNLOAD) $((now / 1048576)) MB  ${rate} KB/s"
		fi
		printf '\r%s%s' "${ESC}[2K" "$(framed "$line")"
		sleep 0.2 2>/dev/null || sleep 1
	done

	if wait "$pid"; then
		size="$(wc -c <"$dest" | tr -d ' ')"
		done_ "Downloaded $label ($((size / 1048576)) MB)"
	else
		printf '\n'
		die "download failed: $url"
	fi
}

verify_sha() {
	# verify_sha <file> <checksums> <release name>
	file="$1"; sums="$2"; name="$3"
	want="$(awk -v n="$name" '{ f = $2; sub(/^\*/, "", f) } f == n { print $1 }' "$sums" | head -n 1)"
	if [ -z "$want" ]; then
		note "no checksum published for $name; skipping verification"
		return 0
	fi
	if have shasum; then
		got="$(shasum -a 256 "$file" | awk '{ print $1 }')"
	elif have sha256sum; then
		got="$(sha256sum "$file" | awk '{ print $1 }')"
	elif have openssl; then
		got="$(openssl dgst -sha256 "$file" | awk '{ print $NF }')"
	else
		note "no sha256 tool found; skipping verification"
		return 0
	fi
	if [ "$got" != "$want" ]; then
		die "checksum mismatch for $name" \
			"expected  $want" \
			"     got  $got" \
			"The download was corrupted or tampered with. Nothing was installed."
	fi
	done_ "Checksum verified"
}

select_bundle() {
	# select_bundle <checksums> <os> <arch>; prints "<kind> <name>"
	sums="$1"; os="$2"; arch="$3"
	raw="relo-$os-$arch"
	if awk -v n="$raw" '{ f = $2; sub(/^\*/, "", f) } f == n { found = 1; exit 0 } END { exit !found }' "$sums"; then
		printf 'raw %s' "$raw"
		return 0
	fi
	name="$(awk -v os="$os" -v arch="$arch" '{ f = $2; sub(/^\*/, "", f) } f ~ ("^relo-.+-" os "-" arch "\\.tar\\.gz$") { print f; exit }' "$sums")"
	if [ -n "$name" ]; then
		printf 'tarball %s' "$name"
		return 0
	fi
	return 1
}

# -------------------------------------------------------------- the ports ---

PORTS=""

read_ports() {
	cfg="$RELO_HOME_DIR/config.toml"
	if [ -f "$cfg" ]; then
		found="$(awk '
			/^[ \t]*\[/ {
				s = $0
				sub(/^[ \t]*\[/, "", s); sub(/\].*$/, "", s)
				gsub(/[ \t]/, "", s)
				next
			}
			/^[ \t]*[A-Za-z_]+[ \t]*=/ {
				line = $0
				sub(/#.*/, "", line)
				if (split(line, kv, "=") < 2) next
				k = kv[1]; v = kv[2]
				gsub(/[ \t"]/, "", k); gsub(/[ \t"]/, "", v)
				if (k == "") next
				if (s == "server" && k == "port") print v
			}' "$cfg" | awk '!seen[$0]++')"
		if [ -n "$found" ]; then
			PORTS="$found"
			note "Ports read from $cfg"
			return 0
		fi
	fi
	PORTS="$DEFAULT_PORT"
	note "No config.toml yet; using the default port"
}

port_holders() {
	# port_holders <port>; "pid name" per line
	if have lsof; then
		for pid in $(lsof -nP -iTCP:"$1" -sTCP:LISTEN -t 2>/dev/null || true); do
			printf '%s %s\n' "$pid" "$(ps -p "$pid" -o comm= 2>/dev/null | sed 's#.*/##')"
		done
	elif have ss; then
		ss -ltnp 2>/dev/null | awk -v p=":$1" '$4 ~ p {
			if (match($0, /pid=[0-9]+/)) print substr($0, RSTART + 4, RLENGTH - 4), "unknown"
		}'
	elif have fuser; then
		for pid in $(fuser -n tcp "$1" 2>/dev/null || true); do
			printf '%s %s\n' "$pid" "$(ps -p "$pid" -o comm= 2>/dev/null | sed 's#.*/##')"
		done
	fi
}

kill_port() {
	# kill_port <pid> [signal]
	pid="$1"
	sig="${2:-TERM}"
	[ -n "$pid" ] || return 0
	kill -"$sig" "$pid" 2>/dev/null || true
}

free_ports() {
	lead "Checking the ports Relo serves"
	FOREIGN_PORTS=""

	for port in $PORTS; do
		[ "$port" != "0" ] || continue
		holders="$(port_holders "$port")"
		if [ -z "$holders" ]; then
			row "  ${MID}·${OFF} $port available"
			continue
		fi
		while IFS=' ' read -r pid name; do
			[ -n "$pid" ] || continue
			case "$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')" in
			relo | relo.exe | relo-desktop | relo-desktop.exe | relolauncher)
				row "  ${BRIGHT}✓${OFF} $port stopping $name (pid $pid)"
				kill_port "$pid"
				;;
			*)
				row "  ${BRIGHT}!${OFF} $port held by $name (pid $pid)"
				FOREIGN_PORTS="$FOREIGN_PORTS $port"
				;;
			esac
		done <<EOF
$holders
EOF
	done

	if [ -n "$FOREIGN_PORTS" ]; then
		# shellcheck disable=SC2086  # the port list is a list, and splitting is the point
		list="$(printf '%s\n' $FOREIGN_PORTS | awk '!seen[$0]++ { printf " %s", $0 }')"
		if [ "$FORCE_PORTS" = 1 ]; then
			note "Taking ports$list with --force-ports"
		elif [ "$ASSUME_YES" = 1 ]; then
			die "Ports${list} are held by another program" \
				"Nothing was installed." \
				"Rerun with --force-ports to take them, or free them yourself."
		else
			note "Stopping another program may interrupt its work."
			confirm "Stop the program using ports$list?" ||
				die "Ports${list} are held by another program" "Nothing was installed."
		fi
		for port in $FOREIGN_PORTS; do
			while IFS=' ' read -r pid name; do
				[ -n "$pid" ] || continue
				case "$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')" in
				relo | relo.exe | relo-desktop | relo-desktop.exe | relolauncher) ;;
				*) kill_port "$pid" ;;
				esac
			done <<EOF
$(port_holders "$port")
EOF
		done
	fi

	sleep 1 2>/dev/null || true
	for port in $PORTS; do
		[ "$port" != "0" ] || continue
		left="$(port_holders "$port")"
		[ -n "$left" ] || continue
		while IFS=' ' read -r pid name; do
			[ -n "$pid" ] || continue
			kill_port "$pid" KILL
		done <<EOF
$left
EOF
	done
	done_ "Ports ready"
}

stop_daemon() {
	bin="$(command -v relo 2>/dev/null || true)"
	if [ -z "$bin" ] && [ -x "$BIN" ]; then bin="$BIN"; fi
	[ -n "$bin" ] || return 0
	step "Stopping the running daemon"
	if "$bin" daemon stop --force >/dev/null 2>&1; then
		done_ "Daemon stopped"
	else
		note "No daemon was running"
	fi
}

# ------------------------------------------------------------- the daemon ---

start_daemon() {
	if [ "$START_AFTER" = 0 ]; then
		note "Leaving the daemon stopped (--no-start)"
		return 0
	fi

	step "Starting the daemon"
	if "$BIN" daemon start >/dev/null 2>&1; then
		done_ "Daemon started"
	else
		note "The daemon did not start on its own"
	fi

	if [ "$AUTOSTART" = 1 ]; then
		if "$BIN" autostart enable >/dev/null 2>&1; then
			done_ "Autostart enabled"
		else
			note "Autostart was not changed"
		fi
	fi

	server_port="$(printf '%s\n' "$PORTS" | head -n 1)"
	[ -n "$server_port" ] || server_port="$DEFAULT_PORT"
	step "Waiting for the console to respond"
	i=0
	while [ "$i" -lt 20 ]; do
		body="$(curl -fsS --max-time 2 "http://127.0.0.1:$server_port/healthz" 2>/dev/null || true)"
		if [ -n "$body" ]; then
			version="$(printf '%s' "$body" | sed -n 's/.*"version"[ ]*:[ ]*"\([^"]*\)".*/\1/p')"
			CONSOLE_READY=1
			done_ "Console is ready at http://127.0.0.1:$server_port"
			[ -n "$version" ] && note "Reported version: $version"
			return 0
		fi
		i=$((i + 1))
		sleep 0.5 2>/dev/null || sleep 1
	done
	note "The console is not answering yet. Try: relo daemon status"
}

summary() {
	server_port="$(printf '%s\n' "$PORTS" | head -n 1)"
	hline '├' '┤'
	row ""
	row "  ${BRIGHT}✓${OFF} ${BOLD}Relo ${RESOLVED_VERSION} is installed.${OFF}"
	row ""
	row "  ${MID}Binary${OFF}   $BIN"
	row "  ${MID}Data${OFF}     $RELO_HOME_DIR"
	row ""
	if [ "$CONSOLE_READY" = 1 ]; then
		row "  ${BOLD}Your next step${OFF}"
		row "  Open the console and add your first connection:"
	else
		row "  ${BOLD}Ready when you are${OFF}"
		row "  Start Relo, then open the console:"
		row "    \"$BIN\" daemon start"
	fi
	row "  ${BRIGHT}http://127.0.0.1:${server_port:-$DEFAULT_PORT}${OFF}"
	row ""
	row "  ${MID}Useful commands${OFF}"
	row "    relo daemon status       Check how Relo is doing"
	row "    relo daemon restart      Restart Relo"
	row "    relo autostart disable   Turn off start at login"
	row ""
	note "To update, run this installer again."
}

# -------------------------------------------------------------- the prompt --

options_screen() {
	section "Make it yours"
	say "  A few choices, then the installer runs."
	say "  ${DIM}Relo home: $RELO_HOME_DIR${OFF}"
	say ""

	_default=1
	[ "$CHANNEL" = "beta" ] && _default=2
	[ -n "$VERSION" ] && _default=3
	menu_select "Which release?" "$_default" "Latest" "Beta" "Version…" >/dev/null || {
		note "Setup canceled. Nothing was changed."; exit 0
	}
	case "$MENU_PICK" in
	1) CHANNEL="stable"; VERSION="" ;;
	2) CHANNEL="beta"; VERSION="" ;;
	3)
		note "Enter a tag such as v0.1.0. Enter keeps $([ -n "$VERSION" ] && printf '%s' "$VERSION" || printf 'the latest stable')."
		VERSION="$(ask "Release tag" "$VERSION")"
		[ -n "$VERSION" ] && CHANNEL="stable"
		;;
	esac

	_folder_label="$INSTALL_DIR (default)"
	[ -n "${RELO_INSTALL_DIR:-}" ] && _folder_label="$INSTALL_DIR"
	menu_select "Where should the relo binary go?" 1 "$_folder_label" "Custom folder…" >/dev/null || {
		note "Setup canceled. Nothing was changed."; exit 0
	}
	if [ "$MENU_PICK" = 2 ]; then
		note "Enter a folder path. Enter keeps $INSTALL_DIR."
		_new_dir="$(ask "Install folder" "$INSTALL_DIR")"
		[ "$_new_dir" = "$INSTALL_DIR" ] || INSTALL_DIR="$_new_dir"
		resolve_paths
	fi

	if [ "$START_SET" = 1 ]; then
		note "Start after install: $([ "$START_AFTER" = 1 ] && printf 'yes' || printf 'no')"
	elif [ "$(choose_yes "Start Relo after installing?" yes)" = "yes" ]; then
		START_AFTER=1
	else
		START_AFTER=0
	fi
	if [ "$AUTOSTART_SET" = 1 ]; then
		note "Start at login: $([ "$AUTOSTART" = 1 ] && printf 'yes' || printf 'no')"
	elif [ "$START_AFTER" = 0 ]; then
		AUTOSTART=0
	elif [ "$(choose_yes "Start Relo at login?" yes)" = "yes" ]; then
		AUTOSTART=1
	else
		AUTOSTART=0
	fi

	menu_select "Ready to install?" 1 "Install" "Cancel" >/dev/null || {
		note "Setup canceled. Nothing was changed."; exit 0
	}
	if [ "$MENU_PICK" = 2 ]; then
		note "Setup canceled. Nothing was changed."
		exit 0
	fi
	resolve_paths
}

# ---------------------------------------------------------------- install ---

resolve_version() {
	if [ "$CHANNEL" = "beta" ] && [ -z "$VERSION" ]; then
		VERSION="$(latest_beta)"
		[ -n "$VERSION" ] || die "No beta release found" \
			"Every merge to main publishes one; the feed may be between builds." \
			"Rerun with --version <tag> to pin a build."
	fi
	if [ -n "$VERSION" ]; then
		case "$VERSION" in
		v*) tag="$VERSION" ;;
		*) tag="v$VERSION" ;;
		esac
		RESOLVED_VERSION="${tag#v}"
		BASE="$RELEASES/download/$tag"
		return 0
	fi
	BASE="$RELEASES/latest/download"
	RESOLVED_VERSION="latest"
	if have curl; then
		effective="$(curl -fsSIL -o /dev/null -w '%{url_effective}' "$RELEASES/latest" 2>/dev/null || true)"
		tag_seen="$(printf '%s' "$effective" | sed 's#/*$##; s#.*/##; s/^v//')"
		[ -n "$tag_seen" ] && RESOLVED_VERSION="$tag_seen"
	fi
}

# latest_beta reads the newest prerelease tag from the releases feed, the
# same feed the daemon follows for beta updates.
latest_beta() {
	feed="${RELEASES%/releases}/releases.atom"
	if have curl; then
		curl -fsSL --connect-timeout 10 "$feed" 2>/dev/null |
			sed -n 's#.*/releases/tag/\([^"<>/]*\)["<>].*#\1#p' |
			awk -F- 'NF > 1 { print; exit }'
	elif have wget; then
		wget -q -O - --timeout=10 "$feed" 2>/dev/null |
			sed -n 's#.*/releases/tag/\([^"<>/]*\)["<>].*#\1#p' |
			awk -F- 'NF > 1 { print; exit }'
	fi
}

main() {
	detect_platform
	resolve_paths
	VERSION_LINE="${VERSION:-latest release}"
	screen_open
	if [ "$INTERACTIVE" = 1 ]; then
		options_screen
	else
		section "Use your selected settings"
		note "Running without prompts. Use --help to see setup options."
	fi

	reject_foreign
	done_ "No other Relo installation is in the way"

	section "Download Relo"
	resolve_version
	note "Release ${RESOLVED_VERSION}"

	TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/relo-install.XXXXXX")"
	RAW="relo-$OS-$ARCH"

	# One manifest lists every asset in the release. A release carrying the
	# raw binary uses it; one carrying only the versioned tarballs falls
	# back to this host's archive, whose version is read out of the entry
	# itself (same rule as the npm installer).
	if ! fetch_file "$BASE/checksums.txt" "$TMP_DIR/checksums.txt"; then
		die "no checksums.txt in the release" \
			"No release may be published yet; rerun with --version <tag> for a published tag."
	fi
	if ! picked="$(select_bundle "$TMP_DIR/checksums.txt" "$OS" "$ARCH")"; then
		die "no $RAW or versioned tarball in the release" \
			"No release may be published yet; rerun with --version <tag> for a published tag."
	fi
	kind="${picked%% *}"; ASSET="${picked#* }"
	if [ "$kind" = "tarball" ]; then
		RESOLVED_VERSION="${ASSET#relo-}"
		RESOLVED_VERSION="${RESOLVED_VERSION%-$OS-$ARCH.tar.gz}"
	fi

	if [ "$kind" = "raw" ]; then
		fetch_with_progress "$BASE/$ASSET" "$TMP_DIR/relo" "$ASSET"
		verify_sha "$TMP_DIR/relo" "$TMP_DIR/checksums.txt" "$ASSET"
	else
		have tar || die "tar is required to install Relo" \
			"Install tar, then run this installer again."
		fetch_with_progress "$BASE/$ASSET" "$TMP_DIR/bundle.tar.gz" "$ASSET"
		verify_sha "$TMP_DIR/bundle.tar.gz" "$TMP_DIR/checksums.txt" "$ASSET"
		lead "Extracting $ASSET"
		tar -xzf "$TMP_DIR/bundle.tar.gz" -C "$TMP_DIR" || die "could not unpack $ASSET"
		done_ "Extracted $ASSET"
		[ -f "$TMP_DIR/relo" ] || die "the release archive holds no relo binary"
	fi

	section "Install Relo"
	stop_daemon
	read_ports
	free_ports

	step "Installing to $INSTALL_DIR"
	mkdir -p "$INSTALL_DIR"
	chmod 0755 "$TMP_DIR/relo"
	mv -f "$TMP_DIR/relo" "$INSTALL_DIR/relo.new.$$"
	mv -f "$INSTALL_DIR/relo.new.$$" "$BIN"
	if [ "$OS" = "darwin" ] && have xattr; then
		xattr -d com.apple.quarantine "$BIN" 2>/dev/null || true
	fi
	installed="$("$BIN" version 2>/dev/null | head -n 1 || true)"
	done_ "Installed ${installed:-relo} at $BIN"

	{
		printf 'channel=script\n'
		printf 'version=%s\n' "$RESOLVED_VERSION"
		printf 'os=%s\n' "$OS"
		printf 'arch=%s\n' "$ARCH"
		printf 'installed=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || printf 'unknown')"
	} >"$INSTALL_DIR/$RECEIPT"

	case ":$PATH:" in
	*":$INSTALL_DIR:"*) ;;
	*)
		note "$INSTALL_DIR is not on PATH. Add it:"
		say "      ${DIM}export PATH=\"$INSTALL_DIR:\$PATH\"${OFF}"
		;;
	esac

	section "Get you ready"
	start_daemon
	summary
	screen_close
}

main "$@"
