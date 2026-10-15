#!/usr/bin/env python3
"""Check curved shore contracts and measure ledges in the exported PNGs.

Run normally for contract checks, or use ``--report --source originals`` to
measure the retained baseline. ``--source candidate`` measures the revision
outputs; ``--source published`` measures the runtime atlas source sprites.
Contour measurements use native source pixels, never the analytic coverage
function, so an interior ledge cannot pass solely through matching tile joins.
"""
import argparse
import hashlib
import itertools
import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import png_codec as pc
from export_water_shore_sprites import ROOT, tile_coordinates

V3 = ROOT / "art_source/terrain_revisions/water_shore_v3"
LAYOUT = json.loads((V3 / "layout.json").read_text())
THRESHOLDS = (32, 64, 100, 128, 180, 240)
JOIN_BAND = 0.05
INTERIOR_MIN = 0.16
TARGET_MASKS = {("b", mask) for mask in (3, 6, 9, 12)} | {
    ("c", mask) for mask in (1, 2, 4, 8)
}

# Native-pixel budgets for the reviewed free shore contour at six opacity
# levels. Broad ellipse crests and the almost-opaque core may have longer runs;
# the former 20-29px shelves in the visible shore band must not return.
CONTOUR_RUN_LIMITS = {
    ("b", 3): ("horizontal", (10, 10, 9, 12, 12, 12)),
    ("b", 6): ("vertical", (5, 6, 6, 6, 6, 8)),
    ("b", 9): ("vertical", (4, 4, 6, 9, 8, 8)),
    ("b", 12): ("horizontal", (10, 10, 12, 15, 12, 16)),
    ("c", 1): ("horizontal", (18, 12, 12, 6, 1, 1)),
    ("c", 2): ("vertical", (8, 9, 5, 3, 1, 1)),
    ("c", 4): ("horizontal", (12, 12, 12, 6, 2, 1)),
    ("c", 8): ("vertical", (9, 10, 6, 1, 1, 1)),
}


def longest_interior_boundary_run(rgba, diamond, width, height, threshold,
                                  axis, interior_min=INTERIOR_MIN):
    """Return the longest native-pixel straight alpha-contour segment.

    A contour pixel has alpha >= threshold with a neighbor below threshold
    perpendicular to ``axis``. Both sides of the contour must be inside the
    original diamond; the covered pixel must be at least ``interior_min`` from
    every world-tile edge. Scan both contour orientations. Outer diamond edges
    and joins are excluded because those belong to separate contract checks.
    """
    if axis not in ("horizontal", "vertical"):
        raise ValueError(f"Unknown contour axis: {axis}")
    if not 1 <= threshold <= 255:
        raise ValueError(f"Invalid alpha threshold: {threshold}")

    def alpha(pixels, x, y):
        if 0 <= x < width and 0 <= y < height:
            return pixels[(y * width + x) * 4 + 3]
        return 0

    horizontal = axis == "horizontal"
    rows, columns = (height, width) if horizontal else (width, height)
    longest = 0
    for direction in (-1, 1):
        for row in range(rows):
            run = 0
            for column in range(columns):
                x, y = (column, row) if horizontal else (row, column)
                nx, ny = (x, y + direction) if horizontal else (x + direction, y)
                u, v = tile_coordinates(x, y, width, height)
                inside = min(u, v, 1 - u, 1 - v) >= interior_min
                boundary = (inside and alpha(diamond, x, y) > 0
                            and alpha(diamond, nx, ny) > 0
                            and alpha(rgba, x, y) >= threshold
                            and alpha(rgba, nx, ny) < threshold)
                run = run + 1 if boundary else 0
                longest = max(longest, run)
    return longest


def contour_run_measurements(rgba, diamond, width, height,
                             interior_min=INTERIOR_MIN):
    """Return threshold -> {horizontal: pixels, vertical: pixels}."""
    return {
        threshold: {
            axis: longest_interior_boundary_run(
                rgba, diamond, width, height, threshold, axis, interior_min)
            for axis in ("horizontal", "vertical")
        }
        for threshold in THRESHOLDS
    }


