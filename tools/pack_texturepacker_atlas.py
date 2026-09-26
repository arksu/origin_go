#!/usr/bin/env python3
"""Pack a directory of PNGs into a TexturePacker JSON-hash atlas for PixiJS v8.

Stdlib only. Frame semantics match the Pixi v8 Spritesheet parser
(node_modules/pixi.js/lib/spritesheet/Spritesheet.mjs) and renderer
(Texture.updateUvs + TextureMatrix + updateQuadBounds):

- `rotated: true` -> texture rotate 2; the on-sheet region is the sprite
  rotated 90° CLOCKWISE; JSON frame w/h stay in the sprite's ORIGINAL
  orientation (the parser swaps them to get the on-sheet region);
- `trimmed` frames keep spriteSourceSize in original sprite coordinates;
- full field set is always emitted (trimmed:false included);
- no `aliases`: duplicate images are packed as separate frames (the Pixi
  parser does not read TexturePacker aliases).

Pair tool: extract_texturepacker_atlas.py — pack -> extract must roundtrip
pixel-exact (tools/tests/).

Usage: python3 tools/pack_texturepacker_atlas.py <in_dir> <out_json> <out_png> [flags]
"""
import argparse
import json
import os
import re
import sys
import time

import png_codec as pc

ANIM_SEQ_RE = re.compile(r"^(.+)\.(\d+)$")


def fail(msg):
    raise SystemExit(f"pack_texturepacker_atlas: error: {msg}")


# ---------------------------------------------------------------------------
# Input scan
# ---------------------------------------------------------------------------

def scan_sprites(in_dir, key_prefix, key_excludes):
    """Collect (key, path) for all PNGs; dotfiles skipped, non-PNG = error."""
    if not os.path.isdir(in_dir):
        fail(f"input directory not found: {in_dir}")
    entries = []
    for root, dirs, files in os.walk(in_dir):
        dirs[:] = sorted(d for d in dirs if not d.startswith("."))
        for name in sorted(files):
            if name.startswith("."):
                continue
            path = os.path.join(root, name)
            rel = os.path.relpath(path, in_dir).replace(os.sep, "/")
            if not name.lower().endswith(".png"):
                fail(f"non-PNG input (PNG-only tool): {rel}")
            key = rel
            if key_prefix:
                if not any(rel.startswith(e) for e in key_excludes):
                    key = key_prefix + rel
            entries.append((key, path))
    if not entries:
        fail(f"no PNG files under {in_dir}")
    seen = {}
    for key, path in entries:
        if key in seen:
            fail(f"key collision: {key} from {seen[key]} and {path}")
        seen[key] = path
    return entries


# ---------------------------------------------------------------------------
# Trim
# ---------------------------------------------------------------------------

def trim_bbox(pixels, w, h, alpha_threshold):
    """Bounding box of pixels with alpha >= threshold. None if empty."""
    min_x, min_y, max_x, max_y = w, h, -1, -1
    for y in range(h):
        row = y * w * 4
        for x in range(w):
            if pixels[row + x * 4 + 3] >= alpha_threshold:
                if x < min_x:
                    min_x = x
                if x > max_x:
                    max_x = x
                if y < min_y:
                    min_y = y
                if y > max_y:
                    max_y = y
    if max_x < 0:
        return None
    return min_x, min_y, max_x - min_x + 1, max_y - min_y + 1


# ---------------------------------------------------------------------------
# Layout algorithms. Entries are (key, w, h) of the content rects (already
# inflated by padding); both return [(key, x, y, rotated)] with x,y the
# top-left of the placed rect (post-rotation dims), or None on overflow.
# ---------------------------------------------------------------------------

def _sort_key(e):
    key, w, h = e
    return (-max(w, h), -min(w, h), key)


def _orientations(w, h, allow_rotate):
    yield False, (w, h)
    if allow_rotate and w != h:
        yield True, (h, w)


def _contains(small, big):
    return (small[0] >= big[0] and small[1] >= big[1]
            and small[0] + small[2] <= big[0] + big[2]
            and small[1] + small[3] <= big[1] + big[3])


