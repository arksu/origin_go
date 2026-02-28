from __future__ import annotations

import sys
import unittest
from pathlib import Path

TOOL_DIR = Path(__file__).resolve().parents[1]
if str(TOOL_DIR) not in sys.path:
    sys.path.insert(0, str(TOOL_DIR))

import watcher


class TestWatcherConfig(unittest.TestCase):
    def test_default_unfaker_command_points_to_local_script(self) -> None:
        command = watcher.build_default_unfaker_command()
        self.assertIn("unfaker.py", command)
        self.assertIn("tools/image-pixel-processor", command)

    def test_parse_args_without_unfaker_command(self) -> None:
        original_argv = sys.argv
        try:
            sys.argv = [
                "watcher.py",
                "--input",
                "./input",
                "--output",
                "./output",
            ]
            args = watcher.parse_args()
        finally:
            sys.argv = original_argv

        self.assertEqual(args.input, "./input")
        self.assertEqual(args.output, "./output")
        self.assertEqual(args.bg_model, "auto")
        self.assertFalse(hasattr(args, "unfaker_command"))

    def test_parse_args_with_custom_stages(self) -> None:
        original_argv = sys.argv
        try:
            sys.argv = [
                "watcher.py",
                "--input",
                "./input",
                "--output",
                "./output",
                "--stages",
                "unfaker,frame_split",
            ]
            args = watcher.parse_args()
        finally:
            sys.argv = original_argv

        self.assertEqual(args.stages, "unfaker,frame_split")

    def test_normalize_stages(self) -> None:
        stages = watcher.normalize_stages(" remove_background, frame_split ,unfaker ")
        self.assertEqual(stages, ("remove_background", "frame_split", "unfaker"))

    def test_parse_args_with_custom_bg_model(self) -> None:
        original_argv = sys.argv
        try:
            sys.argv = [
                "watcher.py",
                "--input",
                "./input",
                "--output",
                "./output",
                "--bg-model",
                "birefnet-general",
            ]
            args = watcher.parse_args()
        finally:
            sys.argv = original_argv

        self.assertEqual(args.bg_model, "birefnet-general")


if __name__ == "__main__":
    unittest.main()
