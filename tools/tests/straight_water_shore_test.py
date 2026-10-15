#!/usr/bin/env python3
"""Verify targeted straight shore variants without weakening the v1 joins."""
import hashlib
import itertools
import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import png_codec as pc
from export_water_shore_sprites import ROOT, REVISION, tile_coordinates, straight_shore_offset

V2 = ROOT / "art_source/terrain_revisions/water_shore_v2"
LAYOUT = json.loads((V2 / "layout.json").read_text())
RECIPE = json.loads((V2 / "shore-mask.json").read_text())


class StraightShoreTest(unittest.TestCase):
    def test_twenty_four_valid_frames_preserve_exact_join_bands(self):
        self.assertEqual(len(LAYOUT["frames"]), 24)
        width, height, diamond = pc.decode_rgba_png(str(ROOT / "art_source/tiles/water/base_1.png"))
        for frame in LAYOUT["frames"]:
            w, h, rgba = pc.decode_rgba_png(str(V2 / "candidate" / frame["name"]))
            self.assertEqual((w, h), (63, 32))
            original_path = V2 / "originals" / frame["originalName"]
            self.assertEqual(hashlib.sha256(original_path.read_bytes()).hexdigest(), frame["sha256"])
            _, _, original = pc.decode_rgba_png(str(original_path))
            for y in range(h):
                for x in range(w):
                    index = (y * w + x) * 4
                    u, v = tile_coordinates(x, y, w, h)
                    if diamond[index + 3] == 0 or rgba[index + 3] == 0:
                        self.assertEqual(rgba[index:index + 4], bytes(4), frame["name"])
                    if min(u, v, 1 - u, 1 - v) <= RECIPE["joinBand"]:
                        self.assertEqual(rgba[index:index + 4], original[index:index + 4], frame["name"])
                    if rgba[index + 3] and original[index + 3]:
                        self.assertEqual(rgba[index:index + 3], original[index:index + 3], frame["name"])

    def test_all_six_variants_have_distinct_contours_and_bounded_bends(self):
        for mask in (1, 2, 4, 8):
            alphas = []
            for variant in range(1, 7):
                name = f"b{mask}" + ("" if variant == 1 else f"_{variant}") + ".png"
                alphas.append(pc.decode_rgba_png(str(V2 / "candidate" / name))[2][3::4])
                for i in range(101):
                    point = i / 100
                    for u, v in ((0, point), (1, point), (point, 0), (point, 1)):
                        self.assertEqual(straight_shore_offset(mask, u, v, variant, RECIPE), 0)
                    self.assertLessEqual(abs(straight_shore_offset(mask, point, 0.5, variant, RECIPE)), 0.12)
                    self.assertLessEqual(abs(straight_shore_offset(mask, 0.5, point, variant, RECIPE)), 0.12)
            for first, second in itertools.combinations(alphas, 2):
                self.assertGreater(sum(a != b for a, b in zip(first, second)), 20, f"b{mask} duplicate variant")

    def test_other_fifty_four_overlays_remain_byte_identical(self):
        turns = json.loads((ROOT / "art_source/terrain_revisions/water_shore_v3/layout.json").read_text())
        refined_turns = {frame["name"] for frame in turns["frames"]}
        count = 0
        for frame in json.loads((REVISION / "layout.json").read_text())["frames"]:
            if frame["kind"] == "b" and frame["mask"] in (1, 2, 4, 8):
                continue
            if frame["name"] in refined_turns:
                continue
            count += 1
            self.assertEqual((REVISION / "candidate" / frame["name"]).read_bytes(),
                             (ROOT / "art_source/tiles/water" / frame["name"]).read_bytes())
        self.assertEqual(count, 54)


if __name__ == "__main__":
    unittest.main()
