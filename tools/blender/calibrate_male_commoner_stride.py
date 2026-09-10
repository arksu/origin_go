"""Measure support-foot travel and project it through the saved bake camera."""
from __future__ import annotations

import json
import math
from pathlib import Path

import bpy
from bpy_extras.object_utils import world_to_camera_view
from mathutils import Matrix, Vector

ROOT = Path(__file__).resolve().parents[2]
OUT = ROOT / "art_source/characters/male_commoner_v3/pixel_style/walk"
DIRECTIONS = [("NE", 135), ("E", 90), ("SE", 45), ("S", 0), ("SW", 315), ("W", 270), ("NW", 225), ("N", 180)]


def slope(samples):
    mean_phase = sum(phase for phase, _ in samples) / len(samples)
    mean_y = sum(position for _, position in samples) / len(samples)
    return sum((phase - mean_phase) * (position - mean_y) for phase, position in samples) / sum((phase - mean_phase)**2 for phase, _ in samples)


def main():
    bpy.ops.wm.open_mainfile(filepath=str(OUT / "male_commoner_walk.blend"))
    scene = bpy.context.scene
    rig = bpy.data.objects["Commoner • walk skeleton"]
    bpy.data.objects["Walk • MODEL DIRECTION"].rotation_euler.z = 0
    settings = json.loads((OUT / "walk_config.json").read_text())
    metrics = json.loads((OUT / "gait_metrics.json").read_text())
    samples = {side: [] for side in ("l", "r")}
    for index, record in enumerate(metrics[:-1]):
        scene.frame_set(index + 1)
        for side in samples:
            sample = record[side]
            # Exclude heel strike and toe-off, when ankle rotation is not root travel.
            if .12 <= sample["phase"] <= .38 and abs(sample["pitch"]) < .001:
                ankle = rig.matrix_world @ rig.pose.bones[f"foot.{side}"].head
                samples[side].append((sample["phase"], ankle.y))
    strides = {side: slope(points) for side, points in samples.items()}
    stride = sum(strides.values()) / 2
    if stride <= 0 or abs(strides["l"] - strides["r"]) > .001:
        raise RuntimeError(f"Inconsistent support travel: {strides}")
    width, height = settings["frame_size"]
    origin = world_to_camera_view(scene, scene.camera, Vector((0, 0, 0)))
    directions = []
    for index, (name, azimuth) in enumerate(DIRECTIONS):
        forward = Matrix.Rotation(math.radians(azimuth), 3, "Z") @ Vector((0, -stride, 0))
        projected = world_to_camera_view(scene, scene.camera, forward)
        sprite_dx, sprite_dy = (projected.x - origin.x) * width, -(projected.y - origin.y) * height
        world_angle = index * math.pi / 4 - math.pi / 2
        game_dx, game_dy = math.cos(world_angle), math.sin(world_angle)
        # A world tile has a 64x32-pixel diamond in the client, before camera zoom.
        tile_dx, tile_dy = 32 * (game_dx - game_dy), 16 * (game_dx + game_dy)
        distance_tiles = math.hypot(sprite_dx, sprite_dy) / math.hypot(tile_dx, tile_dy)
        directions.append({"direction": name, "stride_screen_px": math.hypot(sprite_dx, sprite_dy), "stride_vector_px": [sprite_dx, sprite_dy], "world_tiles_per_cycle": distance_tiles, "world_units_per_cycle_at_12_units_per_tile": distance_tiles * 12, "frames_per_world_unit_at_12_units_per_tile": 8 / (distance_tiles * 12)})
    lengths = [direction["world_tiles_per_cycle"] for direction in directions]
    spread = max(lengths) - min(lengths)
    if spread > 1e-5:
        raise RuntimeError(f"Projection mismatch requires directional calibration: {directions}")
    length_tiles = sum(lengths) / len(lengths)
    result = {"source": "male_commoner_walk.blend", "method": "OLS slope of the saved posed foot-bone position against normalized cycle, during flat support", "samples_per_foot": {side: len(points) for side, points in samples.items()}, "measured_model_metres_per_cycle": strides, "camera_elevation_degrees": 30, "camera_ortho_scale": scene.camera.data.ortho_scale, "frame_size": [width, height], "tile_size_px": [64, 32], "cycle_distance_tiles": length_tiles, "direction_spread_tiles": spread, "directions": directions, "frames": 8, "reference_cycle_ms": 960, "default_server_walk_speed_world_units_per_second": 32, "animation_fps_at_default_walk_speed": 32 * 8 / (12 * length_tiles)}
    (OUT / "stride_calibration.json").write_text(json.dumps(result, indent=2) + "\n")
    print("STRIDE_CALIBRATION", json.dumps(result), flush=True)


if __name__ == "__main__":
    main()
