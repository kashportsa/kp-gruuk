#!/bin/sh
set -e

# KP-Gruuk installer
# Usage: curl -sSL https://raw.githubusercontent.com/kashportsa/kp-gruuk/main/install.sh | sh
#
# Private repo: set GITHUB_TOKEN to a personal access token with 'repo' read scope,
# or install the GitHub CLI (gh) — it will be used automatically if available.

REPO="kashportsa/kp-gruuk"
INSTALL_DIR="/usr/local/bin"
BINARY="gruuk"

# ── helpers ──────────────────────────────────────────────────────────────────

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
    if [ -n "$GITHUB_TOKEN" ]; then
        AUTH_HEADER="Authorization: Bearer ${GITHUB_TOKEN}"
    elif command -v gh > /dev/null 2>&1; then
        TOKEN=$(gh auth token 2>/dev/null || true)
        AUTH_HEADER="${TOKEN:+Authorization: Bearer ${TOKEN}}"
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
        echo "" >&2
        # Detect the specific private-repo auth failure
        if echo "$RESPONSE" | grep -q '"Not Found"'; then
            echo "The repository is private. Authenticate using one of:" >&2
            echo "  1. Set GITHUB_TOKEN to a PAT with 'repo' read scope:" >&2
            echo "       GITHUB_TOKEN=ghp_xxx sh <(curl -sSL ...)" >&2
            echo "  2. Install the GitHub CLI (https://cli.github.com) and run 'gh auth login'" >&2
        fi
        exit 1
    fi

    echo "$TAG"
}

install_binary() {
    PLATFORM=$(detect_platform)
    VERSION=$(get_latest_version)

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}-${PLATFORM}"

    echo "Installing gruuk ${VERSION} (${PLATFORM})..."

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    gh_fetch "$DOWNLOAD_URL" > "${TMPDIR}/${BINARY}"

    # Sanity-check: make sure we got a binary, not an HTML/JSON error page
    MAGIC=$(head -c 4 "${TMPDIR}/${BINARY}" | od -An -tx1 | tr -d ' \n')
    if [ "$MAGIC" != "7f454c46" ] && [ "$MAGIC" != "cffaedfe" ] && [ "$MAGIC" != "cefaedfe" ] && [ "$MAGIC" != "feedface" ] && [ "$MAGIC" != "feadfeed" ]; then
        echo "Error: Downloaded file does not appear to be a valid binary." >&2
        echo "Check that your GITHUB_TOKEN has access to this repository." >&2
        exit 1
    fi

    chmod +x "${TMPDIR}/${BINARY}"

    # Try /usr/local/bin (directly, then via sudo), fall back to ~/.local/bin
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

    echo ""
    echo "Done! Run 'gruuk expose <port>' to get started."
}

install_binary
