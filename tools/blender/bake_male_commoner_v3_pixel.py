"""Resolve Blender supersamples to small, opaque pixels and a fixed art palette.

This is the deterministic sprite-bake stage, not an image-generation substitute.
Requires Pillow and NumPy; no network or service is used.
"""
from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

import numpy as np
from PIL import Image, ImageColor, ImageDraw, ImageFont

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "art_source/characters/male_commoner_v3/pixel_style"
CONFIG = json.loads((OUT / "style_config.json").read_text())
BACKGROUND = "#202b30"
DIRECTIONS = ["s", "se", "e", "ne", "n", "nw", "w", "sw"]


def font(size):
    candidates = [Path("/System/Library/Fonts/Supplemental/Arial.ttf"), Path("/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")]
    for path in candidates:
        if path.exists():
            return ImageFont.truetype(str(path), size)
    return ImageFont.load_default(size=size)


def resolve_frame(path, width, height):
    source = Image.open(path).convert("RGBA")
    factor = CONFIG["supersampling"]
    if source.size != (width * factor, height * factor):
        raise ValueError(f"Incorrect source dimensions: {path}: {source.size}")
    # BOX integrates coverage, avoiding Lanczos ringing and invented edge halos.
    pixels = np.asarray(source.resize((width, height), Image.Resampling.BOX))
    opaque = pixels[:, :, 3] >= CONFIG["alpha_threshold"]
    if not opaque.any():
        raise ValueError(f"Empty frame: {path}")
    palette = np.array([ImageColor.getrgb(color) for color in CONFIG["palette"]], dtype=np.float32)
    rgb = pixels[:, :, :3].astype(np.float32)
    differences = rgb[:, :, None, :] - palette[None, None, :, :]
    # float32 avoids overflow in squared RGB distances for very dark/light colours.
    distance = np.sum(differences * differences * [2, 4, 3], axis=3)
    result = np.zeros((height, width, 4), dtype=np.uint8)
    result[opaque, :3] = palette[np.argmin(distance, axis=2)][opaque].astype(np.uint8)
    result[opaque, 3] = 255
    padded = np.pad(opaque, 1)
    adjacent = padded[:-2, 1:-1] | padded[2:, 1:-1] | padded[1:-1, :-2] | padded[1:-1, 2:]
    outline = adjacent & ~opaque
    result[outline, :3] = ImageColor.getrgb(CONFIG["outline"])
    result[outline, 3] = 255
    mask = result[:, :, 3] != 0
    if mask[0].any() or mask[-1].any() or mask[:, 0].any() or mask[:, -1].any():
        raise ValueError(f"Character touches the frame border: {path}")
    final = Image.fromarray(result)
    return final, {"opaque_bbox": list(final.getbbox()), "colors_used": len(np.unique(result[mask, :3], axis=0)), "alpha_values": sorted(np.unique(result[:, :, 3]).tolist())}


