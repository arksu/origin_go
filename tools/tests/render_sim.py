#!/usr/bin/env python3
"""Offline simulation of the client tile renderer.

Renders the same small shore map twice —
  A: HEAD atlas + pre-fix mesher (quad = full tile, uv = frame)
  B: current atlas + fixed mesher (quad = trim rect, uv = uvs corners)
— and reports pixel differences. JS number semantics for getRandomByCoord
are emulated with doubles + fmod.
"""
import json
import math
import os
import subprocess
import sys
import tempfile

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), ".."))
import png_codec as pc  # noqa: E402

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
TILE_W, TILE_H = 63, 32
TW2, TH2 = 63 // 2, 32 // 2
BX, BY = [0, 1, 2, 1], [1, 0, 1, 2]
CX, CY = [0, 0, 2, 2], [0, 2, 2, 0]
IDS = {"water_deep": 1, "water": 3, "grass": 35, "sand": 68}


def rnd(x, y, z=None, s=None):
    seed = float(s if s is not None else x)
    seed = math.fmod(seed * 1103515245.0 + 12345.0, 2147483647.0)
    seed *= (y + x)
    seed = math.fmod(seed * 1103515245.0 + 12345.0, 2147483647.0)
    seed = math.fmod(seed * 1103515245.0 + 12345.0, 2147483647.0)
    if z is not None:
        seed *= z
        seed = math.fmod(seed * 1103515245.0 + 12345.0, 2147483647.0)
        seed = math.fmod(seed * 1103515245.0 + 12345.0, 2147483647.0)
    return seed


def pick(variants, x, y):
    total = sum(v["w"] for v in variants)
    if total == 0:
        return None
    w = math.fmod(rnd(x, y), float(total))
    for v in variants:
        w -= v["w"]
        if w < 0:
            return v["img"]
    return variants[0]["img"]


class Atlas:
    """Frame canvases for one atlas: key -> (w, h, rgba, trim or None)."""

    def __init__(self, json_path, png_path, strip=False):
        frames = json.load(open(json_path))["frames"]
        sw, sh, sheet = pc.decode_rgba_png(png_path)
        self.canvases = {}
        for key, fr in frames.items():
            k = key[6:] if strip and key.startswith("tiles/") else key
            f = fr["frame"]
            if fr.get("rotated"):
                region = pc.crop(sw, sheet, f["x"], f["y"], f["h"], f["w"])
                _, _, region = pc.rotate90ccw(region, f["h"], f["w"])
            else:
                region = pc.crop(sw, sheet, f["x"], f["y"], f["w"], f["h"])
            ow, oh = fr["sourceSize"]["w"], fr["sourceSize"]["h"]
            if fr.get("trimmed"):
                ss = fr["spriteSourceSize"]
                canvas = pc.paste_onto_canvas(ow, oh, f["w"], f["h"], ss["x"], ss["y"], region)
                trim = (ss["x"], ss["y"], f["w"], f["h"])
            else:
                canvas = region
                trim = None
            self.canvases[k] = (ow, oh, canvas, trim)

    def frame(self, key):
        return self.canvases[key]


def blend_at(img, di, canvas, si):
    """alpha-composite canvas pixel at si onto img pixel at di, in place."""
    a = canvas[si + 3]
    if a == 0:
        return
    if a == 255:
        img[di:di + 4] = canvas[si:si + 4]
        return
    ia = 255 - a
    for c in range(3):
        img[di + c] = (canvas[si + c] * a + img[di + c] * ia) // 255
    img[di + 3] = max(img[di + 3], a)


