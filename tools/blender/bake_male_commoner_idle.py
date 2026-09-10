"""Apply the established pixel finish to idle and append it after eight walk cells."""
from __future__ import annotations

import json
import sys
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from PIL import Image, ImageDraw
from commoner_pixel_ink import resolve_frame, CONFIG, FINISH
from bake_male_commoner_v3_pixel import font, paste_sprite

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / "art_source/characters/male_commoner_v3/pixel_style"
OUT = SOURCE / "idle"
DIRECTIONS = ["s", "se", "e", "ne", "n", "nw", "w", "sw"]


def main():
    width, height = 80, 96
    destination = OUT / "pixel"
    destination.mkdir(exist_ok=True)
    walk = Image.open(SOURCE / "walk/pixel/8/walk_atlas.png").convert("RGBA")
    if walk.size != (width * 8, height * 8):
        raise ValueError(f"Unexpected source walk atlas: {walk.size}")
    idle = Image.new("RGBA", (width, height * 8))
    combined = Image.new("RGBA", (width * 9, height * 8))
    combined.paste(walk, (0, 0))
    preview = Image.new("RGB", (1040, 765), "#202b30")
    draw = ImageDraw.Draw(preview)
    draw.text((28, 20), "ПОКОЙ / ОТДЕЛЬНАЯ СТОЙКА / 8 НАПРАВЛЕНИЙ", font=font(25), fill="#f7dfb8")
    draw.text((28, 57), "80 × 96 px · увеличение 3× · обе стопы на земле", font=font(19), fill="#a2b6b7")
    records = []
    for row, direction in enumerate(DIRECTIONS):
        sprite, stats = resolve_frame(OUT / "raw" / f"{direction}.png", width, height, geometry_path=OUT / "geometry" / f"{direction}.npz")
        sprite.save(destination / f"{direction}.png")
        idle.paste(sprite, (0, row * height))
        combined.paste(sprite, (width * 8, row * height))
        center, bottom = row % 4 * 260 + 130, row // 4 * 330 + 390
        paste_sprite(preview, sprite, center, bottom, 3)
        draw.text((center - 16, bottom + 10), direction.upper(), font=font(20), fill="#a2b6b7")
        records.append({"direction": direction, **stats})
    idle.save(destination / "idle_atlas.png")
    combined.save(destination / "walk_idle_atlas.png")
    (OUT / "previews").mkdir(exist_ok=True)
    preview.save(OUT / "previews/idle_8_directions.png")
    # The original eight columns must survive packing without any changed pixels.
    if combined.crop((0, 0, width * 8, height * 8)).tobytes() != walk.tobytes():
        raise RuntimeError("Packing modified approved walk frames")
    (OUT / "manifest.json").write_text(json.dumps({"action": "standing_idle", "source": "male_commoner_idle.blend", "frame_size": [width, height], "anchor": [40, 84], "directions": DIRECTIONS, "idle_column": 8, "walk_frame_count": 8, "walk_frame_duration_ms": 120, "palette": CONFIG["palette"], "pixel_finish": FINISH, "frames": records}, indent=2) + "\n")
    print("PIXEL_IDLE_COMPLETE: 8 standing frames; original walk pixels preserved")


if __name__ == "__main__":
    main()
