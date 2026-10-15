#!/usr/bin/env python3
"""Water shore export and neighbor-mask continuity regression checks."""
import hashlib
import itertools
import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import png_codec as pc
from export_water_shore_sprites import ROOT, REVISION, coverage, edge_coverage

RECIPE = json.loads((REVISION / "shore-mask.json").read_text())
SIDE = ((-1, 0), (0, -1), (1, 0), (0, 1))
CORNER = ((-1, -1), (-1, 1), (1, 1), (1, -1))


def masks(water, x, y):
    border = sum(1 << bit for bit, (dx, dy) in enumerate(SIDE) if (x + dx, y + dy) in water)
    corner = 0
    for bit, (dx, dy) in enumerate(CORNER):
        if (x + dx, y + dy) in water and (x + dx, y) not in water and (x, y + dy) not in water:
            corner |= 1 << bit
    return border, corner


def tile_coverage(water, x, y, u, v, variant):
    if (x, y) in water:
        return 1.0
    border, corner = masks(water, x, y)
    return 1 - (1 - coverage("b", border, u, v, variant, RECIPE)) * (1 - coverage("c", corner, u, v, variant, RECIPE))


class WaterShoreTest(unittest.TestCase):
    def test_all_neighbor_configurations_have_matching_shared_edges(self):
        # Both axis directions, all 1024 water/land neighborhoods around each
        # pair of dry tiles, and every variant pairing. Includes mixed b+c and
        # the diagonal suppression used by Chunk.addBordersAndCorners.
        for direction in ((1, 0), (0, 1)):
            dx, dy = direction
            positions = sorted({(x + ox, y + oy) for x, y in ((0, 0), direction)
                                for ox in (-1, 0, 1) for oy in (-1, 0, 1)} - {(0, 0), direction})
            for bits in range(1 << len(positions)):
                water = {position for bit, position in enumerate(positions) if bits & (1 << bit)}
                for point in (0.0, 0.10, 0.25, 0.46, 0.50, 0.75, 0.90, 1.0):
                    first = (1.0, point) if dx else (point, 1.0)
                    second = (0.0, point) if dx else (point, 0.0)
                    for a, b in itertools.product((1, 2, 3), repeat=2):
                        self.assertAlmostEqual(tile_coverage(water, 0, 0, *first, a), tile_coverage(water, dx, dy, *second, b), places=12)

    def test_shared_profile_is_monotone_and_matches_authored_width(self):
        values = [edge_coverage(index / 1000, RECIPE) for index in range(1001)]
        self.assertEqual(values[0], 1)
        self.assertEqual(values[-1], 0)
        self.assertTrue(all(a >= b for a, b in zip(values, values[1:])))
        self.assertAlmostEqual(edge_coverage(0.4, RECIPE), 100 / 255)
        self.assertAlmostEqual(edge_coverage(0.6, RECIPE), 61 / 255)

    def test_all_exported_frames_keep_canvas_alpha_and_source_contracts(self):
        layout = json.loads((REVISION / "layout.json").read_text())
        width, height, diamond = pc.decode_rgba_png(str(ROOT / "art_source/tiles/water/base_1.png"))
        self.assertEqual(len(layout["frames"]), 90)
        names = {frame["name"] for frame in layout["frames"]}
        self.assertEqual(names, {path.name for path in (REVISION / "candidate").glob("*.png")})
        for frame in layout["frames"]:
            path = REVISION / "candidate" / frame["name"]
            w, h, rgba = pc.decode_rgba_png(str(path))
            self.assertEqual((w, h), (63, 32), path.name)
            self.assertTrue(any(0 < alpha < 255 for alpha in rgba[3::4]), path.name)
            for index in range(0, len(rgba), 4):
                if diamond[index + 3] == 0:
                    self.assertEqual(rgba[index:index + 4], bytes(4), path.name)
                if rgba[index + 3] == 0:
                    self.assertEqual(rgba[index:index + 3], bytes(3), path.name)
            original = REVISION / "originals" / frame["name"]
            self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(), frame["sha256"])

    def test_authored_interiors_and_variant_shapes_are_preserved(self):
        for kind in ("b", "c"):
            for mask in range(1, 16):
                variants = []
                for variant in range(1, 4):
                    name = f"{kind}{mask}" + ("" if variant == 1 else f"_{variant}") + ".png"
                    w, h, rgba = pc.decode_rgba_png(str(REVISION / "candidate" / name))
                    _, _, original = pc.decode_rgba_png(str(REVISION / "originals" / name))
                    for y in range(h):
                        for x in range(w):
                            horizontal = 2 * (x + 0.5) / w - 1
                            vertical = (y + 0.5) / (h / 2)
                            u, v = (horizontal + vertical) / 2, (vertical - horizontal) / 2
                            if min(u, v, 1 - u, 1 - v) >= RECIPE["sourceInteriorFull"]:
                                index = (y * w + x) * 4
                                self.assertEqual(rgba[index:index + 4], original[index:index + 4], name)
                    variants.append(rgba[3::4])
                for first, second in itertools.combinations(variants, 2):
                    self.assertGreater(sum(a != b for a, b in zip(first, second)), 20, f"{kind}{mask} lost contour variation")


if __name__ == "__main__":
    unittest.main()
