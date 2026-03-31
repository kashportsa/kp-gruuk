#!/bin/sh
set -e

# KP-Gruuk installer
# Usage: curl -sSL https://raw.githubusercontent.com/kashportsa/kp-gruuk/main/install.sh | sh

REPO="kashportsa/kp-gruuk"
INSTALL_DIR="/usr/local/bin"
BINARY="gruuk"

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        darwin) OS="darwin" ;;
        linux)  OS="linux" ;;
        *)
            echo "Error: Unsupported operating system: $OS"
            exit 1
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64) ARCH="amd64" ;;
        arm64|aarch64) ARCH="arm64" ;;
        *)
            echo "Error: Unsupported architecture: $ARCH"
            exit 1
            ;;
    esac

    echo "${OS}-${ARCH}"
}

# Get the latest release tag from GitHub
get_latest_version() {
    if command -v curl > /dev/null 2>&1; then
        curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    elif command -v wget > /dev/null 2>&1; then
        wget -qO- "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/'
    else
        echo "Error: curl or wget required"
        exit 1
    fi
}

# Download and install the binary
install() {
    PLATFORM=$(detect_platform)
    VERSION=$(get_latest_version)

    if [ -z "$VERSION" ]; then
        echo "Error: Could not determine latest version"
        exit 1
    fi

    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}-${PLATFORM}"

    echo "Installing gruuk ${VERSION} (${PLATFORM})..."

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    if command -v curl > /dev/null 2>&1; then
        curl -sSL "$DOWNLOAD_URL" -o "${TMPDIR}/${BINARY}"
    elif command -v wget > /dev/null 2>&1; then
        wget -qO "${TMPDIR}/${BINARY}" "$DOWNLOAD_URL"
    fi

    chmod +x "${TMPDIR}/${BINARY}"

    # Try to install to /usr/local/bin, fall back to ~/.local/bin
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
        echo "Installed to ${INSTALL_DIR}/${BINARY}"
    else
        # Try with sudo
        if command -v sudo > /dev/null 2>&1; then
            echo "Installing to ${INSTALL_DIR} (requires sudo)..."
            sudo mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
            echo "Installed to ${INSTALL_DIR}/${BINARY}"
        else
            # Fallback to ~/.local/bin
            LOCAL_BIN="$HOME/.local/bin"
            mkdir -p "$LOCAL_BIN"
            mv "${TMPDIR}/${BINARY}" "${LOCAL_BIN}/${BINARY}"
            echo "Installed to ${LOCAL_BIN}/${BINARY}"
            echo ""
            echo "Make sure ${LOCAL_BIN} is in your PATH:"
            echo "  export PATH=\"\$HOME/.local/bin:\$PATH\""
        fi
    fi

    echo ""
    echo "Done! Run 'gruuk expose <port>' to get started."
}

install