def pack_maxrects(entries, W, H, allow_rotate):
    """MaxRects with Best-Short-Side-Fit. Free rects kept as an antichain:
    after each placement all rects intersecting the placed rect are split and
    containment is pruned (stale contained rects would allow overlapping
    placements)."""
    free = [(0, 0, W, H)]
    placements = []
    for key, w, h in sorted(entries, key=_sort_key):
        best = None
        for fx, fy, fw, fh in free:
            if fw * fh < w * h:
                continue
            for rot, (rw, rh) in _orientations(w, h, allow_rotate):
                if rw <= fw and rh <= fh:
                    dh, dv = fw - rw, fh - rh
                    short = dh if dh < dv else dv
                    long_ = dv if short == dh else dh
                    cand = (short, long_, fy, fx, rot)
                    if best is None or cand < best[0]:
                        best = (cand, fx, fy, rot)
        if best is None:
            return None
        _, px, py, rot = best
        rw, rh = (h, w) if rot else (w, h)
        placements.append((key, px, py, rot))
        placed = (px, py, rw, rh)

        survivors, pieces = [], []
        for fr in free:
            if not pc.rects_overlap(fr, placed):
                survivors.append(fr)
                continue
            fx, fy, fw, fh = fr
            rx, ry = placed[0], placed[1]
            re, be = rx + rw, ry + rh
            if rx > fx:
                pieces.append((fx, fy, rx - fx, fh))
            if re < fx + fw:
                pieces.append((re, fy, fx + fw - re, fh))
            if ry > fy:
                pieces.append((fx, fy, fw, ry - fy))
            if be < fy + fh:
                pieces.append((fx, be, fw, fy + fh - be))
        # dedupe exact duplicates from adjacent splits
        pieces = list(dict.fromkeys(pieces))
        # prune: new pieces vs survivors and vs each other (both directions)
        pieces.sort(key=lambda r: (-r[2] * r[3], r))
        kept = []
        for p in pieces:
            if any(_contains(p, q) for q in kept) or any(_contains(p, s) for s in survivors):
                continue
            kept.append(p)
        survivors = [s for s in survivors if not any(_contains(s, p) for p in pieces)]
        free = survivors + kept
    return placements


def pack_skyline(entries, W, H, allow_rotate):
    """Bottom-Left Skyline. Deterministic; O(n * segments * orientations)."""
    skyline = [(0, 0, W)]  # (x, y, width)
    placements = []
    for key, w, h in sorted(entries, key=_sort_key):
        best = None
        for rot, (rw, rh) in _orientations(w, h, allow_rotate):
            if rw > W:
                continue
            for i in range(len(skyline)):
                x0 = skyline[i][0]
                if x0 + rw > W:
                    break
                top, span, j = 0, 0, i
                while span < rw and j < len(skyline):
                    sx, sy, sw = skyline[j]
                    if sy > top:
                        top = sy
                    span += sw
                    j += 1
                if span < rw or top + rh > H:
                    continue
                cand = (top + rh, x0, rot, i, top)
                if best is None or cand < best:
                    best = cand
        if best is None:
            return None
        _, px, rot, _, py = best
        placements.append((key, px, py, rot))
        rw, rh = (h, w) if rot else (w, h)
        # rebuild skyline around the placed rect
        new = []
        for sx, sy, sw in skyline:
            if sx + sw <= px or sx >= px + rw:
                new.append((sx, sy, sw))
                continue
            if sx < px:
                new.append((sx, sy, px - sx))
            if sx + sw > px + rw:
                new.append((px + rw, sy, sx + sw - (px + rw)))
        new.append((px, py + rh, rw))
        new.sort()
        merged = []
        for seg in new:
            if merged and merged[-1][0] + merged[-1][2] == seg[0] and merged[-1][1] == seg[1]:
                x0, y0, w0 = merged[-1]
                merged[-1] = (x0, y0, w0 + seg[2])
            else:
                merged.append(seg)
        skyline = merged
    return placements


