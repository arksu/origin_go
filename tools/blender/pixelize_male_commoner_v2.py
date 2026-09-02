"""Convert transparent Blender renders into a stable, palette-locked pixel grid."""

from __future__ import annotations

import argparse
import json
from pathlib import Path

import numpy as np
from PIL import Image


ROOT = Path(__file__).resolve().parents[2]
CHARACTER_ROOT = ROOT / "art_source" / "characters" / "male_commoner_v2"
BAKE_ROOT = CHARACTER_ROOT / "bake"
PALETTE_PATH = CHARACTER_ROOT / "pixel_bake_palette.json"


def parse_arguments() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--preview", action="store_true", help="label the manifest as a preview bake")
    return parser.parse_args()


def load_config() -> dict[str, object]:
    with PALETTE_PATH.open() as source:
        config = json.load(source)
    if len(config["logical_size"]) != 2 or len(config["source_size"]) != 2:
        raise ValueError("Pixel bake configuration has invalid dimensions")
    if not config["palette_rgba"]:
        raise ValueError("Pixel bake palette is empty")
    return config


def dilate_4_connected(mask: np.ndarray) -> np.ndarray:
    padded = np.pad(mask, 1, constant_values=False)
    return mask | padded[:-2, 1:-1] | padded[2:, 1:-1] | padded[1:-1, :-2] | padded[1:-1, 2:]


def pixelize(source_path: Path, config: dict[str, object]) -> tuple[Image.Image, Image.Image]:
    source = Image.open(source_path).convert("RGBA")
    expected_source_size = tuple(config["source_size"])
    if source.size != expected_source_size:
        raise ValueError(f"{source_path} has size {source.size}; expected {expected_source_size}")
    logical_size = tuple(config["logical_size"])
    resized = source.resize(logical_size, Image.Resampling.LANCZOS)
    pixels = np.asarray(resized, dtype=np.uint8)
    opaque = pixels[:, :, 3] >= 96
    if not opaque.any():
        raise ValueError(f"{source_path} has no opaque pixels")

    palette = np.asarray(config["palette_rgba"], dtype=np.int16)[:, :3]
    rgb = pixels[:, :, :3].astype(np.int16)
    distance = np.sum((rgb[:, :, None, :] - palette[None, None, :, :]) ** 2, axis=3)
    nearest = palette[np.argmin(distance, axis=2)].astype(np.uint8)
    result = np.zeros_like(pixels)
    result[opaque, :3] = nearest[opaque]
    result[opaque, 3] = 255
    outline = dilate_4_connected(opaque) & ~opaque
    result[outline] = np.asarray(config["outline_rgba"], dtype=np.uint8)
    logical = Image.fromarray(result, "RGBA")
    display_scale = int(config["display_scale"])
    review = logical.resize((logical.width * display_scale, logical.height * display_scale), Image.Resampling.NEAREST)
    return logical, review


def write_manifest(config: dict[str, object], mode: str, frames: list[Path]) -> None:
    grouped: dict[str, dict[str, dict[str, list[str]]]] = {}
    for frame in frames:
        relative = frame.relative_to(BAKE_ROOT / "raw")
        render_pass, action, direction, filename = relative.parts
        grouped.setdefault(render_pass, {}).setdefault(action, {}).setdefault(direction, []).append(filename)
    for pass_frames in grouped.values():
        for action_frames in pass_frames.values():
            for filenames in action_frames.values():
                filenames.sort()
    manifest = {
        "character": "male_commoner_v2",
        "mode": mode,
        "logical_size": config["logical_size"],
        "source_size": config["source_size"],
        "display_scale": config["display_scale"],
        "layer_order": ["body", "hair", "base_loincloth", "face_details"],
        "outline_rgba": config["outline_rgba"],
        "palette_rgba": config["palette_rgba"],
        "frames": grouped,
    }
    (BAKE_ROOT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")


def main() -> None:
    options = parse_arguments()
    config = load_config()
    raw_root = BAKE_ROOT / "raw"
    frames = sorted(raw_root.glob("*/*/*/*.png"))
    if not frames:
        raise RuntimeError(f"No raw frames found under {raw_root}")
    for raw_frame in frames:
        relative = raw_frame.relative_to(raw_root)
        logical, review = pixelize(raw_frame, config)
        pixel_output = BAKE_ROOT / "pixel" / relative
        review_output = BAKE_ROOT / "preview_4x" / relative
        pixel_output.parent.mkdir(parents=True, exist_ok=True)
        review_output.parent.mkdir(parents=True, exist_ok=True)
        logical.save(pixel_output)
        review.save(review_output)
    write_manifest(config, "preview" if options.preview else "full", frames)


if __name__ == "__main__":
    main()
