#!/bin/sh
# project-journal installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nhdms/project-journal/main/scripts/install.sh | sh
#
# Env overrides:
#   PJ_VERSION   pin to a specific tag (default: latest)
#   PJ_INSTALL_DIR  install location (default: /usr/local/bin or ~/.local/bin)

set -e

REPO="nhdms/project-journal"
PJ_VERSION="${PJ_VERSION:-latest}"

# --- Detect OS ---
OS="$(uname -s)"
case "${OS}" in
  Linux)  GOOS="linux" ;;
  Darwin) GOOS="darwin" ;;
  *) echo "ERROR: unsupported OS: ${OS}"; exit 1 ;;
esac

# --- Detect arch ---
ARCH="$(uname -m)"
case "${ARCH}" in
  x86_64|amd64)   GOARCH="amd64" ;;
  aarch64|arm64)  GOARCH="arm64" ;;
  *) echo "ERROR: unsupported arch: ${ARCH}"; exit 1 ;;
esac

# --- Resolve install dir ---
if [ -z "${PJ_INSTALL_DIR}" ]; then
  if [ -w "/usr/local/bin" ] 2>/dev/null; then
    PJ_INSTALL_DIR="/usr/local/bin"
  elif [ "$(id -u)" = "0" ]; then
    PJ_INSTALL_DIR="/usr/local/bin"
  else
    PJ_INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "${PJ_INSTALL_DIR}"
  fi
fi

# --- Resolve version ---
if [ "${PJ_VERSION}" = "latest" ]; then
  echo "==> Resolving latest release..."
  PJ_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -m1 '"tag_name"' \
    | sed -E 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/')"
  if [ -z "${PJ_VERSION}" ]; then
    echo "ERROR: could not resolve latest release. Falling back to 'go install'..."
    if command -v go >/dev/null 2>&1; then
      go install "github.com/${REPO}/cmd/pj@latest"
      echo "Installed via 'go install' to $(go env GOPATH)/bin/pj"
      exit 0
    fi
    echo "ERROR: no Go available either. Install Go or specify PJ_VERSION."
    exit 1
  fi
fi

ASSET="pj-${GOOS}-${GOARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${PJ_VERSION}/${ASSET}"

echo "==> Downloading ${PJ_VERSION} for ${GOOS}-${GOARCH}"
echo "    ${URL}"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

if ! curl -fsSL "${URL}" -o "${TMP}/${ASSET}"; then
  echo "ERROR: download failed."
  echo "  Available assets: https://github.com/${REPO}/releases/${PJ_VERSION}"
  exit 1
fi

# Optional checksum verify
if curl -fsSL "${URL}.sha256" -o "${TMP}/${ASSET}.sha256" 2>/dev/null; then
  echo "==> Verifying checksum..."
  (cd "${TMP}" && sha256sum -c "${ASSET}.sha256" >/dev/null) \
    || { echo "ERROR: checksum mismatch"; exit 1; }
fi

echo "==> Extracting..."
tar -xzf "${TMP}/${ASSET}" -C "${TMP}"

BIN="${TMP}/pj-${GOOS}-${GOARCH}"
[ -x "${BIN}" ] || chmod +x "${BIN}"

# Install
DEST="${PJ_INSTALL_DIR}/pj"
echo "==> Installing to ${DEST}"
if [ -w "${PJ_INSTALL_DIR}" ]; then
  mv "${BIN}" "${DEST}"
elif command -v sudo >/dev/null 2>&1; then
  sudo mv "${BIN}" "${DEST}"
else
  echo "ERROR: cannot write to ${PJ_INSTALL_DIR} and no sudo available."
  echo "  Set PJ_INSTALL_DIR=~/.local/bin or run as root."
  exit 1
fi

echo ""
"${DEST}" --help | head -3 || true
echo ""

# PATH check
case ":${PATH}:" in
  *":${PJ_INSTALL_DIR}:"*) echo "==> ${PJ_INSTALL_DIR} is in PATH ✓" ;;
  *)
    echo "==> WARNING: ${PJ_INSTALL_DIR} is NOT in PATH"
    echo "    Add to ~/.bashrc or ~/.zshrc:"
    echo "      export PATH=\"${PJ_INSTALL_DIR}:\$PATH\""
    ;;
esac

# jq check (needed for plugin)
if ! command -v jq >/dev/null 2>&1; then
  echo "==> NOTE: jq not found (required for the Claude Code plugin hooks)"
  echo "    Ubuntu/Debian:  sudo apt install -y jq"
  echo "    Fedora/RHEL:    sudo dnf install -y jq"
  echo "    Arch:           sudo pacman -S jq"
  echo "    Alpine:         sudo apk add jq"
  echo "    macOS:          brew install jq"
fi

echo ""
echo "Done. Try: pj --help"
