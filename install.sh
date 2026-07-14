#!/bin/sh
set -e

REPO="jullury/akama"
INSTALL_DIR="/usr/local/bin"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64)         ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

case "$OS" in
  linux|darwin) ;;
  *)
    echo "Unsupported OS: $OS"
    exit 1
    ;;
esac

# --- Install host CLI binary ---
ASSET="akama-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

echo "Downloading akama host CLI for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "/tmp/akama"
chmod +x "/tmp/akama"

echo "Installing to ${INSTALL_DIR}/akama..."
if [ -w "$INSTALL_DIR" ]; then
  mv "/tmp/akama" "${INSTALL_DIR}/akama"
else
  sudo mv "/tmp/akama" "${INSTALL_DIR}/akama"
fi

echo ""
echo "akama host CLI installed successfully!"
echo ""
echo "The akama binary manages host-side operations:"
echo "  - akama init      : configure Telegram bot token, API keys, admin user"
echo "  - akama start     : launch Docker containers (daemon, postgres, ollama)"
echo "  - akama stop      : stop all containers"
echo "  - akama status    : check container health and active jobs"
echo "  - akama logs      : view daemon container logs"
echo "  - akama restart   : restart all containers"
echo "  - akama update    : pull latest daemon image and recreate container"
echo ""
echo "The daemon (Telegram bot + agent execution) runs inside the"
echo "akama-daemon Docker container, managed automatically by 'akama start'."
echo ""
echo "Run 'akama init' to get started."
