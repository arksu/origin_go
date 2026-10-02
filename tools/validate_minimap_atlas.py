#!/usr/bin/env python3
"""Validate the static minimap atlas using only the standard library.

The default inputs are the distributed client assets. --self-test checks valid
and deliberately broken fixtures without requiring those assets to exist.
"""

import argparse
import copy
import json
from pathlib import Path
import re
import sys
import tempfile

from png_codec import decode_rgba_png, encode_rgba_png, rects_overlap


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_ASSET_ROOT = REPOSITORY_ROOT / "web_new/public/assets/game"
TILE_IDS_SOURCE = REPOSITORY_ROOT / "web_new/src/game/tiles/tileIds.ts"
MINIMAP_CONFIG_SOURCE = REPOSITORY_ROOT / "web_new/src/constants/minimap.ts"


def read_client_contract():
    """Derive coverage and material dimensions from the client's source of truth."""
    source = TILE_IDS_SOURCE.read_text(encoding="utf-8")
    source = re.sub(r"//[^\n]*|/\*[\s\S]*?\*/", "", source)
    constants = {
        name: int(value)
        for name, value in re.findall(r"export\s+const\s+(TILE_\w+)\s*=\s*(\d+)\b", source)
    }
    renderable = re.search(r"RENDERABLE_TILE_IDS[^=]*=\s*\[([^\]]*)\]", source)
    if not renderable:
        raise ValueError(f"{TILE_IDS_SOURCE}: cannot read RENDERABLE_TILE_IDS")
    names = [name.strip() for name in renderable.group(1).split(",") if name.strip()]
    if not names or any(name not in constants for name in names):
        raise ValueError(f"{TILE_IDS_SOURCE}: renderable list must reference numeric TILE_* constants")
    tile_ids = [constants[name] for name in names]
    if len(set(tile_ids)) != len(tile_ids) or constants.get("TILE_VOID") in tile_ids:
        raise ValueError(f"{TILE_IDS_SOURCE}: renderable IDs must be unique and exclude TILE_VOID")
    size_match = re.search(
        r"export\s+const\s+MINIMAP_TEXTURE_SIZE\s*=\s*(\d+)\b",
        MINIMAP_CONFIG_SOURCE.read_text(encoding="utf-8"),
    )
    if not size_match or int(size_match.group(1)) < 1:
        raise ValueError(f"{MINIMAP_CONFIG_SOURCE}: cannot read a positive MINIMAP_TEXTURE_SIZE")
    return tile_ids, int(size_match.group(1))


def unique_json_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def validate_atlas(image_path, manifest_path, tile_ids, texture_size):
    manifest = json.loads(
        Path(manifest_path).read_text(encoding="utf-8"),
        object_pairs_hook=unique_json_object,
    )
    if not isinstance(manifest, dict):
        raise ValueError("manifest must be an object keyed by decimal tile ID")
    expected_keys = {str(tile_id) for tile_id in tile_ids}
    actual_keys = set(manifest)
    if actual_keys != expected_keys:
        raise ValueError(
            "manifest tile IDs differ from RENDERABLE_TILE_IDS: "
            f"missing={sorted(expected_keys - actual_keys)}, unexpected={sorted(actual_keys - expected_keys)}"
        )
    image_width, image_height, rgba = decode_rgba_png(image_path)
    if image_width < 1 or image_height < 1 or len(rgba) != image_width * image_height * 4:
        raise ValueError("invalid PNG pixel dimensions")
    rectangles = {}
    required_fields = {"x", "y", "width", "height"}
    for tile_id in tile_ids:
        region = manifest[str(tile_id)]
        if not isinstance(region, dict) or set(region) != required_fields:
            raise ValueError(f"tile {tile_id}: rectangle must contain exactly x, y, width, height")
        if any(type(region[field]) is not int for field in required_fields):
            raise ValueError(f"tile {tile_id}: rectangle coordinates and sizes must be integers, not booleans")
        left, top, width, height = (region[field] for field in ("x", "y", "width", "height"))
        if width != texture_size or height != texture_size:
            raise ValueError(f"tile {tile_id}: region must be {texture_size} by {texture_size}")
        if left < 0 or top < 0 or left + width > image_width or top + height > image_height:
            raise ValueError(f"tile {tile_id}: rectangle is out of bounds in {image_width} by {image_height} PNG")
        rectangle = (left, top, width, height)
        for previous_id, previous_rectangle in rectangles.items():
            if rects_overlap(rectangle, previous_rectangle):
                raise ValueError(f"tile {tile_id}: region overlaps tile {previous_id}")
        rectangles[tile_id] = rectangle
        for row in range(top, top + height):
            for column in range(left, left + width):
                if rgba[(row * image_width + column) * 4 + 3] != 255:
                    raise ValueError(f"tile {tile_id}: region must be opaque; transparent pixel at ({column}, {row})")
    return image_width, image_height


