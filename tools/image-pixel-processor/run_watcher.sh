#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="${VENV_DIR:-"$SCRIPT_DIR/.venv"}"
REQUIREMENTS_FILE="$SCRIPT_DIR/requirements.txt"
WATCHER_FILE="$SCRIPT_DIR/watcher.py"
REQUIREMENTS_HASH_FILE="$VENV_DIR/.requirements.sha256"

if [[ ! -d "$VENV_DIR" ]]; then
  echo "Creating virtual environment at: $VENV_DIR"
  python3 -m venv "$VENV_DIR"
fi

# shellcheck disable=SC1091
source "$VENV_DIR/bin/activate"

current_hash="$(python3 - "$REQUIREMENTS_FILE" <<'PY'
from pathlib import Path
import hashlib
import sys

requirements = Path(sys.argv[1])
print(hashlib.sha256(requirements.read_bytes()).hexdigest())
PY
)"
stored_hash=""
if [[ -f "$REQUIREMENTS_HASH_FILE" ]]; then
  stored_hash="$(cat "$REQUIREMENTS_HASH_FILE")"
fi

if [[ "${FORCE_PIP_INSTALL:-0}" == "1" || "$current_hash" != "$stored_hash" ]]; then
  echo "Installing/updating Python dependencies..."
  python3 -m pip install -r "$REQUIREMENTS_FILE"
  printf '%s' "$current_hash" > "$REQUIREMENTS_HASH_FILE"
else
  echo "Dependencies are up to date, skipping pip install."
fi

exec python3 "$WATCHER_FILE" "$@"
