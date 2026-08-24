#!/bin/sh
# dogfetch.sh - download-on-demand wrapper for the dogfetch binary.
#
# Resolves a version (DOGFETCH_VERSION env > pin file > latest GitHub
# release, cached 24h), downloads the goreleaser archive for this
# platform, verifies its sha256 against the release checksums.txt,
# caches the binary under ~/.cache/dogfetch/<version>/, and execs it
# with all arguments. `--self-update` re-pins to the latest release.
#
# POSIX sh; needs curl or wget, tar, and sha256sum or shasum.

set -u

REPO="jtzemp/dogfetch"
CACHE_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/dogfetch"
PIN_FILE="$CACHE_DIR/pin"
LATEST_CACHE="$CACHE_DIR/latest-tag"
LATEST_TTL_MINUTES=1440 # 24h; anonymous GitHub API allows 60 req/h

# fail prints an AXI-shaped error block on stdout and exits 1, so an
# agent reading the output knows what broke and what to do next.
# Only called from the top-level shell (never inside $(...)), so the
# block reaches the real stdout.
fail() {
    _msg="$1"
    shift
    printf 'error: %s (wrapper_error)\n' "$_msg"
    if [ "$#" -gt 0 ]; then
        printf 'help[%d]:\n' "$#"
        for _line in "$@"; do
            printf '  %s\n' "$_line"
        done
    fi
    exit 1
}

# --- prerequisite checks (top level, so fail() output is visible) ---

if command -v curl >/dev/null 2>&1; then
    FETCH="curl"
elif command -v wget >/dev/null 2>&1; then
    FETCH="wget"
else
    fail "neither curl nor wget is available" \
        "Install curl (or wget) and rerun"
fi

if command -v sha256sum >/dev/null 2>&1; then
    SHATOOL="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
    SHATOOL="shasum"
else
    fail "neither sha256sum nor shasum is available" \
        "Install coreutils (sha256sum) or perl (shasum) and rerun"
fi

command -v tar >/dev/null 2>&1 || fail "tar is not available" \
    "Install tar and rerun"

# --- helpers ---

http_get() {
    # http_get URL [OUTFILE]; prints to stdout when no OUTFILE.
    if [ "$FETCH" = "curl" ]; then
        if [ "$#" -ge 2 ]; then
            curl -fsSL --retry 2 -o "$2" "$1"
        else
            curl -fsSL --retry 2 "$1"
        fi
    else
        if [ "$#" -ge 2 ]; then
            wget -qO "$2" "$1"
        else
            wget -qO- "$1"
        fi
    fi
}

sha256_of() {
    if [ "$SHATOOL" = "sha256sum" ]; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

# set_platform_suffix sets SUFFIX (e.g. Linux_x86_64) or fails.
set_platform_suffix() {
    _os=$(uname -s)
    _arch=$(uname -m)
    case "$_os" in
        Linux) _os_name="Linux" ;;
        Darwin) _os_name="Darwin" ;;
        *) fail "unsupported OS: $_os" \
            "dogfetch ships Linux and macOS binaries; build from source: go install github.com/$REPO@latest" ;;
    esac
    case "$_arch" in
        x86_64 | amd64) _arch_name="x86_64" ;;
        arm64 | aarch64) _arch_name="arm64" ;;
        *) fail "unsupported architecture: $_arch" \
            "dogfetch ships x86_64 and arm64 binaries; build from source: go install github.com/$REPO@latest" ;;
    esac
    SUFFIX="${_os_name}_${_arch_name}"
}

# set_latest_tag sets TAG to the newest release tag (vX.Y.Z), using a
# 24h cache. Pass "force" to bypass the cache.
set_latest_tag() {
    if [ "${1:-}" != "force" ] && [ -f "$LATEST_CACHE" ] &&
        find "$LATEST_CACHE" -mmin "-$LATEST_TTL_MINUTES" 2>/dev/null | grep -q .; then
        TAG=$(cat "$LATEST_CACHE")
        [ -n "$TAG" ] && return
    fi
    TAG=$(http_get "https://api.github.com/repos/$REPO/releases/latest" |
        sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
    if [ -z "$TAG" ]; then
        if [ -s "$LATEST_CACHE" ]; then
            TAG=$(cat "$LATEST_CACHE") # stale cache beats failing outright
            return
        fi
        fail "could not resolve the latest dogfetch release from GitHub" \
            "Check connectivity to api.github.com" \
            "Or pin a version: DOGFETCH_VERSION=0.2.0 $0 ..."
    fi
    mkdir -p "$CACHE_DIR"
    printf '%s\n' "$TAG" >"$LATEST_CACHE"
}

# set_version sets VERSION (no leading v): env > pin file > latest.
set_version() {
    if [ -n "${DOGFETCH_VERSION:-}" ]; then
        VERSION="${DOGFETCH_VERSION#v}"
        return
    fi
    if [ -s "$PIN_FILE" ]; then
        _pin=$(cat "$PIN_FILE")
        VERSION="${_pin#v}"
        return
    fi
    set_latest_tag
    VERSION="${TAG#v}"
}

# install_version downloads, verifies, and caches $VERSION; sets BIN.
install_version() {
    BIN="$CACHE_DIR/$VERSION/dogfetch"
    if [ -x "$BIN" ]; then
        return
    fi

    set_platform_suffix
    _archive="dogfetch_${VERSION}_${SUFFIX}.tar.gz"
    _base="https://github.com/$REPO/releases/download/v$VERSION"

    _tmp=$(mktemp -d "${TMPDIR:-/tmp}/dogfetch.XXXXXX") || fail "mktemp failed"
    trap 'rm -rf "$_tmp"' EXIT

    http_get "$_base/$_archive" "$_tmp/$_archive" ||
        fail "download failed: $_base/$_archive" \
            "Check that release v$VERSION exists: https://github.com/$REPO/releases" \
            "Check connectivity to github.com"
    http_get "$_base/checksums.txt" "$_tmp/checksums.txt" ||
        fail "download failed: $_base/checksums.txt" \
            "Check connectivity to github.com"

    _want=$(grep " $_archive\$" "$_tmp/checksums.txt" | cut -d' ' -f1 | head -1)
    [ -n "$_want" ] || fail "no checksum entry for $_archive in checksums.txt" \
        "The release may still be publishing; retry in a minute"
    _got=$(sha256_of "$_tmp/$_archive")
    if [ "$_want" != "$_got" ]; then
        fail "sha256 mismatch for $_archive (expected $_want, got $_got)" \
            "Retry; if it persists, the download may be corrupted or tampered with - do not run it"
    fi

    tar -xzf "$_tmp/$_archive" -C "$_tmp" dogfetch ||
        fail "could not extract dogfetch from $_archive"

    mkdir -p "$CACHE_DIR/$VERSION"
    mv "$_tmp/dogfetch" "$BIN.partial" &&
        chmod 0755 "$BIN.partial" &&
        mv "$BIN.partial" "$BIN" ||
        fail "could not install binary to $BIN"
}

# --- main ---

if [ "${1:-}" = "--self-update" ]; then
    set_latest_tag force
    VERSION="${TAG#v}"
    install_version
    mkdir -p "$CACHE_DIR"
    printf '%s\n' "$TAG" >"$PIN_FILE"
    printf 'dogfetch: pinned to %s (%s)\n' "$TAG" "$BIN"
    exit 0
fi

set_version
install_version

# The binary already picks the agent-friendly default itself (toon on
# stdout, ndjson with --output). Exporting DOGFETCH_FORMAT here would
# pin the format flag and defeat the --output half of that rule.
exec "$BIN" "$@"
