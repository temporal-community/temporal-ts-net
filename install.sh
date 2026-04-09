#!/bin/sh
# Install temporal-ts-net to /usr/local/bin.
# Usage: curl -sSfL https://raw.githubusercontent.com/chaptersix/temporal-start-dev-ext/main/install.sh | sh
set -e

REPO="chaptersix/temporal-start-dev-ext"
BINARY="temporal-ts_net"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s)
case "$OS" in
  Darwin) OS="Darwin" ;;
  Linux)  OS="Linux" ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  ARCH="x86_64" ;;
  amd64)   ARCH="x86_64" ;;
  arm64)   ARCH="arm64" ;;
  aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Get latest version from GitHub
VERSION=$(curl -sSf "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"v?([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
  echo "Failed to determine latest version."
  exit 1
fi

ARCHIVE="temporal-ts-net_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"

echo "Installing temporal-ts-net v${VERSION} (${OS}/${ARCH})..."

# Download and extract to a temp directory
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

curl -sSfL "$URL" -o "${TMPDIR}/${ARCHIVE}"
tar -xzf "${TMPDIR}/${ARCHIVE}" -C "$TMPDIR"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Need sudo to install to ${INSTALL_DIR}"
  sudo mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

echo "Installed ${BINARY} to ${INSTALL_DIR}/${BINARY}"
echo "Verify with: temporal help --all"
