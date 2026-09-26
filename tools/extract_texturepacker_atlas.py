#!/usr/bin/env python3
"""Extract individual sprites from a TexturePacker JSON-hash atlas.

Uses only the Python standard library (no PIL/ImageMagick in this environment).
Supports plain, trimmed and rotated frames.

Pair tool: pack_texturepacker_atlas.py packs a directory back into an atlas;
pack -> extract must be pixel-exact (see tools/tests/).

Usage: python3 tools/extract_texturepacker_atlas.py [atlas.json] [sheet.png] [out_dir]
       [--strip-prefix P]...
Defaults are wired for the web_new tiles atlas -> art_source/tiles.
"""
import argparse
import json
import os
import sys

import png_codec as pc

DEFAULT_JSON = "web_new/public/assets/game/tiles.json"
DEFAULT_PNG = "web_new/public/assets/game/tiles.png"
DEFAULT_OUT = "art_source/tiles"
# Frame keys live in atlas groups "tiles/*" and "terrain/*"; the leading
# "tiles/" duplicates the output dir name, so it is stripped by default.
DEFAULT_STRIP_PREFIXES = ["tiles/"]


def frame_region(fr):
    """On-sheet region (x, y, w, h) of a frame.

    For rotated frames the JSON frame w/h are the sprite's ORIGINAL-orientation
    dims; the on-sheet region is the 90°-CW-rotated sprite, so w/h are swapped.
    """
    f = fr["frame"]
    if fr.get("rotated"):
        return f["x"], f["y"], f["h"], f["w"]
    return f["x"], f["y"], f["w"], f["h"]


def frame_content(fr, sheet_w, sheet):
    """Decode a frame back to its upright sprite pixels (trimmed content)."""
    x, y, w, h = frame_region(fr)
    if x + w > sheet_w or y + h > sheet_h(sheet_w, sheet):
        raise ValueError(f"frame region {x},{y},{w},{h} exceeds sheet")
    region = pc.crop(sheet_w, sheet, x, y, w, h)
    if fr.get("rotated"):
        _, _, region = pc.rotate90ccw(region, w, h)
    return region


def sheet_h(sheet_w, sheet):
    return len(sheet) // (sheet_w * 4)


def strip_prefixes(key, prefixes):
    for p in prefixes:
        if p and key.startswith(p):
            return key[len(p):]
    return key


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("atlas_json", nargs="?", default=DEFAULT_JSON)
    ap.add_argument("sheet_png", nargs="?", default=DEFAULT_PNG)
    ap.add_argument("out_dir", nargs="?", default=DEFAULT_OUT)
    ap.add_argument("--strip-prefix", action="append", default=None,
                    metavar="P", help=f"frame-key prefix to strip (repeatable; default: {DEFAULT_STRIP_PREFIXES})")
    args = ap.parse_args()
    prefixes = args.strip_prefix if args.strip_prefix is not None else DEFAULT_STRIP_PREFIXES

    with open(args.atlas_json) as f:
        frames = json.load(f)["frames"]

    sheet_w, sheet_h_, sheet = pc.decode_rgba_png(args.sheet_png)

    planned = {}
    for key, fr in frames.items():
        rel = strip_prefixes(key, prefixes)
        if rel in planned:
            raise ValueError(f"output path collision: {key} and {planned[rel]}")
        planned[rel] = fr

    for rel, fr in planned.items():
        region = frame_content(fr, sheet_w, sheet)
        if fr.get("trimmed"):
            f = fr["frame"]
            ss = fr["spriteSourceSize"]
            # trimmed+rotated: frame w/h are the trimmed dims (upright), which
            # is exactly what frame_content produced; place at ss offset.
            w, h = fr["sourceSize"]["w"], fr["sourceSize"]["h"]
            if ss["x"] + f["w"] > w or ss["y"] + f["h"] > h:
                raise ValueError(f"{rel}: spriteSourceSize {ss} does not fit sourceSize {w}x{h}")
            pixels = pc.paste_onto_canvas(w, h, f["w"], f["h"], ss["x"], ss["y"], region)
        else:
            w, h = fr["sourceSize"]["w"], fr["sourceSize"]["h"]
            if (w, h) != (fr["frame"]["w"], fr["frame"]["h"]):
                raise ValueError(f"{rel}: untrimmed frame dims != sourceSize")
            pixels = region
        path = os.path.join(args.out_dir, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as f:
            f.write(pc.encode_rgba_png(w, h, pixels))

    # Verify: every written file must parse and match its atlas region byte-for-byte.
    for rel, fr in planned.items():
        path = os.path.join(args.out_dir, rel)
        w, h, pixels = pc.decode_rgba_png(path)
        if (w, h) != (fr["sourceSize"]["w"], fr["sourceSize"]["h"]):
            raise ValueError(f"{rel}: size {w}x{h} != {fr['sourceSize']['w']}x{fr['sourceSize']['h']}")
        expected = frame_content(fr, sheet_w, sheet)
        if fr.get("trimmed"):
            ss = fr["spriteSourceSize"]
            expected = pc.paste_onto_canvas(w, h, fr["frame"]["w"], fr["frame"]["h"],
                                            ss["x"], ss["y"], expected)
        if pixels != expected:
            raise ValueError(f"{rel}: pixel mismatch vs atlas")
    print(f"extracted {len(planned)} frames to {args.out_dir}, all verified against atlas pixels")


if __name__ == "__main__":
    sys.exit(main())