ALGORITHMS = {"maxrects": pack_maxrects, "skyline": pack_skyline}


def next_pow2(n):
    p = 1
    while p < n:
        p *= 2
    return p


def layout(entries, allow_rotate, algorithm, max_size, pad, border):
    """Try ascending PoT square sheet sizes. Returns (size, placements).

    placements are (key, x, y, rotated) with x,y the top-left of the on-sheet
    CONTENT region (padding/border already subtracted).
    """
    pack = ALGORITHMS[algorithm]
    inflated = [(key, w + 2 * pad, h + 2 * pad) for key, w, h in entries]
    largest = max(max(w, h) for _, w, h in inflated)
    size = max(64, next_pow2(largest))
    while size <= max_size:
        if size - 2 * border >= largest:
            placements = pack(inflated, size - 2 * border, size - 2 * border, allow_rotate)
            if placements is not None:
                shifted = [(k, x + border + pad, y + border + pad, r) for k, x, y, r in placements]
                return size, shifted
        size *= 2
    total_area = sum(w * h for _, w, h in entries)
    rotate_hint = "" if allow_rotate else " (rotation is disabled by --no-rotate)"
    fail(f"sprites do not fit in {max_size}x{max_size}{rotate_hint}: "
         f"content area {total_area}px, largest sprite {largest}px incl. padding; "
         f"try --max-size 4096")


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------

def build_frames_json(entries, placements):
    """entries: key -> dict(orig_w, orig_h, off_x, off_y, content(w,h,pixels))."""
    frames = {}
    stats = {"rotated": 0, "trimmed": 0}
    for key, x, y, rotated in placements:
        e = entries[key]
        w, h = e["w"], e["h"]
        frames[key] = {
            "frame": {"x": x, "y": y, "w": w, "h": h},
            "rotated": rotated,
            "trimmed": e["trimmed"],
            "spriteSourceSize": {"x": e["off_x"], "y": e["off_y"], "w": w, "h": h},
            "sourceSize": {"w": e["orig_w"], "h": e["orig_h"]},
        }
        stats["rotated"] += rotated
        stats["trimmed"] += e["trimmed"]
    return frames, stats


def build_animations(keys):
    groups = {}
    for key in keys:
        base = key[:-4] if key.lower().endswith(".png") else key
        m = ANIM_SEQ_RE.match(base)
        if m:
            groups.setdefault(m.group(1), []).append((int(m.group(2)), key))
    return {name: [k for _, k in sorted(vals)] for name, vals in sorted(groups.items())}


