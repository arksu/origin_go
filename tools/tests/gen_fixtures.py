#!/usr/bin/env python3
"""Generate deterministic fixture sprites covering every atlas feature.

Usage: python3 tools/tests/gen_fixtures.py <out_dir>
"""
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
import png_codec as pc  # noqa: E402


def write(out_dir, name, w, h, painter):
    px = bytearray(w * h * 4)
    painter(px, w, h)
    path = os.path.join(out_dir, name)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(pc.encode_rgba_png(w, h, px))


def opaque(px, w, h):
    for i in range(0, len(px), 4):
        px[i:i + 4] = bytes([180, 40, 40, 255])


def bordered(px, w, h):
    for y in range(3, h - 3):
        for x in range(3, w - 3):
            i = (y * w + x) * 4
            px[i:i + 4] = bytes([10, 200, 60, 255])


def dot(px, w, h):
    px[0:4] = bytes([255, 255, 0, 255])


def gradient(px, w, h):
    for y in range(h):
        for x in range(w):
            i = (y * w + x) * 4
            px[i:i + 4] = bytes([(x * 7) % 256, (y * 11) % 256, 90, 255])


def diagonal_hole(px, w, h):
    for y in range(h):
        for x in range(w):
            if (x + y) % 2 == 0 and not (5 <= x < 8 and 5 <= y < 8):
                i = (y * w + x) * 4
                px[i:i + 4] = bytes([30, 30, 220, 255])


def tall_strip(px, w, h):
    for y in range(h):
        for x in range(w):
            i = (y * w + x) * 4
            px[i:i + 4] = bytes([x * 3 % 256, 120, y * 5 % 256, 255])


def stripe(px, w, h):
    # thin off-center stripe -> gets BOTH trimmed and rotated by the packer
    for y in range(2, h - 2):
        for x in range(10, w - 10):
            i = (y * w + x) * 4
            px[i:i + 4] = bytes([220, 160, 30, 255])


def main():
    out = sys.argv[1] if len(sys.argv) > 1 else "tools/tests/fixtures"
    # name, size, painter — names exercise key grouping too
    write(out, "plain_square.png", 48, 48, opaque)
    write(out, "trim_border.png", 50, 40, bordered)
    write(out, "trim_non_square.png", 61, 23, gradient)
    write(out, "one_pixel.png", 1, 1, dot)
    write(out, "hole_trim.png", 33, 31, diagonal_hole)
    write(out, "tall.png", 19, 71, tall_strip)
    write(out, "wide.png", 71, 19, tall_strip)
    write(out, "stripe.png", 61, 11, stripe)
    write(out, "seq/walk.0001.png", 16, 16, opaque)
    write(out, "seq/walk.0002.png", 16, 17, gradient)
    write(out, "keep/ terrain+x.png", 8, 8, dot)  # exotic name must survive
    print(f"fixtures written to {out}")


if __name__ == "__main__":
    main()
