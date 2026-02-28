from __future__ import annotations

import inspect
import sys
import unittest
from pathlib import Path

import numpy as np

TOOL_DIR = Path(__file__).resolve().parents[1]
if str(TOOL_DIR) not in sys.path:
    sys.path.insert(0, str(TOOL_DIR))

import unfaker


class TestUnfakerParityDefaults(unittest.TestCase):
    def test_process_image_defaults_match_strict_parity(self) -> None:
        signature = inspect.signature(unfaker.process_image)
        self.assertEqual(signature.parameters["max_colors"].default, 32)
        self.assertEqual(signature.parameters["detect_method"].default, "auto")
        self.assertEqual(signature.parameters["edge_detect_method"].default, "tiled")
        self.assertEqual(signature.parameters["downscale_method"].default, "dominant")
        self.assertEqual(signature.parameters["dom_mean_threshold"].default, 0.15)
        self.assertEqual(signature.parameters["alpha_threshold"].default, 128)
        self.assertEqual(signature.parameters["snap_grid"].default, True)
        self.assertEqual(signature.parameters["auto_color_detect"].default, False)


class TestUnfakerParityFunctions(unittest.TestCase):
    def test_detect_scale_from_signal(self) -> None:
        signal = np.array([0, 1, 10, 1, 0, 0, 1, 12, 1, 0, 0, 1, 11, 1, 0], dtype=np.float64)
        detected = unfaker.detect_scale_from_signal(signal)
        self.assertEqual(detected, 5)

    def test_runs_based_detect(self) -> None:
        # 4x4 image with clear 2x2 color runs in both axes.
        image = np.zeros((4, 4, 4), dtype=np.uint8)
        image[:, :, 3] = 255
        image[:, 0:2, 0:3] = [255, 0, 0]
        image[:, 2:4, 0:3] = [0, 255, 0]
        self.assertEqual(unfaker.runs_based_detect(image), 2)

    def test_downscale_by_dominant_color(self) -> None:
        # One 2x2 block where red is dominant.
        image = np.zeros((2, 2, 4), dtype=np.uint8)
        image[:, :, 3] = 255
        image[0, 0, :3] = [255, 0, 0]
        image[0, 1, :3] = [255, 0, 0]
        image[1, 0, :3] = [255, 0, 0]
        image[1, 1, :3] = [0, 255, 0]

        downscaled = unfaker.downscale_by_dominant_color(image, scale=2, threshold=0.15)
        self.assertEqual(tuple(downscaled.shape), (1, 1, 4))
        self.assertEqual(downscaled[0, 0, 0], 255)
        self.assertEqual(downscaled[0, 0, 1], 0)
        self.assertEqual(downscaled[0, 0, 2], 0)
        self.assertEqual(downscaled[0, 0, 3], 255)

    def test_finalize_pixels(self) -> None:
        image = np.array(
            [
                [[10, 20, 30, 127], [40, 50, 60, 128]],
                [[70, 80, 90, 0], [100, 110, 120, 255]],
            ],
            dtype=np.uint8,
        )
        result = unfaker.finalize_pixels(image)

        self.assertTrue(np.array_equal(result[0, 0], np.array([0, 0, 0, 0], dtype=np.uint8)))
        self.assertTrue(np.array_equal(result[0, 1], np.array([40, 50, 60, 255], dtype=np.uint8)))
        self.assertTrue(np.array_equal(result[1, 0], np.array([0, 0, 0, 0], dtype=np.uint8)))
        self.assertTrue(np.array_equal(result[1, 1], np.array([100, 110, 120, 255], dtype=np.uint8)))

    def test_manual_scale_1_preserves_size(self) -> None:
        image = np.zeros((32, 48, 4), dtype=np.uint8)
        image[:, :, 3] = 255
        image[8:24, 12:36, :3] = [180, 120, 60]

        result = unfaker.process_image_sync(
            image,
            max_colors=32,
            manual_scale=1,
            detect_method="auto",
            edge_detect_method="tiled",
            downscale_method="dominant",
            dom_mean_threshold=0.15,
            cleanup={"morph": False, "jaggy": False},
            alpha_threshold=128,
            snap_grid=True,
            auto_color_detect=False,
        )
        self.assertEqual(result["manifest"].final_size, (48, 32))

    def test_validate_image_size_allows_4k_area(self) -> None:
        unfaker.validate_image_size(width=3072, height=3584)
        unfaker.validate_image_size(width=4096, height=4096)

    def test_validate_image_size_rejects_above_4k_area(self) -> None:
        with self.assertRaises(ValueError):
            unfaker.validate_image_size(width=4097, height=4096)

        with self.assertRaises(ValueError):
            unfaker.validate_image_size(width=5000, height=4000)


if __name__ == "__main__":
    unittest.main()