def report_contour_runs(source):
    """Measure all 24 frames without inventing unreviewed shape budgets."""
    directories = {
        "originals": V3 / "originals",
        "candidate": V3 / "candidate",
        "published": ROOT / "art_source/tiles/water",
    }
    width, height, diamond = pc.decode_rgba_png(
        str(ROOT / "art_source/tiles/water/base_1.png"))
    frames = {}
    for frame in LAYOUT["frames"]:
        name = frame.get("originalName", frame["name"]) if source == "originals" else frame["name"]
        w, h, rgba = pc.decode_rgba_png(str(directories[source] / name))
        if (w, h) != (width, height):
            raise ValueError(f"Unexpected source canvas: {name} ({w}x{h})")
        frames[frame["name"]] = contour_run_measurements(rgba, diamond, w, h)
    return {"source": source, "interiorMin": INTERIOR_MIN,
            "thresholds": THRESHOLDS, "frames": frames}


class CurvedWaterShoreTest(unittest.TestCase):
    def test_layout_covers_exactly_the_eight_masks_and_three_variants(self):
        frames = LAYOUT["frames"]
        self.assertEqual(len(frames), 24)
        actual = {(frame["kind"], frame["mask"], frame["variant"]) for frame in frames}
        expected = {(kind, mask, variant) for kind, mask in TARGET_MASKS
                    for variant in (1, 2, 3)}
        self.assertEqual(actual, expected)
        self.assertEqual(len({frame["name"] for frame in frames}), 24)

    def test_candidates_preserve_join_bands_texture_and_transparency(self):
        width, height, diamond = pc.decode_rgba_png(
            str(ROOT / "art_source/tiles/water/base_1.png"))
        for frame in LAYOUT["frames"]:
            with self.subTest(frame=frame["name"]):
                original_path = V3 / "originals" / frame.get("originalName", frame["name"])
                self.assertEqual(hashlib.sha256(original_path.read_bytes()).hexdigest(),
                                 frame["sha256"])
                ow, oh, original = pc.decode_rgba_png(str(original_path))
                w, h, rgba = pc.decode_rgba_png(str(V3 / "candidate" / frame["name"]))
                self.assertEqual((ow, oh), (63, 32))
                self.assertEqual((w, h), (63, 32))
                self.assertEqual((width, height), (63, 32))
                self.assertTrue(any(0 < alpha < 255 for alpha in rgba[3::4]))
                for y in range(h):
                    for x in range(w):
                        index = (y * w + x) * 4
                        u, v = tile_coordinates(x, y, w, h)
                        pixel = rgba[index:index + 4]
                        if diamond[index + 3] == 0 or rgba[index + 3] == 0:
                            self.assertEqual(pixel, bytes(4), (x, y))
                        if min(u, v, 1 - u, 1 - v) <= JOIN_BAND:
                            self.assertEqual(pixel, original[index:index + 4], (x, y))
                        if rgba[index + 3] and original[index + 3]:
                            self.assertEqual(rgba[index:index + 3],
                                             original[index:index + 3], (x, y))

    def test_all_three_variants_have_distinct_alpha_contours(self):
        for kind, mask in sorted(TARGET_MASKS):
            variants = []
            for variant in (1, 2, 3):
                name = f"{kind}{mask}" + ("" if variant == 1 else f"_{variant}") + ".png"
                variants.append(pc.decode_rgba_png(str(V3 / "candidate" / name))[2][3::4])
            for first, second in itertools.combinations(variants, 2):
                self.assertNotEqual(first, second, f"{kind}{mask} duplicates a contour")

    def test_visible_shore_contours_do_not_reintroduce_long_flat_ledges(self):
        width, height, diamond = pc.decode_rgba_png(
            str(ROOT / "art_source/tiles/water/base_1.png"))
        for frame in LAYOUT["frames"]:
            axis, limits = CONTOUR_RUN_LIMITS[(frame["kind"], frame["mask"])]
            w, h, rgba = pc.decode_rgba_png(str(V3 / "candidate" / frame["name"]))
            self.assertEqual((w, h), (width, height))
            for threshold, limit in zip(THRESHOLDS, limits):
                with self.subTest(frame=frame["name"], alpha=threshold, axis=axis):
                    run = longest_interior_boundary_run(rgba, diamond, w, h, threshold, axis)
                    self.assertLessEqual(run, limit)


if __name__ == "__main__":
    if "--report" in sys.argv:
        parser = argparse.ArgumentParser(description=__doc__)
        parser.add_argument("--report", action="store_true")
        parser.add_argument("--source", choices=("originals", "candidate", "published"),
                            default="candidate")
        args = parser.parse_args()
        print(json.dumps(report_contour_runs(args.source), indent=2))
    else:
        unittest.main()
