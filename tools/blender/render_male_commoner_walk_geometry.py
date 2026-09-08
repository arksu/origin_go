"""Raycast the saved animation for pixel-aligned depth and material boundaries.

These buffers describe the actual posed model. They let the pixel bake draw
occlusion lines without outlining every shading transition or polygon edge.
"""
from __future__ import annotations

import argparse
import json
import math
import sys
from pathlib import Path

import bpy
import numpy as np
from mathutils import Matrix, Vector
from mathutils.bvhtree import BVHTree

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import animate_male_commoner_walk as walk


def posed_surface():
    dependency_graph = bpy.context.evaluated_depsgraph_get()
    vertices, triangles, labels = [], [], []
    vertex_offset = 0
    for obj in walk.studio.character_objects():
        if obj.type != "MESH" or obj.hide_render:
            continue
        evaluated = obj.evaluated_get(dependency_graph)
        mesh = evaluated.to_mesh()
        mesh.calc_loop_triangles()
        positions = np.empty(len(mesh.vertices) * 3)
        mesh.vertices.foreach_get("co", positions)
        matrix = np.array(obj.matrix_world)
        positions = positions.reshape((-1, 3)) @ matrix[:3, :3].T + matrix[:3, 3]
        faces = np.array([list(triangle.vertices) for triangle in mesh.loop_triangles], dtype=np.int32)
        semantic = 2 if obj.name.startswith("Hair") else 3 if obj.name.startswith("Linen") else 4 if obj.name.startswith("Eyes") else 5 if obj.name.startswith("Brow") else 1
        vertices.extend(positions.tolist())
        triangles.extend((faces + vertex_offset).tolist())
        labels.extend([semantic] * len(faces))
        vertex_offset += len(positions)
        evaluated.to_mesh_clear()
    return BVHTree.FromPolygons(vertices, triangles, all_triangles=True), np.array(labels, dtype=np.uint8)


def raycast_frame(tree, labels, azimuth):
    width, height = walk.SETTINGS["frame_size"]
    factor = 2
    camera = bpy.context.scene.camera
    inverse_turn = Matrix.Rotation(-math.radians(azimuth), 3, "Z")
    orientation = inverse_turn @ camera.matrix_world.to_3x3()
    origin = inverse_turn @ camera.matrix_world.translation
    right, up, forward = (orientation @ Vector(axis) for axis in ((1, 0, 0), (0, 1, 0), (0, 0, -1)))
    scale = camera.data.ortho_scale
    depths = np.zeros((height * factor, width * factor), dtype=np.float32)
    materials = np.zeros(depths.shape, dtype=np.uint8)
    heights = np.zeros(depths.shape, dtype=np.float32)
    for row in range(height * factor):
        vertical = (.5 - (row + .5) / (height * factor)) * scale
        for column in range(width * factor):
            horizontal = ((column + .5) / (width * factor) - .5) * scale * width / height
            start = origin + right * horizontal + up * vertical
            point, normal, face, distance = tree.ray_cast(start, forward, 20)
            if face is not None:
                depths[row, column] = distance
                materials[row, column] = labels[face]
                heights[row, column] = point.z
    return {"depth": depths, "material": materials, "height": heights}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--preview", action="store_true")
    args = parser.parse_args(sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else [])
    bpy.ops.wm.open_mainfile(filepath=str(walk.OUT / "male_commoner_walk.blend"))
    scene = bpy.context.scene
    actor = bpy.data.objects["Walk • MODEL DIRECTION"]
    actor.rotation_euler.z = 0
    directions = ["s", "se", "e"] if args.preview else list(walk.studio.DIRECTIONS)
    counts = [8] if args.preview else walk.SETTINGS["sample_counts"]
    samples = sorted({index * 48 // count for count in counts for index in range(count)})
    destination = walk.OUT / "geometry"
    destination.mkdir(exist_ok=True)
    for sample in samples:
        scene.frame_set(sample + 1)
        bpy.context.view_layer.update()
        tree, labels = posed_surface()
        for direction in directions:
            buffers = raycast_frame(tree, labels, walk.studio.DIRECTIONS[direction])
            np.savez_compressed(destination / f"{direction}_{sample:02}.npz", **buffers)
        print("GEOMETRY_FRAME", sample, directions, flush=True)
    (destination / "contract.json").write_text(json.dumps({"source": "male_commoner_walk.blend", "pixel_sample_factor": 2, "directions": directions, "timeline_samples": samples, "materials": {"1": "skin", "2": "hair", "3": "linen", "4": "eyes", "5": "brows"}, "camera_and_light_fixed": True}, indent=2)+"\n")
    print("GEOMETRY_COMPLETE", flush=True)


if __name__ == "__main__":
    main()
