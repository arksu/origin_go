#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "$0")/../.." && pwd)"
blender_bin="${BLENDER_BIN:-/Applications/Blender.app/Contents/MacOS/Blender}"
pixel_python="$repo_root/tools/image-pixel-processor/.venv/bin/python"
blend_file="$repo_root/art_source/characters/male_commoner_v2/male_commoner_v2.blend"

if [[ ! -x "$blender_bin" ]]; then
  echo "Blender not found: $blender_bin" >&2
  exit 1
fi
if [[ ! -x "$pixel_python" ]]; then
  echo "Pixel processor virtualenv is missing. See tools/image-pixel-processor/README.md" >&2
  exit 1
fi

"$blender_bin" --background "$blend_file" --python "$repo_root/tools/blender/render_male_commoner_v2_raw.py" -- "$@"
"$pixel_python" "$repo_root/tools/blender/pixelize_male_commoner_v2.py" "$@"
