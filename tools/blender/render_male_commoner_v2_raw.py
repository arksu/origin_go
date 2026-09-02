"""Render deterministic transparent source frames from male_commoner_v2.blend.

Blender must open the .blend before executing this script. Use the companion
bake_male_commoner_v2.sh command rather than invoking it directly.
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

import bpy


ROOT = Path(__file__).resolve().parents[2]
BAKE_ROOT = ROOT / "art_source" / "characters" / "male_commoner_v2" / "bake"
SOURCE_SIZE = (384, 576)
DIRECTIONS = ("ne", "e", "se", "s", "sw")
PASSES = ("composite", "BODY", "HAIR", "BASE_LOINCLOTH", "FACE_DETAILS")
CHARACTER_COLLECTIONS = PASSES[1:]
ACTIONS = {"idle": (24, range(1, 25)), "walk": (8, range(1, 9))}


def parse_arguments() -> argparse.Namespace:
    arguments = sys.argv[sys.argv.index("--") + 1 :] if "--" in sys.argv else []
    parser = argparse.ArgumentParser()
    parser.add_argument("--preview", action="store_true", help="render only idle frame 1 and walk frame 3")
    return parser.parse_args(arguments)


def required_collection(name: str) -> bpy.types.Collection:
    result = bpy.data.collections.get(name)
    if result is None:
        raise RuntimeError(f"Missing required collection: {name}")
    return result


def configure_scene() -> None:
    scene = bpy.context.scene
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x, scene.render.resolution_y = SOURCE_SIZE
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.film_transparent = True
    scene.render.fps = 12
    review_floor = bpy.data.objects.get("review_floor")
    if review_floor:
        review_floor.hide_render = True


def set_pass_visibility(pass_name: str) -> None:
    for collection_name in CHARACTER_COLLECTIONS:
        collection = required_collection(collection_name)
        collection.hide_render = pass_name != "composite" and collection_name != pass_name


def selected_frames(action_name: str, preview: bool) -> list[tuple[int, int]]:
    if not preview:
        return list(enumerate(ACTIONS[action_name][1]))
    return [(0, 1)] if action_name == "idle" else [(2, 3)]


def render(preview: bool) -> None:
    scene = bpy.context.scene
    rig = bpy.data.objects.get("male_commoner_v2_rig")
    if rig is None or rig.type != "ARMATURE":
        raise RuntimeError("Missing male_commoner_v2_rig armature")
    if rig.animation_data is None:
        raise RuntimeError("Character rig has no animation data")
    for direction in DIRECTIONS:
        if bpy.data.objects.get(f"camera_{direction}") is None:
            raise RuntimeError(f"Missing bake camera: {direction}")

    for pass_name in PASSES:
        set_pass_visibility(pass_name)
        for action_name, (_, _) in ACTIONS.items():
            action = bpy.data.actions.get(action_name)
            if action is None:
                raise RuntimeError(f"Missing action: {action_name}")
            rig.animation_data.action = action
            for direction in DIRECTIONS:
                scene.camera = bpy.data.objects[f"camera_{direction}"]
                for frame_index, scene_frame in selected_frames(action_name, preview):
                    scene.frame_set(scene_frame)
                    output = BAKE_ROOT / "raw" / pass_name.lower() / action_name / direction / f"frame_{frame_index:03d}.png"
                    output.parent.mkdir(parents=True, exist_ok=True)
                    scene.render.filepath = str(output)
                    bpy.ops.render.render(write_still=True)

    raw_manifest = {
        "character": "male_commoner_v2",
        "mode": "preview" if preview else "full",
        "source_size": SOURCE_SIZE,
        "directions": DIRECTIONS,
        "passes": PASSES,
        "actions": {name: {"frames": count, "rendered_frames": [index for index, _ in selected_frames(name, preview)]} for name, (count, _) in ACTIONS.items()},
    }
    (BAKE_ROOT / "raw_manifest.json").write_text(json.dumps(raw_manifest, indent=2) + "\n")


def main() -> None:
    options = parse_arguments()
    configure_scene()
    render(options.preview)


if __name__ == "__main__":
    main()
