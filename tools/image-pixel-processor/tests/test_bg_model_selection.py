from __future__ import annotations

import sys
import unittest
from pathlib import Path
from unittest.mock import patch

TOOL_DIR = Path(__file__).resolve().parents[1]
if str(TOOL_DIR) not in sys.path:
    sys.path.insert(0, str(TOOL_DIR))

import bg_remove


class TestBackgroundModelSelection(unittest.TestCase):
    def test_auto_prefers_highest_priority_available(self) -> None:
        with patch.object(bg_remove, "_available_rembg_models", return_value={"u2net", "birefnet-general"}):
            models = bg_remove._resolve_model_candidates("auto")
        self.assertEqual(models[0], "birefnet-general")

    def test_auto_falls_back_when_top_missing(self) -> None:
        with patch.object(bg_remove, "_available_rembg_models", return_value={"u2net"}):
            models = bg_remove._resolve_model_candidates("auto")
        self.assertEqual(models, ("u2net",))

    def test_explicit_model_is_used_as_is(self) -> None:
        models = bg_remove._resolve_model_candidates("isnet-anime")
        self.assertEqual(models, ("isnet-anime",))


if __name__ == "__main__":
    unittest.main()