def stretch_nearest(canvas, ow, oh, dw, dh):
    """Nearest-neighbor resample — what the GPU does when a region is mapped
    onto a differently-sized quad."""
    out = bytearray(dw * dh * 4)
    for y in range(dh):
        srow = min(oh - 1, y * oh // dh)
        for x in range(dw):
            scol = min(ow - 1, x * ow // dw)
            out[(y * dw + x) * 4:(y * dw + x) * 4 + 4] = \
                canvas[(srow * ow + scol) * 4:(srow * ow + scol) * 4 + 4]
    return out


def render(map_w, map_h, tiles, tileset_cfg, atlas, mode):
    """mode 'old': quad = full tile. mode 'new': quad = trim rect."""
    W = (map_w + map_h) * TW2 + TILE_W
    H = (map_w + map_h) * TH2 + TILE_H
    img = bytearray(W * H * 4)
    registered = sorted(IDS.values())
    for ty in range(map_h):
        for tx in range(map_w):
            ttype = tiles[ty * map_w + tx]
            gx, gy = tx, ty
            sx = tx * TW2 - ty * TW2
            sy = tx * TH2 + ty * TH2
            cfg = tileset_cfg[ttype]
            name = pick(cfg["ground"], gx, gy)
            draws = [name]
            below = [i for i in registered if i < ttype]
            for i in below:
                icfg = tileset_cfg[i]
                tr = [[-1] * 3 for _ in range(3)]
                for dx in (-1, 0, 1):
                    for dy in (-1, 0, 1):
                        nx, ny = gx + dx, gy + dy
                        tr[dx + 1][dy + 1] = tiles[ny * map_w + nx] if 0 <= nx < map_w and 0 <= ny < map_h else -1
                if tr[0][0] >= tr[1][0]: tr[0][0] = -1
                if tr[0][0] >= tr[0][1]: tr[0][0] = -1
                if tr[2][0] >= tr[1][0]: tr[2][0] = -1
                if tr[2][0] >= tr[2][1]: tr[2][0] = -1
                if tr[0][2] >= tr[0][1]: tr[0][2] = -1
                if tr[0][2] >= tr[1][2]: tr[0][2] = -1
                if tr[2][2] >= tr[2][1]: tr[2][2] = -1
                if tr[2][2] >= tr[1][2]: tr[2][2] = -1
                border_mask = 0
                corner_mask = 0
                for o in range(4):
                    if tr[BX[o]][BY[o]] == i:
                        border_mask |= 1 << o
                    if tr[CX[o]][CY[o]] == i:
                        corner_mask |= 1 << o
                if border_mask:
                    arr = icfg["borders"][border_mask - 1]
                    if arr:
                        draws.append(pick(arr, gx, gy))
                if corner_mask:
                    arr = icfg["corners"][corner_mask - 1]
                    if arr:
                        draws.append(pick(arr, gx, gy))
            for name in draws:
                if not name:
                    continue
                ow, oh, canvas, trim = atlas.frame(name)
                if mode == "new" or trim is None:
                    # fixed mesher: quad = trim rect, 1:1 sampling
                    qx, qy = sx, sy
                    src_x = src_y = 0
                elif mode == "old":
                    # HEAD atlas: untrimmed frames, quad = full tile
                    qx, qy = sx, sy
                    src_x = src_y = 0
                else:
                    # pre-fix mesher on repacked atlas: trimmed region
                    # stretched over the full-tile quad
                    canvas = stretch_nearest(canvas, ow, oh, TILE_W, TILE_H)
                    ow, oh = TILE_W, TILE_H
                    qx, qy = sx, sy
                    src_x = src_y = 0
                for row in range(oh):
                    py = qy + row - src_y
                    if not (0 <= py < H):
                        continue
                    for col in range(ow):
                        px = qx + col - src_x
                        if 0 <= px < W:
                            blend_at(img, (py * W + px) * 4, canvas, (row * ow + col) * 4)
    return W, H, img


def main():
    tmp = tempfile.mkdtemp(prefix="tiles-sim-")
    head_png = os.path.join(tmp, "tiles_head.png")
    head_json = os.path.join(tmp, "tiles_head.json")
    open(head_png, "wb").write(subprocess.run(
        ["git", "show", "HEAD:web_new/public/assets/game/tiles.png"],
        capture_output=True, cwd=ROOT).stdout)
    open(head_json, "wb").write(subprocess.run(
        ["git", "show", "HEAD:web_new/public/assets/game/tiles.json"],
        capture_output=True, cwd=ROOT).stdout)

    cfg_dir = os.path.join(ROOT, "web_new/src/game/tiles/configs")
    name_by_id = {v: k for k, v in IDS.items()}
    cfg = {t: json.load(open(os.path.join(cfg_dir, f"{name_by_id[t]}.json"))) for t in IDS.values()}

    map_w, map_h = 26, 26
    tiles = bytearray(map_w * map_h)
    for y in range(map_h):
        for x in range(map_w):
            d = x - y  # shore diagonal in iso space
            tiles[y * map_w + x] = IDS["water"] if d < 2 else (IDS["sand"] if d < 5 else IDS["grass"])

    old = Atlas(head_json, head_png)
    new = Atlas(os.path.join(ROOT, "web_new/public/assets/game/tiles.json"),
                os.path.join(ROOT, "web_new/public/assets/game/tiles.png"))

    WA, HA, A = render(map_w, map_h, tiles, cfg, old, "head")
    WB, HB, B = render(map_w, map_h, tiles, cfg, new, "new")
    WC, HC, C = render(map_w, map_h, tiles, cfg, new, "prefix")
    assert (WA, HA) == (WB, HB) == (WC, HC)

    diff = sum(1 for i in range(0, len(A), 4) if A[i:i + 4] != B[i:i + 4])
    print(f"canvas {WA}x{HA}, HEAD vs NEW+fixed differing pixels: {diff}")
    row_h = HA * 3 + 16
    side = bytearray(WA * row_h * 4)
    for idx, src in enumerate((A, B, C)):
        for row in range(HA):
            s = row * WA * 4
            d = (idx * (HA + 8) + row) * WA * 4
            side[d:d + WA * 4] = src[s:s + WA * 4]
    out = os.path.join(tmp, "side_by_side.png")
    open(out, "wb").write(pc.encode_rgba_png(WA, row_h, side))
    print("rows: 1 = HEAD atlas (pre-repack), 2 = repacked + FIXED mesher, 3 = repacked + PRE-FIX mesher")
    print(out)


if __name__ == "__main__":
    main()
