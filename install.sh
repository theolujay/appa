#!/usr/bin/env bash
set -eu

REPO_OWNER="theolujay"
REPO_NAME="appa"

BOLD="$(tput bold 2>/dev/null || printf '')"
GREY="$(tput setaf 0 2>/dev/null || printf '')"
RED="$(tput setaf 1 2>/dev/null || printf '')"
GREEN="$(tput setaf 2 2>/dev/null || printf '')"
YELLOW="$(tput setaf 3 2>/dev/null || printf '')"
BLUE="$(tput setaf 4 2>/dev/null || printf '')"
NO_COLOR="$(tput sgr0 2>/dev/null || printf '')"

CURL_RETRY_OPTS="--retry 3 --retry-all-errors --retry-delay 2"

info()      { printf '%s\n' "${BOLD}${GREY}>${NO_COLOR} $*"; }
warn()      { printf '%s\n' "${YELLOW}! $*${NO_COLOR}"; }
error()     { printf '%s\n' "${RED}x $*${NO_COLOR}" >&2; }
completed() { printf '%s\n' "${GREEN}✓${NO_COLOR} $*"; }

has() { command -v "$1" >/dev/null 2>&1; }

# --- Spinner ---
_spin_pid=""

_spin_cleanup() {
  [ -z "$_spin_pid" ] && { printf '\r\033[2K'; return; }
  kill "$_spin_pid" 2>/dev/null || true
  wait "$_spin_pid" 2>/dev/null || true
  _spin_pid=""
  printf '\r\033[2K'
  tput cnorm 2>/dev/null || true
}

trap '_spin_cleanup' EXIT
trap '_spin_cleanup; exit 130' INT

spin_start() {
  # When stdout is not a TTY (e.g. piped), fall back to a static info line
  if [ ! -t 1 ]; then
    info "$*"
    return
  fi
  _spin_cleanup
  tput civis 2>/dev/null || true
  local msg="$*"
  (
    frames='⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏'
    i=1
    while true; do
      frame=$(printf '%s' "$frames" | cut -d' ' -f$i)
      printf '\r  %s%s%s  %s' "${BLUE}" "$frame" "${NO_COLOR}" "$msg"
      i=$(( (i % 10) + 1 ))
      sleep 0.08
    done
  ) &
  _spin_pid=$!
}

spin_stop() {
  _spin_cleanup
}
# --------------------------------------------------

test_writeable() {
  mkdir -p "$1" 2>/dev/null
  local path="${1}/.appa-install-test"
  if touch "$path" 2>/dev/null; then
    rm "$path"
    return 0
  fi
  return 1
}

detect_platform() {
  local platform
  platform="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$platform" in
    linux)  printf 'linux'  ;;
    darwin) printf 'darwin' ;;
    *)      error "Unsupported OS: $platform"; exit 1 ;;
  esac
}

detect_arch() {
  local arch
  arch="$(uname -m | tr '[:upper:]' '[:lower:]')"
  case "$arch" in
    amd64|x86_64)  printf 'amd64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *)        error "Unsupported architecture: $arch"; exit 1 ;;
  esac
}

get_tmpfile() {
  local suffix="$1"
  if has mktemp; then
    printf '%s' "$(mktemp).$suffix"
  else
    printf '/tmp/%s.%s' "${REPO_NAME}" "$suffix"
  fi
}

download() {
  local file="$1" url="$2"

  if has curl; then
    curl --fail --silent --location ${CURL_RETRY_OPTS} --output "$file" "$url"
  elif has wget; then
    wget --quiet --output-document="$file" "$url"
  else
    error "curl or wget is required to install ${REPO_NAME}"
    exit 1
  fi
}

check_bin_dir() {
  local bin_dir="$1"
  local good

  good=$(IFS=:; for p in $PATH; do [ "$p" = "$bin_dir" ] && printf 1 && break; done)
  if [ "$good" != "1" ]; then
    warn "Bin directory ${bin_dir} is not in your \$PATH"
    info "Add it: export PATH=\"${bin_dir}:\$PATH\""
  fi
}

# --- Defaults ---
VERSION="${APPA_VERSION:-}"
BIN_DIR="${APPA_INSTALL_DIR:-}"
if [ -z "$BIN_DIR" ]; then
  for dir in "$HOME/.local/bin" "$HOME/bin" "/usr/local/bin"; do
    test_writeable "$dir" && { BIN_DIR="$dir"; break; }
  done
  [ -n "$BIN_DIR" ] || BIN_DIR="/usr/local/bin"
fi
PLATFORM="$(detect_platform)"
ARCH="$(detect_arch)"
FORCE=""
HELP=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --release) VERSION="$2"; shift 2 ;;
    --release=*) VERSION="${1#*=}"; shift ;;
    -b|--bin-dir) BIN_DIR="$2"; shift 2 ;;
    -b=*|--bin-dir=*) BIN_DIR="${1#*=}"; shift ;;
    -f|-y|--force|--yes) FORCE=1; shift ;;
    -V|--verbose) set -x; shift ;;
    -h|--help) HELP=1; shift ;;
    *) error "Unknown option: $1"; echo "See --help for usage" >&2; exit 1 ;;
  esac
done

