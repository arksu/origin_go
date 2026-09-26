#!/usr/bin/env python3
"""Extract individual sprites from a TexturePacker JSON-hash atlas.

Uses only the Python standard library (no PIL/ImageMagick in this environment).
Supports plain and trimmed frames; refuses rotated frames (fail fast, none in
our atlases).

Usage: python3 tools/extract_texturepacker_atlas.py [atlas.json] [sheet.png] [out_dir]
Defaults are wired for web_new tiles atlas -> art_source/tiles.
"""
import json
import os
import struct
import sys
import zlib

PNG_SIG = b"\x89PNG\r\n\x1a\n"
DEFAULT_JSON = "web_new/public/assets/game/tiles.json"
DEFAULT_PNG = "web_new/public/assets/game/tiles.png"
DEFAULT_OUT = "art_source/tiles"
# Frame keys live in atlas groups "tiles/*" and "terrain/*"; the leading
# "tiles/" duplicates the output dir name, so it is stripped.
STRIP_PREFIX = "tiles/"


def decode_rgba_png(path):
    """Decode an 8-bit non-interlaced RGBA PNG into (width, height, flat rgba bytes)."""
    with open(path, "rb") as f:
        data = f.read()
    if not data.startswith(PNG_SIG):
        raise ValueError(f"{path}: not a PNG file")
    pos, idat, header = 8, [], None
    while pos < len(data):
        length, ctype = struct.unpack(">I4s", data[pos:pos + 8])
        chunk = data[pos + 8:pos + 8 + length]
        if ctype == b"IHDR":
            header = struct.unpack(">IIBBBBB", chunk)
        elif ctype == b"IDAT":
            idat.append(chunk)
        elif ctype == b"IEND":
            break
        pos += 12 + length
    w, h, depth, ctype_, comp, filt, interlace = header
    if depth != 8 or ctype_ != 6 or interlace != 0:
        raise ValueError(f"{path}: unsupported PNG (depth={depth}, color={ctype_}, interlace={interlace})")
    raw = zlib.decompress(b"".join(idat))
    stride = w * 4
    out = bytearray(w * h * 4)
    prev = bytearray(stride)
    src = 0
    for y in range(h):
        ftype = raw[src]
        src += 1
        row = bytearray(raw[src:src + stride])
        src += stride
        _unfilter_row(row, prev, ftype, 4)
        out[y * stride:(y + 1) * stride] = row
        prev = row
    return w, h, out


def _unfilter_row(row, prev, ftype, bpp):
    if ftype == 0:
        return
    if ftype == 1:  # Sub
        for i in range(bpp, len(row)):
            row[i] = (row[i] + row[i - bpp]) & 0xFF
    elif ftype == 2:  # Up
        for i in range(len(row)):
            row[i] = (row[i] + prev[i]) & 0xFF
    elif ftype == 3:  # Average
        for i in range(len(row)):
            left = row[i - bpp] if i >= bpp else 0
            row[i] = (row[i] + ((left + prev[i]) >> 1)) & 0xFF
    elif ftype == 4:  # Paeth
        for i in range(len(row)):
            a = row[i - bpp] if i >= bpp else 0
            b = prev[i]
            c = prev[i - bpp] if i >= bpp else 0
            p = a + b - c
            pa, pb, pc = abs(p - a), abs(p - b), abs(p - c)
            pred = a if (pa <= pb and pa <= pc) else (b if pb <= pc else c)
            row[i] = (row[i] + pred) & 0xFF
    else:
        raise ValueError(f"unknown PNG filter type {ftype}")


def encode_rgba_png(width, height, rgba):
    def chunk(ctype, payload):
        return struct.pack(">I", len(payload)) + ctype + payload + struct.pack(">I", zlib.crc32(ctype + payload))

    ihdr = struct.pack(">IIBBBBB", width, height, 8, 6, 0, 0, 0)
    stride = width * 4
    raw = b"".join(b"\x00" + bytes(rgba[y * stride:(y + 1) * stride]) for y in range(height))
    return PNG_SIG + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")


def crop(sheet_w, sheet, x, y, w, h):
    stride = sheet_w * 4
    out = bytearray(w * h * 4)
    for row in range(h):
        src = (y + row) * stride + x * 4
        out[row * w * 4:(row + 1) * w * 4] = sheet[src:src + w * 4]
    return out


def paste_onto_canvas(w, h, spr_w, spr_h, off_x, off_y, pixels):
    canvas = bytearray(w * h * 4)
    for row in range(spr_h):
        dst = ((off_y + row) * w + off_x) * 4
        canvas[dst:dst + spr_w * 4] = pixels[row * spr_w * 4:(row + 1) * spr_w * 4]
    return canvas


def main():
    json_path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT_JSON
    png_path = sys.argv[2] if len(sys.argv) > 2 else DEFAULT_PNG
    out_dir = sys.argv[3] if len(sys.argv) > 3 else DEFAULT_OUT

    with open(json_path) as f:
        frames = json.load(f)["frames"]

    sheet_w, sheet_h, sheet = decode_rgba_png(png_path)

    planned = {}
    for key, fr in frames.items():
        rel = key[len(STRIP_PREFIX):] if key.startswith(STRIP_PREFIX) else key
        if rel in planned:
            raise ValueError(f"output path collision: {key} and {planned[rel]}")
        if fr.get("rotated"):
            raise ValueError(f"{key}: rotated frames are not supported")
        planned[rel] = fr

    for rel, fr in planned.items():
        f = fr["frame"]
        pixels = crop(sheet_w, sheet, f["x"], f["y"], f["w"], f["h"])
        if fr.get("trimmed"):
            ss = fr["spriteSourceSize"]
            pixels = paste_onto_canvas(fr["sourceSize"]["w"], fr["sourceSize"]["h"],
                                       f["w"], f["h"], ss["x"], ss["y"], pixels)
            w, h = fr["sourceSize"]["w"], fr["sourceSize"]["h"]
        else:
            w, h = f["w"], f["h"]
        if f["x"] + f["w"] > sheet_w or f["y"] + f["h"] > sheet_h:
            raise ValueError(f"{rel}: frame {f} exceeds sheet {sheet_w}x{sheet_h}")
        path = os.path.join(out_dir, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "wb") as f:
            f.write(encode_rgba_png(w, h, pixels))

    # Verify: every written file must parse and match its source region byte-for-byte.
    for rel, fr in planned.items():
        path = os.path.join(out_dir, rel)
        w, h, pixels = decode_rgba_png(path)
        expected_w, expected_h = fr["sourceSize"]["w"], fr["sourceSize"]["h"]
        if (w, h) != (expected_w, expected_h):
            raise ValueError(f"{rel}: size {w}x{h} != {expected_w}x{expected_h}")
        f = fr["frame"]
        if not fr.get("trimmed"):
            if pixels != crop(sheet_w, sheet, f["x"], f["y"], f["w"], f["h"]):
                raise ValueError(f"{rel}: pixel mismatch vs atlas")
    print(f"extracted {len(planned)} frames to {out_dir}, all verified against atlas pixels")


if __name__ == "__main__":
    main()
