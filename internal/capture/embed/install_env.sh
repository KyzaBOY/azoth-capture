#!/usr/bin/env bash
# Azoth Capture - Linux environment setup.
#
# Prepares, inside the folder this script sits in:
#   - a private Python virtual environment (.venv-capture),
#   - the Aeternum-World capture tools, pinned to one fixed commit,
#   - the matching Windows Frida server (checksum-verified), which runs inside
#     the game's Proton prefix during a capture.
#
# It never runs the game, never touches the system Python, and refuses to run as
# root. The game is launched the normal way through Steam, so Easy Anti-Cheat
# stays active; this script only prepares the tools that attach to it.
set -euo pipefail

if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
    echo "ERROR: do not run this as root."
    exit 1
fi

APP_LAUNCHED=""
if [[ "${1:-}" == "__APP_LAUNCHED__" ]]; then
    APP_LAUNCHED=1
fi

ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
umask 077

# --- fixed versions and integrity --------------------------------------------
NW_CAPTURE_REPO_DEFAULT="https://github.com/Coldzer0/Aeternum-World.git"
# Pinned upstream commit. The checkout is forced to exactly this commit and never
# pulls, so a later upstream change cannot reach this machine. To use another,
# set "RepoCommit" in config.json to a full commit SHA.
NW_CAPTURE_COMMIT_DEFAULT="820156dbc44c86c9436af81aa0dba72e94cb636b"

FRIDA_VERSION="17.9.10"
FRIDA_ASSET="frida-server-${FRIDA_VERSION}-windows-x86_64.exe.xz"
FRIDA_DIGEST="ff4c0c12b96a38e79cbe480392b0d1d8e1631e95644ed47472aec5874b701b95"
FRIDA_URL="https://github.com/frida/frida/releases/download/${FRIDA_VERSION}/${FRIDA_ASSET}"

# --- derived paths -----------------------------------------------------------
CONFIG_FILE="${ROOT}/config.json"
VENV_DIR="${ROOT}/.venv-capture"
VENV_PY="${VENV_DIR}/bin/python"
CAPTURE_DIR="${ROOT}/Aeternum-World"
NW_CAPTURE_DIR="${CAPTURE_DIR}/Tools/nw_capture"
REQUIREMENTS="${NW_CAPTURE_DIR}/requirements.txt"
FRIDA_SERVER="${NW_CAPTURE_DIR}/frida-server.exe"

echo
echo "============================================================"
echo " Azoth Capture - Environment Setup (Linux)"
echo "============================================================"
echo
echo "Working in:"
echo "  ${ROOT}"
echo

# --- prerequisites -----------------------------------------------------------
missing=()
for tool in python3 git curl xz sha256sum; do
    command -v "$tool" >/dev/null 2>&1 || missing+=("$tool")
done
if [[ ${#missing[@]} -gt 0 ]]; then
    echo "ERROR: missing required tools: ${missing[*]}"
    echo "Install them with your distribution's package manager, then try again."
    exit 1
fi
# Debian and Ubuntu ship Python's venv support as a separate package.
if ! python3 -c 'import venv, ensurepip' >/dev/null 2>&1; then
    echo "ERROR: python3 cannot create virtual environments."
    echo "On Debian or Ubuntu install the python3-venv package, then try again."
    exit 1
fi

# --- read optional overrides from config.json -------------------------------
NW_CAPTURE_REPO="$NW_CAPTURE_REPO_DEFAULT"
NW_CAPTURE_COMMIT="$NW_CAPTURE_COMMIT_DEFAULT"
if [[ -f "$CONFIG_FILE" ]]; then
    repo=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1])).get("RepoUrl",""))' "$CONFIG_FILE" 2>/dev/null || true)
    commit=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1])).get("RepoCommit",""))' "$CONFIG_FILE" 2>/dev/null || true)
    [[ -n "$repo" ]] && NW_CAPTURE_REPO="$repo"
    [[ -n "$commit" ]] && NW_CAPTURE_COMMIT="$commit"
fi

# --- capture tools (pinned clone) -------------------------------------------
if [[ ! -d "${CAPTURE_DIR}/.git" ]]; then
    echo "Cloning the capture tools..."
    rm -rf "$CAPTURE_DIR"
    git clone "$NW_CAPTURE_REPO" "$CAPTURE_DIR"
fi

current_head=$(git -C "$CAPTURE_DIR" rev-parse HEAD 2>/dev/null || true)
if [[ "$current_head" == "$NW_CAPTURE_COMMIT" ]]; then
    echo "Capture tools are at the pinned commit."
else
    # Fetch only when the pin is not checked out yet, so a reinstall works
    # offline once the tools are in place.
    echo "Moving the capture tools to the pinned commit ${NW_CAPTURE_COMMIT:0:10}..."
    git -C "$CAPTURE_DIR" fetch --quiet origin
    git -C "$CAPTURE_DIR" checkout --quiet --force --detach "$NW_CAPTURE_COMMIT"
fi

# Keep recorded sessions out of git's view inside the checkout.
exclude_file="${CAPTURE_DIR}/.git/info/exclude"
mkdir -p "$(dirname "$exclude_file")"
grep -q "Tools/nw_capture/captures" "$exclude_file" 2>/dev/null || \
    printf 'Tools/nw_capture/captures/\n' >> "$exclude_file"

if [[ ! -f "${NW_CAPTURE_DIR}/nw_capture.py" ]]; then
    echo "ERROR: nw_capture.py not found after checkout."
    exit 1
fi

# --- private Python environment ---------------------------------------------
if [[ ! -x "$VENV_PY" ]]; then
    echo "Creating a private Python environment..."
    python3 -m venv "$VENV_DIR"
fi
echo "Installing Python packages..."
"$VENV_PY" -m pip install --quiet --upgrade pip
"$VENV_PY" -m pip install --quiet -r "$REQUIREMENTS"

client=$("$VENV_PY" -c 'import frida; print(frida.__version__)')
if [[ "$client" != "$FRIDA_VERSION" ]]; then
    echo "ERROR: Frida client ${client} does not match the server ${FRIDA_VERSION}."
    exit 1
fi

# --- Windows Frida server (checksum-verified) -------------------------------
if [[ ! -f "$FRIDA_SERVER" ]]; then
    echo "Downloading the Frida server (${FRIDA_VERSION})..."
    work="$(mktemp -d)"
    trap 'rm -rf "$work"' EXIT
    curl -fsSL -o "${work}/${FRIDA_ASSET}" "$FRIDA_URL"
    echo "${FRIDA_DIGEST}  ${work}/${FRIDA_ASSET}" | sha256sum -c -
    xz -d "${work}/${FRIDA_ASSET}"
    mv "${work}/${FRIDA_ASSET%.xz}" "$FRIDA_SERVER"
    chmod 600 "$FRIDA_SERVER"
fi

echo
echo "Verifying the environment..."
"$VENV_PY" - <<'PY'
import sys, frida, cryptography, lz4, websockets
print("python      :", sys.executable)
print("version     :", sys.version.split()[0])
print("frida       :", frida.__version__)
print("cryptography:", cryptography.__version__)
print("lz4         :", lz4.__version__)
print("websockets  :", websockets.__version__)
PY

echo
echo "============================================================"
echo " Environment setup complete!"
echo "============================================================"

if [[ -z "$APP_LAUNCHED" ]]; then
    echo
    echo "Done. You can close this window."
fi
