#!/bin/sh
# Relo installer: the daemon, from the latest release, on macOS and Linux.
#
#   curl -fsSL https://relo.ifelse.one/install.sh | sh
#
# The script installs exactly one binary. An installation that belongs to
# another channel -- Homebrew, npm, the macOS app, a .deb or a .rpm -- is
# refused instead of stacked, because two Relos fight over the same state
# directory and the same ports.
set -eu

REPO="jonaskahn/relo"
RELEASES="${RELO_RELEASE_BASE:-https://github.com/$REPO/releases}"
DEFAULT_PORT=10101
DEFAULT_OPENAI=10201
DEFAULT_ANTHROPIC=10202
DEFAULT_GEMINI=10203
RECEIPT=".relo-install"
BAR_WIDTH=20
TERMINAL=0
[ ! -t 1 ] || TERMINAL=1
CONSOLE_READY=0

# ---------------------------------------------------------------- output ----

if [ -n "${NO_COLOR:-}" ] || [ ! -t 1 ]; then
	COLOR=0
else
	COLOR=1
fi

if [ "$COLOR" = 1 ]; then
	BOLD="$(printf '\033[1m')"
	DIM="$(printf '\033[2m')"
	OFF="$(printf '\033[0m')"
	RED="$(printf '\033[38;2;200;16;46m')"
	ROSE="$(printf '\033[38;2;240;90;110m')"
	GREEN="$(printf '\033[38;2;80;200;140m')"
	AMBER="$(printf '\033[38;2;230;170;60m')"
	SLATE="$(printf '\033[38;2;140;140;155m')"
else
	BOLD=""; DIM=""; OFF=""; RED=""; ROSE=""; GREEN=""; AMBER=""; SLATE=""
fi

say() { printf '%s\n' "$*"; }
rule() { say "  ${DIM}----------------------------------------${OFF}"; }
step() { printf '  %s>%s %s\n' "$RED" "$OFF" "$*"; }
done_() { printf '  %s+%s %s\n' "$GREEN" "$OFF" "$*"; }
note() { printf '  %si%s %s\n' "$SLATE" "$OFF" "$*"; }
oops() { printf '  %s!%s %s\n' "$RED" "$OFF" "$*"; }
section() {
	say ""
	rule
	say "  ${RED}$1 / 04${OFF}  ${BOLD}$2${OFF}"
	say ""
}

die() {
	oops "$1" >&2
	shift
	for line in "$@"; do printf '    %s\n' "$line" >&2; done
	exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

banner() {
	say ""
	if [ "$TERMINAL" = 1 ]; then
		say "  ${RED}██████  ${OFF}${BOLD}███████ ██       ██████${OFF}"
		say "  ${RED}██   ██ ${OFF}${BOLD}██      ██      ██    ██${OFF}"
		say "  ${RED}██████  ${OFF}${BOLD}█████   ██      ██    ██${OFF}"
		say "  ${RED}██  ██  ${OFF}${BOLD}██      ██      ██    ██${OFF}"
		say "  ${RED}██   ██ ${OFF}${BOLD}███████ ███████  ██████${OFF}"
	else
		say "  RELO"
	fi
	say ""
	say "  ${BOLD}One daemon. Your tools. Every provider.${OFF}"
	say "  ${DIM}Let's get Relo ready on ${OS}/${ARCH}.${OFF}"
	say ""
}

# --------------------------------------------------------------- options ----

VERSION="${RELO_VERSION:-}"
INSTALL_DIR="${RELO_INSTALL_DIR:-}"
RELO_HOME_OPT="${RELO_HOME:-}"
START_AFTER=1
AUTOSTART=1
ASSUME_YES="${RELO_INSTALL_YES:-0}"
FORCE_PORTS=0
INTERACTIVE=0

usage() {
	cat <<'EOF'
Relo installer

Usage: install.sh [options]

  --version <tag>   install a pinned release, for example --version v0.1.0
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
	--dir) INSTALL_DIR="${2:?--dir needs a value}"; shift 2 ;;
	--dir=*) INSTALL_DIR="${1#*=}"; shift ;;
	--home) RELO_HOME_OPT="${2:?--home needs a value}"; shift 2 ;;
	--home=*) RELO_HOME_OPT="${1#*=}"; shift ;;
	--no-start) START_AFTER=0; shift ;;
	--no-autostart) AUTOSTART=0; shift ;;
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
	printf '  %s> %s%s ' "$RED" "$1" "$OFF" >/dev/tty
	IFS= read -r answer </dev/tty || die "Setup canceled: the terminal input closed."
	[ -n "$answer" ] || answer="$2"
	printf '%s' "$answer"
}

confirm() {
	# confirm <prompt>; true when the answer is yes
	[ "$INTERACTIVE" = 1 ] || return 1
	while :; do
		answer="$(ask "$1 [y/N]" "n")" || return 1
		case "$answer" in
		y | Y | yes | YES | Yes) return 0 ;;
		n | N | no | NO | No) return 1 ;;
		*) note "Please type y for yes or n for no. Enter means no." >/dev/tty ;;
		esac
	done
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

