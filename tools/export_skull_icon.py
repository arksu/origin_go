#!/usr/bin/env python3
"""Export the generated skull source to a transparent 32x32 inventory icon.

Nearest-neighbor sampling preserves the source pixel clusters and alpha.
--check verifies both the pinned original source and byte-exact publication.
"""

import argparse
import hashlib
from pathlib import Path

from png_codec import decode_rgba_png, encode_rgba_png

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "art_source/items/skull/skull-generated.png"
OUTPUT = ROOT / "web_new/public/assets/game/items/skull.png"
SOURCE_SHA256 = "0030ebd3d13b2e8a119031dce0a24b5be687c848cb172695a04836578f9e3070"
SIZE = 32


def publish(check=False):
    if hashlib.sha256(SOURCE.read_bytes()).hexdigest() != SOURCE_SHA256:
        raise ValueError(f"generated skull source changed: {SOURCE}")
    width, height, rgba = decode_rgba_png(SOURCE)
    if width != height:
        raise ValueError("skull icon source must have a square canvas")
    pixels = bytearray(SIZE * SIZE * 4)
    for y in range(SIZE):
        source_y = int((y + .5) * height / SIZE)
        for x in range(SIZE):
            source_x = int((x + .5) * width / SIZE)
            source_offset = (source_y * width + source_x) * 4
            output_offset = (y * SIZE + x) * 4
            pixels[output_offset:output_offset + 4] = rgba[source_offset:source_offset + 4]
    alpha = pixels[3::4]
    if min(alpha) != 0 or max(alpha) < 250:
        raise ValueError("skull icon must preserve transparency and opaque bone pixels")
    published = encode_rgba_png(SIZE, SIZE, pixels)
    if check:
        if OUTPUT.read_bytes() != published:
            raise ValueError(f"icon needs export: {OUTPUT}")
    else:
        OUTPUT.parent.mkdir(parents=True, exist_ok=True)
        OUTPUT.write_bytes(published)
    print(f"{'Verified' if check else 'Exported'} {OUTPUT.relative_to(ROOT)} ({SIZE}x{SIZE})")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    publish(parser.parse_args().check)