def run_self_tests(tile_ids, texture_size):
    columns = 5
    image_width = columns * texture_size
    image_height = ((len(tile_ids) + columns - 1) // columns) * texture_size
    rgba = bytearray([100, 120, 80, 255] * (image_width * image_height))
    manifest = {
        str(tile_id): {
            "x": index % columns * texture_size,
            "y": index // columns * texture_size,
            "width": texture_size,
            "height": texture_size,
        }
        for index, tile_id in enumerate(tile_ids)
    }
    first_id, second_id = map(str, tile_ids[:2])
    cases = []

    def add_case(label, change, expected_message):
        broken = copy.deepcopy(manifest)
        change(broken)
        cases.append((label, broken, rgba, expected_message))

    add_case("missing ID", lambda fixture: fixture.pop(first_id), "missing=")
    add_case("extra void ID", lambda fixture: fixture.update({"255": manifest[first_id]}), "unexpected=")
    add_case("overlap", lambda fixture: fixture.update({second_id: fixture[first_id]}), "overlaps")
    add_case("out of bounds", lambda fixture: fixture[first_id].update(x=image_width), "out of bounds")
    add_case("boolean coordinate", lambda fixture: fixture[first_id].update(x=False), "integers")
    add_case("fractional coordinate", lambda fixture: fixture[first_id].update(x=0.5), "integers")
    add_case("wrong size", lambda fixture: fixture[first_id].update(width=texture_size - 1), "region must be")
    transparent = bytearray(rgba)
    transparent[3] = 254
    cases.append(("transparency", manifest, transparent, "opaque"))

    with tempfile.TemporaryDirectory(prefix="minimap-atlas-") as directory:
        image_path = Path(directory) / "atlas.png"
        manifest_path = Path(directory) / "atlas.json"

        def write_fixture(mapping, pixels):
            image_path.write_bytes(encode_rgba_png(image_width, image_height, pixels))
            manifest_path.write_text(json.dumps(mapping), encoding="utf-8")

        write_fixture(manifest, rgba)
        assert validate_atlas(image_path, manifest_path, tile_ids, texture_size) == (image_width, image_height)
        for label, mapping, pixels, expected_message in cases:
            write_fixture(mapping, pixels)
            try:
                validate_atlas(image_path, manifest_path, tile_ids, texture_size)
            except ValueError as error:
                assert expected_message in str(error), f"{label}: unexpected error: {error}"
            else:
                raise AssertionError(f"{label}: invalid atlas was accepted")
    print(f"Minimap atlas self-test passed: valid fixture and {len(cases)} invalid fixtures")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("image", nargs="?", type=Path, default=DEFAULT_ASSET_ROOT / "minimap_base.png")
    parser.add_argument("manifest", nargs="?", type=Path, default=DEFAULT_ASSET_ROOT / "minimap_base.json")
    parser.add_argument("--self-test", action="store_true", help="validate synthetic good and bad fixtures")
    arguments = parser.parse_args()
    try:
        tile_ids, texture_size = read_client_contract()
        if arguments.self_test:
            run_self_tests(tile_ids, texture_size)
        else:
            width, height = validate_atlas(arguments.image, arguments.manifest, tile_ids, texture_size)
            print(f"Minimap atlas valid: {len(tile_ids)} opaque {texture_size}x{texture_size} regions in {width}x{height} PNG")
    except (OSError, ValueError) as error:
        print(f"Minimap atlas validation failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