content_length() {
	# The last Content-Length wins: a redirect chain carries one per hop.
	if have curl; then
		curl -fsIL --connect-timeout 10 "$1" 2>/dev/null |
			awk 'tolower($1) == "content-length:" { n = $2 } END { if (n ~ /^[0-9]+$/) print n }'
	else
		wget --server-response --spider "$1" 2>&1 |
			awk 'tolower($1) == "content-length:" { n = $2 } END { if (n ~ /^[0-9]+$/) print n }'
	fi
}

bar() {
	# bar <percent>; a filled/empty meter of BAR_WIDTH cells
	filled=$(( $1 * BAR_WIDTH / 100 ))
	out=""
	i=0
	while [ "$i" -lt "$BAR_WIDTH" ]; do
		if [ "$i" -lt "$filled" ]; then out="$out#"; else out="$out-"; fi
		i=$((i + 1))
	done
	printf '%s' "$out"
}

fetch_with_progress() {
	# fetch_with_progress <url> <dest> <label>
	url="$1"; dest="$2"; label="$3"
	if [ "$TERMINAL" = 0 ]; then
		step "Downloading $label"
		fetch_file "$url" "$dest"
		done_ "Downloaded $label"
		return 0
	fi
	total="$(content_length "$url" || true)"
	case "$total" in
	'' | *[!0-9]*) total="";; # unknown or non-numeric: fall back to the plain download
	esac
	if [ -z "$total" ] || [ "$total" -le 0 ]; then
		step "Downloading $label"
		fetch_file "$url" "$dest"
		done_ "Downloaded $label"
		return 0
	fi

	step "Downloading $label"
	( fetch_file "$url" "$dest" ) &
	pid=$!
	started="$(date +%s)"
	while kill -0 "$pid" 2>/dev/null; do
		now=0
		if [ -f "$dest" ]; then
			now="$(wc -c <"$dest" | tr -d ' ')"
			case "$now" in '' | *[!0-9]*) now=0;; esac
		fi
		elapsed=$(( $(date +%s) - started ))
		[ "$elapsed" -gt 0 ] || elapsed=1
		percent=$((now * 100 / total))
		[ "$percent" -gt 100 ] && percent=100
		printf '\r    %s[%s]%s %3d%%  %s/%s MB  %s KB/s   ' \
			"$ROSE" "$(bar "$percent")" "$OFF" \
			"$percent" "$((now / 1048576))" "$((total / 1048576))" "$(((now / 1024) / elapsed))"
		sleep 0.2 2>/dev/null || sleep 1
	done

	if wait "$pid"; then
		printf '\r    %s[%s]%s 100%%  %s/%s MB                \n' \
			"$GREEN" "$(bar 100)" "$OFF" \
			"$((total / 1048576))" "$((total / 1048576))"
	else
		printf '\n'
		die "download failed: $url"
	fi
}

verify_sha() {
	# verify_sha <file> <checksums> <release name>
	file="$1"; sums="$2"; name="$3"
	want="$(awk -v n="$name" '$2 == n { print $1 }' "$sums" | head -n 1)"
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
				else if (s == "server.data_plane" && (k == "openai" || k == "anthropic" || k == "gemini")) print v
			}' "$cfg" | awk '!seen[$0]++')"
		if [ -n "$found" ]; then
			PORTS="$found"
			note "Ports read from $cfg"
			return 0
		fi
	fi
	PORTS="$DEFAULT_PORT
$DEFAULT_OPENAI
$DEFAULT_ANTHROPIC
$DEFAULT_GEMINI"
	note "No config.toml yet; using the default ports"
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
	step "Checking the ports Relo serves"
	FOREIGN_PORTS=""

	for port in $PORTS; do
		[ "$port" != "0" ] || continue
		holders="$(port_holders "$port")"
		if [ -z "$holders" ]; then
			printf '  %si%s %-6s available\n' "$SLATE" "$OFF" "$port"
			continue
		fi
		while IFS=' ' read -r pid name; do
			[ -n "$pid" ] || continue
			case "$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')" in
			relo | relo.exe | relo-desktop | relo-desktop.exe | relolauncher)
				printf '  %s+%s %-6s stopping %s (pid %s)\n' "$GREEN" "$OFF" "$port" "$name" "$pid"
				kill_port "$pid"
				;;
			*)
				printf '  %s!%s %-6s held by %s (pid %s)\n' "$AMBER" "$OFF" "$port" "$name" "$pid"
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
	"$BIN" daemon start >/dev/null 2>&1 || note "The daemon did not start on its own"

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
			printf '  %s+%s Console is ready at %shttp://127.0.0.1:%s%s\n' \
				"$GREEN" "$OFF" "$BOLD" "$server_port" "$OFF"
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
	say ""
	rule
	say "  ${GREEN}+${OFF} ${BOLD}Relo ${RESOLVED_VERSION} is installed.${OFF}"
	say ""
	say "  ${DIM}Binary${OFF}   $BIN"
	say "  ${DIM}Data${OFF}     $RELO_HOME_DIR"
	say ""
	if [ "$CONSOLE_READY" = 1 ]; then
		say "  ${BOLD}Your next step${OFF}"
		say "  Open the console and add your first connection:"
	else
		say "  ${BOLD}Ready when you are${OFF}"
		say "  Start Relo, then open the console:"
		say "    \"$BIN\" daemon start"
	fi
	say "  ${RED}http://127.0.0.1:${server_port:-$DEFAULT_PORT}${OFF}"
	say ""
	say "  ${DIM}Useful commands${OFF}"
	say "    relo daemon status       Check how Relo is doing"
	say "    relo daemon restart      Restart Relo"
	say "    relo autostart disable   Turn off start at login"
	say ""
	note "To update, run this installer again."
	rule
	say ""
}

