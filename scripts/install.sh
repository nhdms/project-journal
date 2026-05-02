#!/bin/sh
# project-journal installer
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nhdms/project-journal/main/scripts/install.sh | sh
#
# Env overrides:
#   PJ_VERSION         pin version (default: latest)
#   PJ_INSTALL_DIR     binary install dir (default: /usr/local/bin or ~/.local/bin)
#   PJ_SKIP_PLUGIN     set to 1 to skip Claude Code plugin install
#   PJ_PLUGIN_ONLY     set to 1 to skip binary, only install plugin

set -e

REPO="nhdms/project-journal"
PJ_VERSION="${PJ_VERSION:-latest}"
PJ_ASSUME_YES="${PJ_ASSUME_YES:-}"

# Open /dev/tty for prompts when stdin is a pipe (curl | sh)
if [ -t 0 ]; then
  TTY=/dev/stdin
elif [ -r /dev/tty ]; then
  TTY=/dev/tty
else
  TTY=""
fi

# confirm "Question" "default(Y|n)" → returns 0 yes / 1 no
confirm() {
  prompt="$1"
  default="${2:-Y}"
  hint="[Y/n]"
  [ "${default}" = "n" ] && hint="[y/N]"

  if [ "${PJ_ASSUME_YES}" = "1" ]; then
    echo "${prompt} ${hint} → yes (PJ_ASSUME_YES=1)"
    return 0
  fi

  if [ -z "${TTY}" ]; then
    # Non-interactive: use default
    echo "${prompt} ${hint} → ${default} (non-interactive)"
    [ "${default}" = "Y" ] && return 0 || return 1
  fi

  printf "%s %s " "${prompt}" "${hint}"
  read -r ans < "${TTY}" || ans=""
  case "${ans}" in
    [Yy]|[Yy][Ee][Ss]) return 0 ;;
    [Nn]|[Nn][Oo]) return 1 ;;
    "") [ "${default}" = "Y" ] && return 0 || return 1 ;;
    *) [ "${default}" = "Y" ] && return 0 || return 1 ;;
  esac
}

# Detect package manager + offer to install jq
ensure_jq() {
  if command -v jq >/dev/null 2>&1; then
    return 0
  fi

  echo ""
  echo "==> jq is not installed."
  echo "    jq is required for:"
  echo "      - Installing the Claude Code plugin (this script)"
  echo "      - Plugin hooks at runtime (parsing event payloads)"
  echo ""

  CMD=""
  if command -v apt-get >/dev/null 2>&1; then
    CMD="sudo apt-get install -y jq"
  elif command -v dnf >/dev/null 2>&1; then
    CMD="sudo dnf install -y jq"
  elif command -v yum >/dev/null 2>&1; then
    CMD="sudo yum install -y jq"
  elif command -v pacman >/dev/null 2>&1; then
    CMD="sudo pacman -S --noconfirm jq"
  elif command -v apk >/dev/null 2>&1; then
    CMD="sudo apk add jq"
  elif command -v brew >/dev/null 2>&1; then
    CMD="brew install jq"
  fi

  if [ -z "${CMD}" ]; then
    echo "    No supported package manager detected. Install jq manually:"
    echo "      https://stedolan.github.io/jq/download/"
    return 1
  fi

  echo "    Detected install command: ${CMD}"
  if confirm "    Run it now?" "Y"; then
    sh -c "${CMD}"
    if command -v jq >/dev/null 2>&1; then
      echo "==> jq installed ✓"
      return 0
    else
      echo "==> jq install reported success but binary not found in PATH."
      return 1
    fi
  else
    echo "==> Skipping jq install."
    return 1
  fi
}

# ────────────────────────────────────────────────
# 1) Binary install
# ────────────────────────────────────────────────

install_binary() {
  OS="$(uname -s)"
  case "${OS}" in
    Linux)  GOOS="linux" ;;
    Darwin) GOOS="darwin" ;;
    *) echo "ERROR: unsupported OS: ${OS}"; exit 1 ;;
  esac

  ARCH="$(uname -m)"
  case "${ARCH}" in
    x86_64|amd64)   GOARCH="amd64" ;;
    aarch64|arm64)  GOARCH="arm64" ;;
    *) echo "ERROR: unsupported arch: ${ARCH}"; exit 1 ;;
  esac

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
        return 0
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
    exit 1
  fi

  if curl -fsSL "${URL}.sha256" -o "${TMP}/${ASSET}.sha256" 2>/dev/null; then
    echo "==> Verifying checksum..."
    (cd "${TMP}" && sha256sum -c "${ASSET}.sha256" >/dev/null) \
      || { echo "ERROR: checksum mismatch"; exit 1; }
  fi

  echo "==> Extracting..."
  tar -xzf "${TMP}/${ASSET}" -C "${TMP}"

  BIN="${TMP}/pj-${GOOS}-${GOARCH}"
  [ -x "${BIN}" ] || chmod +x "${BIN}"

  DEST="${PJ_INSTALL_DIR}/pj"
  echo "==> Installing binary to ${DEST}"
  if [ -w "${PJ_INSTALL_DIR}" ]; then
    mv "${BIN}" "${DEST}"
  elif command -v sudo >/dev/null 2>&1; then
    sudo mv "${BIN}" "${DEST}"
  else
    echo "ERROR: cannot write to ${PJ_INSTALL_DIR} and no sudo."
    exit 1
  fi

  case ":${PATH}:" in
    *":${PJ_INSTALL_DIR}:"*) echo "==> ${PJ_INSTALL_DIR} is in PATH ✓" ;;
    *)
      echo "==> WARNING: ${PJ_INSTALL_DIR} is NOT in PATH"
      echo "    Add to ~/.bashrc or ~/.zshrc:"
      echo "      export PATH=\"${PJ_INSTALL_DIR}:\$PATH\""
      ;;
  esac
}

