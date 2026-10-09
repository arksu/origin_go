#!/usr/bin/env python3
"""Publish the selected paired skeleton sprites without changing their artwork.

Both images retain their common square canvas. Nearest-neighbor resampling to
128 px matches the commoner's ~96 px body height and keeps the pixel clusters
crisp. The client's common (64, 56) abdomen anchor is independent of the skull.
Run with --check to verify the published assets without rewriting them.
"""

import argparse
import hashlib
from pathlib import Path

from png_codec import decode_rgba_png, encode_rgba_png

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "art_source/objects/skeleton"
OUTPUT = ROOT / "web_new/public/assets/game/obj/skeleton"
SIZE = 128
SOURCE_SHA256 = {
    "skeleton-with-skull.png": "05a63399a62a8767a67fa3e1f310dbc383ab2053d3830327f508ea3320912cd1",
    "skeleton-without-skull.png": "dcc2b31c6dc1a4fbd11b549ad8ddfba8ef1e466acf9b47d09c1ac9cd523559df",
}


def publish(check=False):
    dimensions = None
    for name, expected_hash in SOURCE_SHA256.items():
        source = SOURCE / name
        if hashlib.sha256(source.read_bytes()).hexdigest() != expected_hash:
            raise ValueError(f"selected source changed: {source}")
        width, height, rgba = decode_rgba_png(source)
        if width != height or dimensions is not None and dimensions != (width, height):
            raise ValueError("paired skeletons must share their square canvas")
        dimensions = (width, height)
        pixels = bytearray(SIZE * SIZE * 4)
        for y in range(SIZE):
            source_y = int((y + .5) * height / SIZE)
            for x in range(SIZE):
                source_x = int((x + .5) * width / SIZE)
                offset = (source_y * width + source_x) * 4
                output_offset = (y * SIZE + x) * 4
                pixels[output_offset:output_offset + 4] = rgba[offset:offset + 4]
        # A separate compact silhouette shadow can be hidden while lifting.
        # Its client layer moves two pixels down/right, opposite the fixed light.
        shadow = bytearray(pixels)
        for offset in range(0, len(shadow), 4):
            shadow[offset:offset + 3] = bytes((15, 22, 11))
            shadow[offset + 3] = round(shadow[offset + 3] * .2)
        for output_name, output_pixels in [(name, pixels), (f"{Path(name).stem}-shadow.png", shadow)]:
            published = encode_rgba_png(SIZE, SIZE, output_pixels)
            output = OUTPUT / output_name
            if check:
                if output.read_bytes() != published:
                    raise ValueError(f"sprite needs export: {output}")
            else:
                output.parent.mkdir(parents=True, exist_ok=True)
                output.write_bytes(published)
            print(f"{'Verified' if check else 'Exported'} {output.relative_to(ROOT)} ({SIZE}x{SIZE})")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    publish(parser.parse_args().check)
