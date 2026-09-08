"""Resolve walk renders into palette-constrained sprites and synchronized GIF proofs."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from PIL import Image, ImageColor, ImageDraw
from bake_male_commoner_v3_pixel import font, paste_sprite
from commoner_pixel_ink import resolve_frame, CONFIG, FINISH

OUT = Path(__file__).resolve().parents[2] / "art_source/characters/male_commoner_v3/pixel_style/walk"
BACKGROUND = "#202b30"
DIRECTIONS = ["s", "se", "e", "ne", "n", "nw", "w", "sw"]
PALETTE = ["#000000"] + CONFIG["palette"] + [BACKGROUND, "#f7dfb8", "#a2b6b7", "#314044"]


def indexed(image):
    palette = Image.new("P", (1, 1))
    values = [channel for color in PALETTE for channel in ImageColor.getrgb(color)]
    palette.putpalette(values + [0] * (768 - len(values)))
    return image.convert("RGB").quantize(palette=palette, dither=Image.Dither.NONE)


def save_gif(frames, path, durations, transparent=False):
    converted = [indexed(frame) for frame in frames]
    options = {"transparency": 0} if transparent else {}
    converted[0].save(path, save_all=True, append_images=converted[1:], duration=durations, loop=0, disposal=2, optimize=False, **options)
    with Image.open(path) as check:
        total = 0
        for frame in range(check.n_frames):
            check.seek(frame)
            total += check.info["duration"]
        if total != 960:
            raise ValueError(f"Incorrect GIF loop duration: {path}: {total}")


def sheet(frames, directions, filename="walk_poses.png"):
    canvas = Image.new("RGB", (8 * 242, len(directions) * 328 + 85), BACKGROUND)
    draw = ImageDraw.Draw(canvas)
    draw.text((24, 18), "ХОДЬБА / 8 ФАЗ / КАДР 80 × 96 PX / УВЕЛИЧЕНИЕ 3×", font=font(25), fill="#f7dfb8")
    for row, direction in enumerate(directions):
        for frame in range(8):
            center = 121 + frame * 242
            bottom = 365 + row * 328
            paste_sprite(canvas, frames[(8, direction, frame)], center, bottom, 3)
            draw.text((center - 44, bottom + 6), f"{direction.upper()} · {frame+1}", font=font(18), fill="#a2b6b7")
    canvas.save(OUT / "previews" / filename)


def overview(frames, count):
    result = []
    for index in range(count):
        canvas = Image.new("RGB", (1040, 775), BACKGROUND)
        draw = ImageDraw.Draw(canvas)
        draw.text((28, 20), f"ОБЫЧНАЯ ХОДЬБА / {count} КАДРОВ / ЦИКЛ 0,96 С", font=font(26), fill="#f7dfb8")
        draw.text((28, 58), "80 × 96 px · увеличение 3× · неподвижные камера и свет", font=font(18), fill="#a2b6b7")
        for position, direction in enumerate(DIRECTIONS):
            column, row = position % 4, position // 4
            center, bottom = column * 260 + 130, row * 330 + 390
            paste_sprite(canvas, frames[(count, direction, index)], center, bottom, 3)
            draw.text((center - 16, bottom + 10), direction.upper(), font=font(20), fill="#a2b6b7")
        result.append(canvas)
    save_gif(result, OUT / f"previews/walk_8_directions_{count}f.gif", [960 // count] * count)


def comparison(frames):
    result = []
    for index in range(24):
        canvas = Image.new("RGB", (800, 930), BACKGROUND)
        draw = ImageDraw.Draw(canvas)
        draw.text((28, 18), "6 ИЛИ 8 КАДРОВ? ОДИН ТЕМП ШАГА", font=font(27), fill="#f7dfb8")
        for column, count in enumerate((6, 8)):
            center = 200 + column * 400
            draw.text((center - 112, 65), f"{count} кадров · {960//count} мс/кадр", font=font(22), fill="#f7dfb8")
            for row, direction in enumerate(("se", "e")):
                bottom = 480 + row * 415
                paste_sprite(canvas, frames[(count, direction, index * count // 24)], center, bottom, 4)
        result.append(canvas)
    save_gif(result, OUT / "previews/walk_6_vs_8.gif", [40] * 24)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--preview", action="store_true")
    args = parser.parse_args()
    settings = json.loads((OUT / "walk_config.json").read_text())
    width, height = settings["frame_size"]
    (OUT / "previews").mkdir(exist_ok=True)
    frames, records = {}, []
    counts, directions = ([8], ["s", "e", "se"]) if args.preview else ([6, 8], DIRECTIONS)
    for count in counts:
        atlas = Image.new("RGBA", (width * count, height * len(directions)))
        for row, direction in enumerate(directions):
            animation = []
            destination = OUT / "pixel" / str(count) / direction
            destination.mkdir(parents=True, exist_ok=True)
            for index in range(count):
                sprite, stats = resolve_frame(OUT / "raw" / str(count) / direction / f"{index:02}.png", width, height)
                sprite.save(destination / f"{index:02}.png")
                frames[(count, direction, index)] = sprite
                atlas.paste(sprite, (index * width, row * height))
                animation.append(sprite)
                records.append({"sample_count": count, "direction": direction, "frame": index, **stats})
            save_gif(animation, destination / "walk.gif", [960 // count] * count, transparent=True)
        atlas.save(OUT / "pixel" / str(count) / "walk_atlas.png")
    sheet(frames, ["s", "se", "e"])
    if not args.preview:
        sheet(frames, ["n", "ne", "nw"], "walk_back_poses.png")
        for count in counts:
            overview(frames, count)
        comparison(frames)
        manifest = {"action": "ordinary_walk", "recommended_frames": 8, "cycle_ms": 960, "frame_size": [width, height], "anchor": [width//2, 84], "directions": DIRECTIONS, "rotation_subject": "model", "camera_and_lights_fixed": True, "supersampling": 4, "palette": CONFIG["palette"], "variants": {"6": {"frame_ms": 160}, "8": {"frame_ms": 120}}, "frames": records}
        manifest["pixel_finish"] = FINISH
        manifest["outline_width_px"] = 1
        manifest["internal_contours"] = "posed depth discontinuities and hair/skin/linen boundaries"
        (OUT / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
    print(f"PIXEL_WALK_COMPLETE: {len(records)} frames")


if __name__ == "__main__":
    main()