# -------------------------------------------------------------- the prompt --

options_screen() {
	banner
	section "01" "Make it yours"
	say "  You're one Enter away. Want to make a change first?"
	say ""
	while :; do
		if [ "$START_AFTER" = 1 ]; then start_label="${GREEN}[ON ]${OFF}"; else start_label="${SLATE}[OFF]${OFF}"; fi
		if [ "$AUTOSTART" = 1 ]; then auto_label="${GREEN}[ON ]${OFF}"; else auto_label="${SLATE}[OFF]${OFF}"; fi
		[ "$START_AFTER" = 1 ] || auto_label="${AMBER}[ --]${OFF}"
		if [ -n "$VERSION" ]; then version_hint="(pinned release)"; else version_hint="(recommended)"; fi
		say "  ${DIM}INSTALLATION${OFF}"
		say "  ${DIM}----------------------------------------${OFF}"
		say "  ${RED}[1]${OFF} ${BOLD}Release${OFF}"
		say "      ${VERSION:-Latest stable} ${DIM}${version_hint}${OFF}"
		say ""
		say "  ${RED}[2]${OFF} ${BOLD}Install folder${OFF}"
		say "      $INSTALL_DIR"
		say ""
		say "  ${DIM}ON YOUR SCHEDULE${OFF}"
		say "  ${DIM}----------------------------------------${OFF}"
		say "  ${RED}[3]${OFF} ${start_label} ${BOLD}Start after install${OFF}"
		say "      ${DIM}Have the console ready as soon as we're done.${OFF}"
		say ""
		say "  ${RED}[4]${OFF} ${auto_label} ${BOLD}Start at login${OFF}"
		if [ "$START_AFTER" = 1 ]; then
			say "      ${DIM}Keep Relo ready whenever you sign in.${OFF}"
		else
			say "      ${AMBER}Not changed (start after install is off)${OFF}"
		fi
		say ""
		say "  ${RED}${BOLD}>>>  Press Enter to continue installation${OFF}"
		say "  ${DIM}     1-4 to adjust a setting / q to cancel${OFF}"
		say ""
		choice="$(ask "What would you like to do?" "")"
		say ""
		case "$choice" in
		"") break ;;
		1)
			note "Enter a release such as v0.1.0. Leave blank for latest stable."
			VERSION="$(ask "Which release?" "")"
			;;
		2)
			note "Press Enter to keep $INSTALL_DIR."
			INSTALL_DIR="$(ask "Where should Relo be installed?" "$INSTALL_DIR")"
			resolve_paths
			;;
		3) [ "$START_AFTER" = 1 ] && START_AFTER=0 || START_AFTER=1 ;;
		4) [ "$AUTOSTART" = 1 ] && AUTOSTART=0 || AUTOSTART=1 ;;
		q | Q) note "Setup canceled. Nothing was changed."; exit 0 ;;
		*) note "Choose 1, 2, 3 or 4; Enter to install; q to cancel." ;;
		esac
		say ""
	done
	resolve_paths
}

# ---------------------------------------------------------------- install ---

resolve_version() {
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

main() {
	detect_platform
	resolve_paths
	if [ "$INTERACTIVE" = 1 ]; then
		options_screen
	else
		banner
		section "01" "Use your selected settings"
		note "Running without prompts. Use --help to see setup options."
	fi

	reject_foreign
	done_ "No other Relo installation is in the way"

	section "02" "Download Relo"
	resolve_version
	note "Release ${RESOLVED_VERSION}"

	TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/relo-install.XXXXXX")"
	ASSET="relo-$OS-$ARCH"

	fetch_with_progress "$BASE/$ASSET" "$TMP_DIR/relo" "$ASSET"
	if fetch_file "$BASE/checksums.txt" "$TMP_DIR/checksums.txt" 2>/dev/null; then
		verify_sha "$TMP_DIR/relo" "$TMP_DIR/checksums.txt" "$ASSET"
	else
		note "This release publishes no checksums.txt"
	fi

	section "03" "Install Relo"
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

	section "04" "Get you ready"
	start_daemon
	summary
}

main "$@"
