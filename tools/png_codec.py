"""Shared stdlib-only PNG codec and pixel primitives for the atlas tools.

No PIL/ImageMagick on this machine; everything here is pure Python.
Only 8-bit non-interlaced RGBA is supported (fail fast otherwise).
"""
import struct
import zlib

PNG_SIG = b"\x89PNG\r\n\x1a\n"


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
    w, h, depth, ctype_, _comp, _filt, interlace = header
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


def crop(width, pixels, x, y, w, h):
    """Extract a w×h region from a flat RGBA image of the given width."""
    stride = width * 4
    out = bytearray(w * h * 4)
    for row in range(h):
        src = (y + row) * stride + x * 4
        out[row * w * 4:(row + 1) * w * 4] = pixels[src:src + w * 4]
    return out


def paste_onto_canvas(w, h, spr_w, spr_h, off_x, off_y, pixels):
    """Center a spr_w×spr_h image onto a w×h transparent canvas at (off_x, off_y)."""
    canvas = bytearray(w * h * 4)
    for row in range(spr_h):
        dst = ((off_y + row) * w + off_x) * 4
        canvas[dst:dst + spr_w * 4] = pixels[row * spr_w * 4:(row + 1) * spr_w * 4]
    return canvas


def blit_rect(dst, dst_w, x, y, src, w, h):
    """Blit a w×h flat RGBA buffer onto dst (dst_w wide) at (x, y)."""
    for row in range(h):
        doff = ((y + row) * dst_w + x) * 4
        dst[doff:doff + w * 4] = src[row * w * 4:(row + 1) * w * 4]


def rotate90cw(pixels, w, h):
    """Rotate a w×h flat RGBA image 90° clockwise. Returns (h, w, rotated).

    new(x', y') = old(y', h - 1 - x') — matches the Pixi v8 `rotate: 2`
    spritesheet convention (region on sheet = sprite rotated 90° CW).
    """
    out = bytearray(w * h * 4)
    for y in range(h):
        rowbase = y * w * 4
        # column x' = h-1-y in the result, read top-down
        for x in range(w):
            doff = (x * h + (h - 1 - y)) * 4
            out[doff:doff + 4] = pixels[rowbase + x * 4:rowbase + x * 4 + 4]
    return h, w, out


def rotate90ccw(pixels, w, h):
    """Rotate a w×h flat RGBA image 90° counter-clockwise. Returns (h, w, rotated).

    new(x', y') = old(w - 1 - y', x') — exact inverse of rotate90cw.
    """
    out = bytearray(w * h * 4)
    for y in range(h):
        rowbase = y * w * 4
        for x in range(w):
            doff = ((w - 1 - x) * h + y) * 4
            out[doff:doff + 4] = pixels[rowbase + x * 4:rowbase + x * 4 + 4]
    return h, w, out


def rects_overlap(a, b):
    """a, b are (x, y, w, h) on-sheet content regions."""
    ax, ay, aw, ah = a
    bx, by, bw, bh = b
    return ax < bx + bw and bx < ax + aw and ay < by + bh and by < ay + ah