def verify(entries, placements, sheet_w, sheet):
    """Overlap check + per-frame pixel reconstruction vs source content."""
    regions = []
    for key, x, y, rotated in placements:
        e = entries[key]
        w, h = (e["h"], e["w"]) if rotated else (e["w"], e["h"])
        regions.append((key, x, y, w, h))
    regions.sort(key=lambda r: (r[1], r[2]))
    for i in range(len(regions)):
        k1, x1, y1, w1, h1 = regions[i]
        for j in range(i + 1, len(regions)):
            k2, x2, y2, w2, h2 = regions[j]
            if x2 >= x1 + w1:
                break
            if pc.rects_overlap((x1, y1, w1, h1), (x2, y2, w2, h2)):
                fail(f"layout overlap: {k1} and {k2}")
    for key, x, y, rotated in placements:
        e = entries[key]
        w, h = (e["h"], e["w"]) if rotated else (e["w"], e["h"])
        region = pc.crop(sheet_w, sheet, x, y, w, h)
        if rotated:
            _, _, region = pc.rotate90ccw(region, w, h)
        if region != e["pixels"]:
            fail(f"self-verify: pixels of {key} corrupted in sheet")


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("in_dir")
    ap.add_argument("out_json")
    ap.add_argument("out_png")
    ap.add_argument("--algorithm", choices=sorted(ALGORITHMS), default="maxrects")
    ap.add_argument("--max-size", type=int, default=2048)
    ap.add_argument("--padding", type=int, default=0, help="spacing between sprites")
    ap.add_argument("--border-padding", type=int, default=0, help="empty margin around the sheet")
    ap.add_argument("--no-rotate", action="store_true", help="disable 90° rotation when packing")
    ap.add_argument("--no-trim", action="store_true", help="disable trimming")
    ap.add_argument("--alpha-threshold", type=int, default=1,
                    help="alpha >= this counts as opaque for trimming (default 1)")
    ap.add_argument("--scale", type=int, default=1,
                    help="written to meta.scale (no resampling is done)")
    ap.add_argument("--key-prefix", default="", help="prefix added to frame keys (e.g. tiles/)")
    ap.add_argument("--key-exclude", default="",
                    help="comma-separated prefixes excluded from --key-prefix (e.g. terrain)")
    ap.add_argument("--animations-from-sequences", action="store_true",
                    help="group name.NNN.png files into animations entries")
    args = ap.parse_args()
    if args.scale != 1:
        print("warning: --scale != 1 changes meta.scale (rendered sprite size) "
              "but no resampling is performed")
    t0 = time.time()

    # scan + decode + trim
    files = scan_sprites(args.in_dir, args.key_prefix, [e for e in args.key_exclude.split(",") if e])
    entries = {}
    for key, path in files:
        ow, oh, pixels = pc.decode_rgba_png(path)
        box = (0, 0, ow, oh) if args.no_trim else trim_bbox(pixels, ow, oh, args.alpha_threshold)
        if box is None:
            fail(f"fully transparent sprite: {key}")
        bx, by, bw, bh = box
        trimmed = (bw, bh) != (ow, oh)
        content = pc.crop(ow, pixels, bx, by, bw, bh) if trimmed else pixels
        entries[key] = {
            "orig_w": ow, "orig_h": oh, "off_x": bx, "off_y": by,
            "w": bw, "h": bh, "pixels": content, "trimmed": trimmed,
        }
    print(f"scanned {len(entries)} sprites "
          f"(trimmed {sum(1 for e in entries.values() if e['trimmed'])}) in {time.time()-t0:.1f}s")

    size, placements = layout([(k, e["w"], e["h"]) for k, e in entries.items()],
                              not args.no_rotate, args.algorithm, args.max_size,
                              args.padding, args.border_padding)

    # render sheet
    sheet = bytearray(size * size * 4)
    for key, x, y, rotated in placements:
        e = entries[key]
        if rotated:
            rw, rh, pixels = pc.rotate90cw(e["pixels"], e["w"], e["h"])
        else:
            rw, rh, pixels = e["w"], e["h"], e["pixels"]
        pc.blit_rect(sheet, size, x, y, pixels, rw, rh)

    verify(entries, placements, size, sheet)

    frames, stats = build_frames_json(entries, placements)
    atlas = {"frames": frames}
    if args.animations_from_sequences:
        atlas["animations"] = build_animations(frames.keys())
    atlas["meta"] = {
        "app": "pack_texturepacker_atlas.py",
        "version": "1.0",
        "image": os.path.basename(args.out_png),
        "format": "RGBA8888",
        "size": {"w": size, "h": size},
        "scale": str(args.scale),
    }
    out_png = os.path.abspath(args.out_png)
    out_json = os.path.abspath(args.out_json)
    os.makedirs(os.path.dirname(out_png), exist_ok=True)
    os.makedirs(os.path.dirname(out_json), exist_ok=True)
    with open(out_png, "wb") as f:
        f.write(pc.encode_rgba_png(size, size, sheet))
    with open(out_json, "w") as f:
        json.dump(atlas, f, ensure_ascii=False, indent=1)

    used = sum((e["h"] if r else e["w"]) * (e["w"] if r else e["h"])
               for k, x, y, r in placements for e in [entries[k]])
    print(f"packed {len(placements)} frames into {size}x{size} {args.algorithm} "
          f"(rotated {stats['rotated']}, trimmed {stats['trimmed']}, fill {used/(size*size):.1%}) "
          f"in {time.time()-t0:.1f}s -> {out_json}, {out_png}")


if __name__ == "__main__":
    sys.exit(main())