if [ -n "$HELP" ]; then
  cat <<EOF
Install ${REPO_NAME} — curl -fsSL https://appa.${REPO_OWNER}.dev/install.sh | sh

Options:
  --release VERSION     Install a specific version (default: latest)
  -b, --bin-dir DIR     Install to DIR (default: /usr/local/bin)
  -f, -y, --force       Skip confirmation prompt
  -V, --verbose         Enable verbose output
  -h, --help            Show this help

Environment:
  APPA_VERSION          Version override
  APPA_INSTALL_DIR      Install directory override
EOF
  exit 0
fi

# --- Resolve version ---
if [ -z "$VERSION" ]; then
  spin_start "Fetching latest version..."
  VERSION="$(curl --fail --silent --show-error ${CURL_RETRY_OPTS} \
    "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" \
    | grep -o '"tag_name": "v.*"' | cut -d'"' -f4 | cut -c2-)"
  spin_stop

  if [ -z "$VERSION" ]; then
    error "Failed to detect latest version from GitHub API"
    info "Set APPA_VERSION=x.y.z to specify a version manually"
    exit 1
  fi
fi

BINARY="${REPO_NAME}-${PLATFORM}-${ARCH}"
URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${VERSION}/${BINARY}"
CHECKSUM_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/v${VERSION}/checksums.txt"

# --- Print configuration ---
printf '\n'
info "${BOLD}Bin directory${NO_COLOR}: ${GREEN}${BIN_DIR}${NO_COLOR}"
info "${BOLD}Platform${NO_COLOR}:      ${GREEN}${PLATFORM}${NO_COLOR}"
info "${BOLD}Arch${NO_COLOR}:          ${GREEN}${ARCH}${NO_COLOR}"
info "${BOLD}Version${NO_COLOR}:       ${GREEN}${VERSION}${NO_COLOR}"
printf '\n'

# --- Confirm ---
if [ -t 0 ] && [ -z "${FORCE}" ]; then
  printf '%s' "${YELLOW}?${NO_COLOR} Install ${REPO_NAME} ${GREEN}${VERSION}${NO_COLOR} to ${BOLD}${GREEN}${BIN_DIR}${NO_COLOR}? [y/N] "
  read -r yn </dev/tty || true
  case "$yn" in
    y|Y|yes|YES) ;;
    *) error 'Aborting' >&2; exit 1 ;;
  esac
fi

# --- Determine sudo ---
if test_writeable "$BIN_DIR"; then
  SUDO=""
else
  if ! has sudo; then
    error "sudo is required to install to ${BIN_DIR}"
    info "Set APPA_INSTALL_DIR=\$HOME/.local/bin (or another writable directory)"
    exit 1
  fi
  warn "Elevated permissions required to install to ${BIN_DIR}"
  SUDO="sudo"
fi

check_bin_dir "$BIN_DIR"
printf '\n'

# --- Download ---
archive="$(get_tmpfile "$BINARY")"
checksums="$(get_tmpfile "checksums.txt")"

spin_start "Downloading ${REPO_NAME} v${VERSION} (${PLATFORM}/${ARCH})..."
download "$archive" "$URL"
spin_stop

spin_start "Downloading checksums..."
download "$checksums" "$CHECKSUM_URL"
spin_stop

# --- Verify ---
spin_start "Verifying checksum..."
expected="$(grep -m1 -w "$BINARY" "$checksums" | cut -d' ' -f1 || true)"
actual=""
verified=0

if [ -n "$expected" ] && has sha256sum; then
  actual="$(sha256sum "$archive" | cut -d' ' -f1)"
  verified=1
elif [ -n "$expected" ] && has shasum; then
  actual="$(shasum --algorithm 256 "$archive" | cut -d' ' -f1)"
  verified=1
fi
spin_stop

if [ "$verified" = "0" ]; then
  warn "No SHA-256 tool found; skipping checksum verification"
elif [ "$actual" != "$expected" ]; then
  error "Checksum mismatch for $BINARY"
  info "Expected: $expected"
  info "Actual:   $actual"
  exit 1
fi

chmod 755 "$archive"

# --- Install ---
spin_start "Installing to ${BIN_DIR}/${REPO_NAME}..."
${SUDO} mkdir -p "$BIN_DIR"
${SUDO} cp "$archive" "${BIN_DIR}/${REPO_NAME}"
spin_stop

# --- Cleanup ---
rm -f "$archive" "$checksums"

# --- Done ---
printf '\n'
printf '%s' "${BLUE}"
cat << 'EOF'

      _|_|
    _|    _|  _|_|_|    _|_|_|      _|_|_|
    _|_|_|_|  _|    _|  _|    _|  _|    _|
    _|    _|  _|    _|  _|    _|  _|    _|
    _|    _|  _|_|_|    _|_|_|      _|_|_|
              _|        _|
              _|        _|

EOF
printf '%s' "${NO_COLOR}"
completed "${REPO_NAME} ${GREEN}${VERSION}${NO_COLOR} installed to ${BOLD}${GREEN}${BIN_DIR}/${REPO_NAME}${NO_COLOR}"
completed "Run ${BOLD}${REPO_NAME} --help${NO_COLOR} to get started"