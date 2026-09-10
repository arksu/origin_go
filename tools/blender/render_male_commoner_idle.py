"""Pose the corrected walk rig at rest and bake eight views with fixed lighting."""
from __future__ import annotations

import json
import math
import sys
from pathlib import Path

import bpy
import numpy as np
from mathutils import Matrix, Vector

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import animate_male_commoner_walk as walk
from render_male_commoner_walk_geometry import posed_surface, raycast_frame

OUT = walk.studio.OUT / "idle"


def set_standing_pose(rig, ankles):
    bones = rig.data.bones
    matrices = {}
    # Retain the walk's natural finger curl, with neither arm swinging.
    walk.pose_at(rig, .25)
    finger_bases = {bone.name: bone.matrix_basis.copy() for bone in rig.pose.bones if bone.name.startswith("finger")}
    torso = Matrix.Translation((0, 0, -.020))
    for name in ("pelvis", "spine", "chest", "head"):
        matrices[name] = torso @ bones[name].matrix_local
    for side, sign in (("l", 1), ("r", -1)):
        hip = torso @ bones[f"thigh.{side}"].head_local
        ankle = ankles[side]
        knee = walk.solve_knee(hip, ankle, bones[f"thigh.{side}"].length, bones[f"shin.{side}"].length)
        matrices[f"thigh.{side}"] = walk.aimed_bone(bones[f"thigh.{side}"], hip, knee)
        matrices[f"shin.{side}"] = walk.aimed_bone(bones[f"shin.{side}"], knee, ankle)
        foot_shift = Matrix.Translation(ankle - bones[f"foot.{side}"].head_local)
        for name in (f"foot.{side}", f"toes.{side}"):
            matrices[name] = foot_shift @ bones[name].matrix_local
        matrices[f"clavicle.{side}"] = torso @ bones[f"clavicle.{side}"].matrix_local
        shoulder = torso @ bones[f"upper_arm.{side}"].head_local
        elbow = shoulder + Vector((sign * .22, -.02, -1)).normalized() * bones[f"upper_arm.{side}"].length
        wrist = elbow + Vector((sign * .08, -.11, -1)).normalized() * bones[f"forearm.{side}"].length
        hand_end = wrist + Vector((sign * .02, -.055, -1)).normalized() * bones[f"hand.{side}"].length
        for name, start, end in ((f"upper_arm.{side}", shoulder, elbow), (f"forearm.{side}", elbow, wrist), (f"hand.{side}", wrist, hand_end)):
            matrices[name] = walk.aimed_bone(bones[name], start, end)
        for finger in range(1, 6):
            for segment in range(1, 4):
                name = f"finger{finger}.{segment}.{side}"
                parent = bones[name].parent.name
                matrices[name] = matrices[parent] @ bones[parent].matrix_local.inverted() @ bones[name].matrix_local @ finger_bases[name]
    for bone in bones:
        basis = bone.matrix_local.inverted()
        if bone.parent:
            basis = basis @ bone.parent.matrix_local @ matrices[bone.parent.name].inverted()
        rig.pose.bones[bone.name].matrix_basis = basis @ matrices[bone.name]
    bpy.context.view_layer.update()


def ground_feet(rig, body):
    ankles = {side: Vector((sign * .130, walk.JOINTS[f"{side}-ankle"].y, .090)) for side, sign in (("l", 1), ("r", -1))}
    masks = walk.foot_masks(body)
    for iteration in range(4):
        set_standing_pose(rig, ankles)
        mesh = body.evaluated_get(bpy.context.evaluated_depsgraph_get()).data
        coordinates = np.empty(len(mesh.vertices) * 3)
        mesh.vertices.foreach_get("co", coordinates)
        coordinates = coordinates.reshape((-1, 3))
        if not np.isfinite(coordinates).all():
            raise RuntimeError("Non-finite standing surface")
        soles = {side: float(coordinates[mask, 2].min()) for side, mask in masks.items()}
        if iteration == 3:
            if any(abs(height - .001) > .001 for height in soles.values()):
                raise RuntimeError(f"Idle soles do not contact the ground: {soles}")
            return soles
        for side in ankles:
            ankles[side].z += .001 - soles[side]


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    (OUT / "geometry").mkdir(exist_ok=True)
    bpy.ops.wm.open_mainfile(filepath=str(walk.OUT / "male_commoner_walk.blend"))
    scene = bpy.context.scene
    scene.frame_set(1)
    rig = bpy.data.objects["Commoner • walk skeleton"]
    rig.animation_data_clear()
    actor = bpy.data.objects["Walk • MODEL DIRECTION"]
    actor.rotation_euler.z = 0
    body = next(obj for obj in walk.studio.character_objects() if obj.name.startswith("Body"))
    for obj in walk.studio.character_objects():
        if obj.name.startswith("Linen") and obj.type == "MESH" and obj.data.shape_keys:
            obj.data.shape_keys.animation_data_clear()
            for key in obj.data.shape_keys.key_blocks[1:]:
                key.value = 0
    soles = ground_feet(rig, body)
    for bone in rig.pose.bones:
        bone.rotation_mode = "QUATERNION"
        for property_name in ("location", "rotation_quaternion", "scale"):
            bone.keyframe_insert(property_name, frame=1, group=bone.name)
    rig.animation_data.action.name = "Idle • standing • both feet planted"
    scene.frame_start = scene.frame_end = 1
    scene["asset_stage"] = "Standing idle; eight model directions; fixed walk camera and lighting"
    bpy.context.preferences.filepaths.save_version = 0
    bpy.ops.wm.save_as_mainfile(filepath=str(OUT / "male_commoner_idle.blend"), compress=True)
    fixed = {obj.name: obj.matrix_world.copy() for obj in bpy.data.objects if obj.type == "LIGHT" or obj == scene.camera}
    tree, labels = posed_surface()
    width, height = walk.SETTINGS["frame_size"]
    for direction, azimuth in walk.studio.DIRECTIONS.items():
        buffers = raycast_frame(tree, labels, azimuth)
        np.savez_compressed(OUT / "geometry" / f"{direction}.npz", **buffers)
        actor.rotation_euler.z = math.radians(azimuth)
        bpy.context.view_layer.update()
        for name, expected in fixed.items():
            if not np.allclose(np.array(bpy.data.objects[name].matrix_world), np.array(expected), atol=1e-6):
                raise RuntimeError(f"Camera or light moved: {name}")
        walk.studio.render(OUT / "raw" / f"{direction}.png", (width * 4, height * 4), 32)
        print("IDLE_DIRECTION", direction, flush=True)
    (OUT / "validation.json").write_text(json.dumps({"feet_min_z_m": soles, "both_feet_planted": True, "camera_and_lights_fixed": True, "frame_size": [width, height], "anchor": [40, 84]}, indent=2) + "\n")
    print("IDLE_RENDER_COMPLETE", flush=True)


if __name__ == "__main__":
    main()