def paste_sprite(canvas, sprite, center, bottom, scale):
    enlarged = sprite.resize((sprite.width * scale, sprite.height * scale), Image.Resampling.NEAREST)
    canvas.paste(enlarged, (center - enlarged.width // 2, bottom - enlarged.height), enlarged)


def review_sheet(frames):
    sheet = Image.new("RGB", (1440, 1130), BACKGROUND)
    draw = ImageDraw.Draw(sheet)
    draw.text((42, 25), "V3 / ПЕРСОНАЖ ДЛЯ ПИКСЕЛЬНОГО РЕНДЕРА", font=font(30), fill="#f7dfb8")
    draw.text((42, 67), "Рендер Blender / фиксированная палитра / увеличение без сглаживания", font=font(18), fill="#a2b6b7")
    for index, (width, height) in enumerate(CONFIG["frame_sizes"]):
        center = 240 + index * 480
        draw.rounded_rectangle((index * 480 + 22, 110, index * 480 + 458, 650), radius=12, fill="#2c383b")
        draw.text((center - 95, 132), f"{width} x {height} px", font=font(24), fill="#f7dfb8")
        sprite = frames[("se", width, height)]
        scale = 7 if height <= 64 else 5
        paste_sprite(sheet, sprite, center, 613, scale)
        paste_sprite(sheet, sprite, index * 480 + 70, 244, 1)
        draw.text((index * 480 + 40, 253), "1:1", font=font(14), fill="#a2b6b7")
    draw.text((42, 687), "48 x 64 px / восемь направлений одной модели / увеличение 3x", font=font(22), fill="#f7dfb8")
    for index, direction in enumerate(DIRECTIONS):
        sprite = frames.get((direction, 48, 64))
        if sprite:
            center = 92 + index * 179
            paste_sprite(sheet, sprite, center, 980, 3)
            draw.text((center - 14, 991), direction.upper(), font=font(16), fill="#a2b6b7")
    for index, color in enumerate(CONFIG["palette"]):
        draw.rectangle((42 + index * 32, 1057, 70 + index * 32, 1080), fill=color)
    draw.text((800, 1057), "Статичная модель; без скелетных анимаций", font=font(18), fill="#a2b6b7")
    sheet.save(OUT / "previews/pixel_readability.png")


def model_sheet():
    sheet = Image.new("RGB", (1560, 650), BACKGROUND)
    draw = ImageDraw.Draw(sheet)
    draw.text((35, 22), "V3 / МУЛЬТЯШНАЯ МОДЕЛЬ ДЛЯ ПИКСЕЛЬНОГО ЗАПЕКАНИЯ", font=font(28), fill="#f7dfb8")
    labels = {"front": "СПЕРЕДИ", "hero": "ТРИ ЧЕТВЕРТИ", "back": "СЗАДИ", "portrait": "ЛИЦО"}
    for index, name in enumerate(("front", "hero", "back", "portrait")):
        source = Image.open(OUT / "previews" / f"model_{name}.png").convert("RGB")
        source.thumbnail((380, 760), Image.Resampling.LANCZOS)
        sheet.paste(source, (index * 390 + (390 - source.width) // 2, 105))
        draw.text((index * 390 + 35, 608), labels[name], font=font(18), fill="#a2b6b7")
    sheet.save(OUT / "previews/model_turnaround.png")


def concept_sheet():
    sheet = Image.new("RGB", (1520, 940), BACKGROUND)
    draw = ImageDraw.Draw(sheet)
    draw.text((30, 20), "V3 / СВЕРКА С КОНЦЕПТОМ И ПРОВЕРКА ПОВЯЗКИ", font=font(28), fill="#f7dfb8")
    reference = Image.open(OUT.parent / "reference/male_commoner_concept.png").convert("RGB")
    panels = [
        (reference.crop((122, 120, 425, 774)), "КОНЦЕПТ · ФРАГМЕНТ", 220, 100, (390, 740)),
        (Image.open(OUT / "previews/model_front.png").convert("RGB"), "ОБНОВЛЁННАЯ МОДЕЛЬ", 730, 100, (600, 740)),
        (Image.open(OUT / "previews/model_portrait.png").convert("RGB"), "ЛИЦО", 1290, 100, (300, 375)),
        (Image.open(OUT / "previews/model_linen_side.png").convert("RGB"), "ПОВЯЗКА СБОКУ", 1290, 550, (300, 375)),
    ]
    for source, label, center, top, bounds in panels:
        source.thumbnail(bounds, Image.Resampling.LANCZOS)
        sheet.paste(source, (center - source.width // 2, top))
        draw.text((center - source.width // 2, top - 30), label, font=font(18), fill="#a2b6b7")
    draw.text((30, 890), "Мультяшные пропорции сохранены для маленького игрового кадра.", font=font(20), fill="#a2b6b7")
    sheet.save(OUT / "previews/concept_comparison.png")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--allow-partial", action="store_true")
    args = parser.parse_args()
    frames, reports = {}, {}
    for direction in DIRECTIONS:
        for width, height in CONFIG["frame_sizes"]:
            name = f"{direction}_{width}x{height}.png"
            raw = OUT / "bake/raw" / name
            if not raw.exists() and args.allow_partial:
                continue
            sprite, report = resolve_frame(raw, width, height)
            destination = OUT / "bake/pixel" / f"{width}x{height}" / f"{direction}.png"
            destination.parent.mkdir(parents=True, exist_ok=True)
            sprite.save(destination)
            frames[(direction, width, height)] = sprite
            report["size"] = [width, height]
            report["ground_anchor"] = [width / 2, height * (.5 + CONFIG["camera_target_height"] * math.cos(math.radians(CONFIG["camera_elevation_degrees"])) / CONFIG["camera_scale"])]
            reports[str(destination.relative_to(OUT))] = report
    for width, height in CONFIG["frame_sizes"]:
        atlas = Image.new("RGBA", (width * len(DIRECTIONS), height))
        for index, direction in enumerate(DIRECTIONS):
            if (direction, width, height) in frames:
                atlas.paste(frames[(direction, width, height)], (index * width, 0))
        atlas.save(OUT / "bake/pixel" / f"turnaround_{width}x{height}.png")
    contract = json.loads((OUT / "bake/scene_contract.json").read_text())
    manifest = {"stage": "Static style proof; no character animation", "rotation_subject": contract["rotation_subject"], "camera_fixed": contract["camera_fixed"], "key_light_fixed": contract["key_light_fixed"], "directions_in_atlas_order": DIRECTIONS, "palette": CONFIG["palette"], "outline": CONFIG["outline"], "supersampling": CONFIG["supersampling"], "checks": {"dimensions_match": True, "binary_alpha": True, "palette_locked": True, "no_frame_clipping": True, "all_24_frames_present": len(reports) == 24}, "frames": reports}
    (OUT / "bake/manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    review_sheet(frames)
    model_sheet()
    concept_sheet()
    print(f"Baked and verified {len(reports)} frames; review: {OUT / 'previews/pixel_readability.png'}")


if __name__ == "__main__":
    main()