# ────────────────────────────────────────────────
# 2) Claude Code plugin install
# ────────────────────────────────────────────────

install_plugin() {
  CLAUDE_DIR="${HOME}/.claude"
  PLUGINS_DIR="${CLAUDE_DIR}/plugins"

  if [ ! -d "${CLAUDE_DIR}" ]; then
    echo "==> ${CLAUDE_DIR} not found — skipping plugin install (Claude Code not detected)"
    return 0
  fi

  echo ""
  echo "==> Claude Code detected at ${CLAUDE_DIR}"
  if ! confirm "    Install the project-journal plugin into Claude Code?" "Y"; then
    echo "==> Skipping plugin install."
    print_manual_plugin_instructions
    return 0
  fi

  if ! ensure_jq; then
    echo "==> jq is required to register the plugin. Skipping."
    print_manual_plugin_instructions
    return 0
  fi

  if ! command -v git >/dev/null 2>&1; then
    echo "==> git required for plugin install — skipping"
    print_manual_plugin_instructions
    return 0
  fi

  MP_NAME="project-journal-local"
  MP_PATH="${PLUGINS_DIR}/marketplaces/${MP_NAME}"
  KMP="${PLUGINS_DIR}/known_marketplaces.json"
  IP="${PLUGINS_DIR}/installed_plugins.json"
  NOW="$(date -u +%FT%TZ)"
  PLUGIN_VERSION="${PJ_VERSION:-unknown}"

  mkdir -p "${PLUGINS_DIR}/marketplaces"

  # Clone or update marketplace
  if [ -d "${MP_PATH}/.git" ]; then
    echo "==> Updating marketplace clone at ${MP_PATH}"
    git -C "${MP_PATH}" pull --quiet --ff-only || true
  else
    echo "==> Cloning marketplace to ${MP_PATH}"
    git clone --quiet "https://github.com/${REPO}.git" "${MP_PATH}"
  fi

  # Backup existing JSONs
  [ -f "${KMP}" ] && cp "${KMP}" "${KMP}.bak.$(date +%s)"
  [ -f "${IP}" ] && cp "${IP}" "${IP}.bak.$(date +%s)"

  # known_marketplaces.json: upsert entry
  TMP=$(mktemp)
  if [ -f "${KMP}" ] && [ -s "${KMP}" ]; then
    jq --arg name "${MP_NAME}" --arg repo "${REPO}" --arg loc "${MP_PATH}" --arg now "${NOW}" \
      '.[$name] = {source: {source:"github", repo:$repo}, installLocation: $loc, lastUpdated: $now}' \
      "${KMP}" > "${TMP}" && mv "${TMP}" "${KMP}"
  else
    jq -n --arg name "${MP_NAME}" --arg repo "${REPO}" --arg loc "${MP_PATH}" --arg now "${NOW}" \
      '{($name): {source: {source:"github", repo:$repo}, installLocation: $loc, lastUpdated: $now}}' \
      > "${KMP}"
  fi
  echo "==> Registered marketplace in ${KMP}"

  # installed_plugins.json: upsert plugin
  PLUGIN_KEY="project-journal@${MP_NAME}"
  PLUGIN_INSTALL_PATH="${MP_PATH}/plugin"
  TMP=$(mktemp)
  if [ -f "${IP}" ] && [ -s "${IP}" ]; then
    jq --arg key "${PLUGIN_KEY}" --arg ipath "${PLUGIN_INSTALL_PATH}" --arg ver "${PLUGIN_VERSION}" --arg now "${NOW}" \
      '.plugins[$key] = [{scope:"user", installPath: $ipath, version: $ver, installedAt: $now, lastUpdated: $now}]' \
      "${IP}" > "${TMP}" && mv "${TMP}" "${IP}"
  else
    jq -n --arg key "${PLUGIN_KEY}" --arg ipath "${PLUGIN_INSTALL_PATH}" --arg ver "${PLUGIN_VERSION}" --arg now "${NOW}" \
      '{version:2, plugins: {($key): [{scope:"user", installPath: $ipath, version: $ver, installedAt: $now, lastUpdated: $now}]}}' \
      > "${IP}"
  fi
  echo "==> Registered plugin in ${IP}"

  # Sanity check plugin files exist
  if [ ! -f "${PLUGIN_INSTALL_PATH}/.claude-plugin/plugin.json" ]; then
    echo "WARNING: plugin manifest not found at ${PLUGIN_INSTALL_PATH}/.claude-plugin/plugin.json"
    echo "         Check that the marketplace clone has the expected layout."
  else
    echo "==> Plugin files at ${PLUGIN_INSTALL_PATH} ✓"
  fi

  echo "==> Plugin installed. Restart Claude Code to activate."
}

print_manual_plugin_instructions() {
  cat <<EOF
==> Manual plugin install (run inside Claude Code):

  /plugin marketplace add nhdms/project-journal
  /plugin install project-journal@project-journal-local

EOF
}

# ────────────────────────────────────────────────
# 3) Run
# ────────────────────────────────────────────────

if [ "${PJ_PLUGIN_ONLY}" != "1" ]; then
  install_binary
fi

if [ "${PJ_SKIP_PLUGIN}" != "1" ]; then
  install_plugin
fi

# ────────────────────────────────────────────────
# 4) Done
# ────────────────────────────────────────────────

echo ""
echo "Done."
echo ""
echo "Quick start:"
echo "  cd ~/your-project"
echo "  pj init"
echo "  pj task add T1 \"First task\""
echo "  pj start T1"
echo "  claude       # plugin auto-injects briefing on session start"
