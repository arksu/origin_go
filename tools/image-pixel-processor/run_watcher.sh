#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="${VENV_DIR:-"$SCRIPT_DIR/.venv"}"
REQUIREMENTS_FILE="$SCRIPT_DIR/requirements.txt"
WATCHER_FILE="$SCRIPT_DIR/watcher.py"

if [[ ! -d "$VENV_DIR" ]]; then
  echo "Creating virtual environment at: $VENV_DIR"
  python3 -m venv "$VENV_DIR"
fi

# shellcheck disable=SC1091
source "$VENV_DIR/bin/activate"

python3 -m pip install -r "$REQUIREMENTS_FILE"

exec python3 "$WATCHER_FILE" "$@"
