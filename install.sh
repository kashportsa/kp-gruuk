#!/bin/sh
set -e

# KP-Gruuk installer / updater
# Usage: curl -sSL https://raw.githubusercontent.com/kashportsa/kp-gruuk/main/install.sh | sh
#
# For private repos: set GITHUB_TOKEN to a PAT with 'repo' read scope,
# or install the GitHub CLI (gh) and run 'gh auth login' — it will be used automatically.

REPO="kashportsa/kp-gruuk"
INSTALL_DIR="/usr/local/bin"
BINARY="gruuk"

# ── helpers ───────────────────────────────────────────────────────────────────

detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        darwin) OS="darwin" ;;
        linux)  OS="linux" ;;
        *)
            echo "Error: Unsupported operating system: $OS" >&2
            exit 1
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *)
            echo "Error: Unsupported architecture: $ARCH" >&2
            exit 1
            ;;
    esac

    echo "${OS}-${ARCH}"
}

# Fetch a URL, injecting a GitHub token when available.
gh_fetch() {
    URL="$1"
    AUTH_HEADER=""

    if [ -n "$GITHUB_TOKEN" ]; then
        AUTH_HEADER="Authorization: Bearer ${GITHUB_TOKEN}"
    elif command -v gh > /dev/null 2>&1; then
        TOKEN=$(gh auth token 2>/dev/null || true)
        [ -n "$TOKEN" ] && AUTH_HEADER="Authorization: Bearer ${TOKEN}"
    fi

    if command -v curl > /dev/null 2>&1; then
        if [ -n "$AUTH_HEADER" ]; then
            curl -sSL -H "$AUTH_HEADER" "$URL"
        else
            curl -sSL "$URL"
        fi
    elif command -v wget > /dev/null 2>&1; then
        if [ -n "$AUTH_HEADER" ]; then
            wget -qO- --header="$AUTH_HEADER" "$URL"
        else
            wget -qO- "$URL"
        fi
    else
        echo "Error: curl or wget is required" >&2
        exit 1
    fi
}

get_latest_version() {
    RESPONSE=$(gh_fetch "https://api.github.com/repos/${REPO}/releases/latest")
    TAG=$(echo "$RESPONSE" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$TAG" ]; then
        echo "" >&2
        echo "Error: Could not fetch latest release from GitHub." >&2
        if echo "$RESPONSE" | grep -q '"Not Found"'; then
            echo "" >&2
            echo "The repository is private. Authenticate using one of:" >&2
            echo "  1. Set GITHUB_TOKEN to a PAT with 'repo' read scope:" >&2
            echo "       GITHUB_TOKEN=ghp_xxx sh <(curl -sSL ...)" >&2
            echo "  2. Install the GitHub CLI (https://cli.github.com) and run 'gh auth login'" >&2
        fi
        exit 1
    fi

    echo "$TAG"
}

# Returns the currently installed version tag (e.g. "v0.1.0"), or empty string.
get_installed_version() {
    INSTALLED_BIN=$(command -v "$BINARY" 2>/dev/null || true)
    [ -z "$INSTALLED_BIN" ] && echo "" && return
    RAW=$("$INSTALLED_BIN" version 2>/dev/null || true)
    echo "$RAW" | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' | head -1
}

# Returns 0 (true) if $1 is strictly greater than $2 in semver.
is_newer() {
    V1=$(echo "$1" | tr -d 'v')
    V2=$(echo "$2" | tr -d 'v')

    M1=$(echo "$V1" | cut -d. -f1)
    N1=$(echo "$V1" | cut -d. -f2)
    P1=$(echo "$V1" | cut -d. -f3)

    M2=$(echo "$V2" | cut -d. -f1)
    N2=$(echo "$V2" | cut -d. -f2)
    P2=$(echo "$V2" | cut -d. -f3)

    if [ "$M1" -ne "$M2" ]; then [ "$M1" -gt "$M2" ]; return; fi
    if [ "$N1" -ne "$N2" ]; then [ "$N1" -gt "$N2" ]; return; fi
    [ "$P1" -gt "$P2" ]
}

# Download and place the binary. Handles /usr/local/bin (with sudo fallback) and ~/.local/bin.
do_install() {
    VERSION="$1"
    PLATFORM="$2"

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}-${PLATFORM}"

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    gh_fetch "$DOWNLOAD_URL" > "${TMPDIR}/${BINARY}"

    # Sanity-check: verify we got a real binary, not an HTML/JSON error page.
    MAGIC=$(head -c 4 "${TMPDIR}/${BINARY}" | od -An -tx1 | tr -d ' \n')
    if [ "$MAGIC" != "7f454c46" ] && \
       [ "$MAGIC" != "cffaedfe" ] && [ "$MAGIC" != "cefaedfe" ] && \
       [ "$MAGIC" != "feedface" ] && [ "$MAGIC" != "feadfeed" ]; then
        echo "Error: Downloaded file does not appear to be a valid binary." >&2
        echo "Check that your GITHUB_TOKEN has access to this repository." >&2
        exit 1
    fi

    chmod +x "${TMPDIR}/${BINARY}"

    # Try /usr/local/bin directly, then via sudo, then fall back to ~/.local/bin.
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
        echo "Installed to ${INSTALL_DIR}/${BINARY}"
    elif command -v sudo > /dev/null 2>&1 && sudo mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}" 2>/dev/null; then
        echo "Installed to ${INSTALL_DIR}/${BINARY}"
    else
        LOCAL_BIN="$HOME/.local/bin"
        mkdir -p "$LOCAL_BIN"
        mv "${TMPDIR}/${BINARY}" "${LOCAL_BIN}/${BINARY}"
        echo "Installed to ${LOCAL_BIN}/${BINARY}"
        echo ""
        echo "Make sure ${LOCAL_BIN} is in your PATH:"
        echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
    fi
}

# ── main ──────────────────────────────────────────────────────────────────────

PLATFORM=$(detect_platform)
LATEST=$(get_latest_version)
CURRENT=$(get_installed_version)

if [ -z "$CURRENT" ]; then
    echo "Installing gruuk ${LATEST} (${PLATFORM})..."
    do_install "$LATEST" "$PLATFORM"
    echo ""
    echo "Done! Run 'gruuk expose <port>' to get started."

elif [ "$CURRENT" = "$LATEST" ]; then
    echo "gruuk is already up to date (${CURRENT})."

elif is_newer "$LATEST" "$CURRENT"; then
    echo "Updating gruuk ${CURRENT} -> ${LATEST} (${PLATFORM})..."
    do_install "$LATEST" "$PLATFORM"
    echo ""
    echo "Done! gruuk is now ${LATEST}."

else
    # Current version is ahead of latest release (e.g. a local dev build).
    echo "gruuk ${CURRENT} is installed (latest release: ${LATEST}). Nothing to do."
fi
