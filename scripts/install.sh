#!/bin/sh
# project-journal installer
# Usage: curl -fsSL https://raw.githubusercontent.com/nhdms/project-journal/main/scripts/install.sh | sh

set -e

REPO="github.com/nhdms/project-journal"
PKG="${REPO}/cmd/pj@latest"

echo "==> Installing pj from ${REPO}"

# Check Go
if ! command -v go >/dev/null 2>&1; then
  echo "ERROR: Go is required but not installed."
  echo ""
  echo "Install Go on Linux:"
  echo "  Ubuntu/Debian:  sudo apt install -y golang-go"
  echo "  Fedora/RHEL:    sudo dnf install -y golang"
  echo "  Arch:           sudo pacman -S go"
  echo "  Or download:    https://go.dev/dl/"
  exit 1
fi

GO_VERSION=$(go version | awk '{print $3}')
echo "==> Found ${GO_VERSION}"

# Install
echo "==> go install ${PKG}"
go install "${PKG}"

# Detect GOPATH/bin
GOBIN="$(go env GOBIN)"
[ -z "${GOBIN}" ] && GOBIN="$(go env GOPATH)/bin"

if [ ! -x "${GOBIN}/pj" ]; then
  echo "ERROR: pj binary not found at ${GOBIN}/pj after install."
  exit 1
fi

echo ""
echo "==> Installed: ${GOBIN}/pj"
"${GOBIN}/pj" --help | head -3 || true
echo ""

# PATH check
case ":${PATH}:" in
  *":${GOBIN}:"*) echo "==> ${GOBIN} is in PATH ✓" ;;
  *)
    echo "==> WARNING: ${GOBIN} is NOT in PATH"
    echo "    Add this to ~/.bashrc or ~/.zshrc:"
    echo "      export PATH=\"${GOBIN}:\$PATH\""
    ;;
esac

# jq check (optional but needed for plugin hooks)
if ! command -v jq >/dev/null 2>&1; then
  echo "==> NOTE: jq not found (required if using the Claude Code plugin)"
  echo "    Ubuntu/Debian:  sudo apt install -y jq"
  echo "    Fedora/RHEL:    sudo dnf install -y jq"
  echo "    Arch:           sudo pacman -S jq"
fi

echo ""
echo "Done. Try: pj --help"
