"""Build game meshes from the approved sculpt without changing its source files.

Run with Blender --background --python-exit-code 1 --python this_file.py.
Cloth snapshots are morph targets; skeletal motion remains ordinary glTF clips.
The client uses dual quaternion skinning, matching the source armature modifier.
"""
from __future__ import annotations

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

ROOT = Path(__file__).resolve().parents[2]
SOURCE = walk.OUT / "male_commoner_walk.blend"
OUT = ROOT / "art_source/characters/male_commoner_v3/realtime"
PUBLIC = ROOT / "web_new/public/assets/game/characters/male_commoner/realtime"


def activate(obj):
    bpy.ops.object.select_all(action="DESELECT")
    obj.hide_set(False)
    obj.select_set(True)
    bpy.context.view_layer.objects.active = obj


def coordinates(mesh):
    result = np.empty(len(mesh.vertices) * 3, dtype=np.float32)
    mesh.vertices.foreach_get("co", result)
    return result.reshape(-1, 3)


def material(source):
    name = "Game " + source.name
    existing = bpy.data.materials.get(name)
    if existing:
        return existing
    result = bpy.data.materials.new(name)
    result.use_nodes = True
    shader = result.node_tree.nodes.get("Principled BSDF")
    shader.inputs["Base Color"].default_value = source.diffuse_color
    shader.inputs["Roughness"].default_value = 1
    shader.inputs["Specular IOR Level"].default_value = 0
    result.diffuse_color = source.diffuse_color
    result["region"] = "skin" if "skin" in source.name else "linen" if "cloth" in source.name else "eyes" if "sclera" in source.name else "hair"
    if source.node_tree:
        for node in source.node_tree.nodes:
            if node.type == "TEX_IMAGE" and node.image:
                image = node.image.copy()
                if max(image.size) > 512:
                    image.scale(512, 512)
                texture = result.node_tree.nodes.new("ShaderNodeTexImage")
                texture.image = image
                result.node_tree.links.new(texture.outputs["Color"], shader.inputs["Base Color"])
                break
    return result


def freeze_rest(source):
    for modifier in source.modifiers:
        if modifier.type == "ARMATURE":
            modifier.show_viewport = False
    if source.data.shape_keys and source.name.startswith("Linen"):
        source.data.shape_keys.animation_data_clear()
        for key in source.data.shape_keys.key_blocks[1:]:
            key.value = 0
    bpy.context.view_layer.update()
    mesh = bpy.data.meshes.new_from_object(source.evaluated_get(bpy.context.evaluated_depsgraph_get()), preserve_all_data_layers=True, depsgraph=bpy.context.evaluated_depsgraph_get())
    result = bpy.data.objects.new("Game " + source.name, mesh)
    bpy.context.scene.collection.objects.link(result)
    result.matrix_world = source.matrix_world
    for group in source.vertex_groups:
        result.vertex_groups.new(name=group.name)
    # Transparent optical shells are subpixel and would cover the actual iris.
    if source.name.startswith("Eyes"):
        import bmesh
        editable = bmesh.new()
        editable.from_mesh(mesh)
        bmesh.ops.delete(editable, geom=[face for face in editable.faces if face.material_index == 1], context="FACES")
        editable.to_mesh(mesh)
        editable.free()
    for index, source_material in enumerate(list(mesh.materials)):
        mesh.materials[index] = material(source_material)
    for attribute in list(mesh.color_attributes):
        mesh.color_attributes.remove(attribute)
    return result


def reduce_mesh(obj, triangle_budget):
    activate(obj)
    obj.data.calc_loop_triangles()
    count = len(obj.data.loop_triangles)
    if count > triangle_budget:
        modifier = obj.modifiers.new("Silhouette-preserving game reduction", "DECIMATE")
        modifier.ratio = triangle_budget / count
        modifier.use_collapse_triangulate = True
        bpy.ops.object.modifier_apply(modifier=modifier.name)
    for polygon in obj.data.polygons:
        polygon.use_smooth = True
    obj.data.update()


def transfer_cloth(source, reference, reduced):
    """Transfer eight fitted drapes through the same rest-surface correspondence."""
    reference.calc_loop_triangles()
    positions = coordinates(reference)
    triangles = np.array([tri.vertices[:] for tri in reference.loop_triangles])
    tree = BVHTree.FromPolygons(positions.tolist(), triangles.tolist(), all_triangles=True)
    base = coordinates(reduced.data)
    hits = [tree.find_nearest(Vector(point)) for point in base]
    corner_indices = triangles[[hit[2] for hit in hits]]
    corners = positions[corner_indices]
    points = np.array([hit[0][:] for hit in hits])
    edge_a, edge_b, relative = corners[:, 1] - corners[:, 0], corners[:, 2] - corners[:, 0], points - corners[:, 0]
    dot = lambda first, second: np.einsum("ij,ij->i", first, second)
    aa, ab, bb = dot(edge_a, edge_a), dot(edge_a, edge_b), dot(edge_b, edge_b)
    denominator = aa * bb - ab * ab
    denominator = np.maximum(denominator, 1e-20)
    second = (bb * dot(edge_a, relative) - ab * dot(edge_b, relative)) / denominator
    third = (aa * dot(edge_b, relative) - ab * dot(edge_a, relative)) / denominator
    barycentric = np.clip(np.column_stack((1 - second - third, second, third)), 0, 1)
    barycentric /= barycentric.sum(axis=1)[:, None]
    reduced.shape_key_add(name="Basis")
    for sample in range(8):
        key = source.data.shape_keys.key_blocks[f"Walk cloth • {1 + sample * 6:02}"]
        key.value = 1
        bpy.context.view_layer.update()
        evaluated = source.evaluated_get(bpy.context.evaluated_depsgraph_get())
        posed_mesh = evaluated.to_mesh()
        posed = coordinates(posed_mesh)
        evaluated.to_mesh_clear()
        if len(posed) != len(positions):
            raise RuntimeError(f"Cloth topology changed: {source.name}")
        displacement = np.einsum("ij,ijk->ik", barycentric, (posed - positions)[corner_indices])
        target = reduced.shape_key_add(name=f"walk_cloth_{sample}")
        target.data.foreach_set("co", (base + displacement).ravel())
        key.value = 0


def join_meshes(objects, name):
    activate(objects[0])
    for obj in objects:
        obj.select_set(True)
    if len(objects) > 1:
        bpy.ops.object.join()
    result = bpy.context.object
    result.name = name
    return result


def bind(obj, rig):
    activate(obj)
    for group in list(obj.vertex_groups):
        if group.name not in rig.data.bones:
            obj.vertex_groups.remove(group)
    bpy.ops.object.vertex_group_limit_total(limit=4)
    bpy.ops.object.vertex_group_normalize_all(lock_active=False)
    for vertex in obj.data.vertices:
        if not vertex.groups or abs(sum(group.weight for group in vertex.groups) - 1) > .002:
            raise RuntimeError(f"Invalid weights on {obj.name}:{vertex.index}")
    modifier = obj.modifiers.new("Preserve muscle volume", "ARMATURE")
    modifier.object = rig
    modifier.use_deform_preserve_volume = True
    obj.parent = rig
    obj["skinning"] = "dual_quaternion"


def carry_pose(rig):
    """Lift both palms above the crown with relaxed outward elbows."""
    bones = rig.data.bones
    desired = {bone.name: rig.pose.bones[bone.name].matrix.copy() for bone in bones}
    chest_transform = desired["chest"] @ bones["chest"].matrix_local.inverted()
    for side, sign in (("l", 1), ("r", -1)):
        shoulder = chest_transform @ bones[f"upper_arm.{side}"].head_local
        elbow = shoulder + Vector((sign * .52, -.10, .56)).normalized() * bones[f"upper_arm.{side}"].length
        wrist = elbow + Vector((-sign * .22, -.08, .94)).normalized() * bones[f"forearm.{side}"].length
        hand_end = wrist + Vector((-sign * .28, -.95, .13)).normalized() * bones[f"hand.{side}"].length
        for name, start, end in ((f"upper_arm.{side}", shoulder, elbow), (f"forearm.{side}", elbow, wrist), (f"hand.{side}", wrist, hand_end)):
            desired[name] = walk.aimed_bone(bones[name], start, end)
        for finger in range(1, 6):
            for segment in range(1, 4):
                name = f"finger{finger}.{segment}.{side}"
                parent = bones[name].parent.name
                desired[name] = desired[parent] @ bones[parent].matrix_local.inverted() @ bones[name].matrix_local
    for bone in bones:
        basis = bone.matrix_local.inverted()
        if bone.parent:
            basis = basis @ bone.parent.matrix_local @ desired[bone.parent.name].inverted()
        rig.pose.bones[bone.name].matrix_basis = basis @ desired[bone.name]
    bpy.context.view_layer.update()


def create_clips(rig, walk_action, idle_matrices):
    walk_action.name = "walk"
    poses = []
    rig.animation_data.action = walk_action
    for frame in range(1, 50):
        bpy.context.scene.frame_set(frame)
        poses.append({bone.name: bone.matrix_basis.copy() for bone in rig.pose.bones})
    clips = [walk_action]
    for name in ("idle", "carry_idle", "carry_walk"):
        rig.animation_data.action = None
        action = bpy.data.actions.new(name)
        rig.animation_data.action = action
        frames = range(1, 50) if name == "carry_walk" else (1, 49)
        for frame in frames:
            bpy.context.scene.frame_set(frame)
            matrices = poses[frame - 1] if name == "carry_walk" else idle_matrices
            for bone in rig.pose.bones:
                bone.matrix_basis = matrices[bone.name]
            bpy.context.view_layer.update()
            if name.startswith("carry"):
                carry_pose(rig)
            for bone in rig.pose.bones:
                bone.rotation_mode = "QUATERNION"
                for channel in ("location", "rotation_quaternion", "scale"):
                    bone.keyframe_insert(channel, frame=frame, group=bone.name)
        clips.append(action)
    rig.animation_data.action = None
    for action in clips:
        track = rig.animation_data.nla_tracks.new()
        track.name = action.name
        track.strips.new(action.name, 1, action)
        track.mute = True
    return clips


def export(rig, objects, name, animations):
    activate(rig)
    for obj in objects:
        obj.select_set(True)
    bpy.ops.export_scene.gltf(filepath=str(PUBLIC / name), export_format="GLB", use_selection=True,
        export_animations=animations, export_animation_mode="NLA_TRACKS", export_frame_range=False,
        export_force_sampling=True, export_anim_slide_to_zero=True, export_morph=True,
        export_morph_normal=True, export_morph_animation=False, export_extras=True,
        export_rest_position_armature=True, export_yup=True)


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    PUBLIC.mkdir(parents=True, exist_ok=True)
    bpy.ops.wm.open_mainfile(filepath=str(walk.studio.OUT / "idle/male_commoner_idle.blend"))
    idle_matrices = {bone.name: bone.matrix_basis.copy() for bone in bpy.data.objects["Commoner • walk skeleton"].pose.bones}
    bpy.ops.wm.open_mainfile(filepath=str(SOURCE))
    scene = bpy.context.scene
    scene.frame_set(1)
    rig = bpy.data.objects["Commoner • walk skeleton"]
    walk_action = rig.animation_data.action
    bpy.data.objects["Walk • MODEL DIRECTION"].rotation_euler.z = 0
    rig.parent = None
    rig.matrix_world = Matrix.Identity(4)
    rig.name = "commoner_rig"
    sources = [obj for obj in walk.studio.character_objects() if obj.type == "MESH" and not obj.hide_render]
    groups = {"body": [], "hair": [], "eyes": [], "linen_wrap": [], "linen_belt": []}
    for source in sources:
        reduced = freeze_rest(source)
        reference = reduced.data.copy() if source.data.shape_keys and source.name.startswith("Linen") else None
        if source.name.startswith("Body"):
            group, budget = "body", 11000
        elif source.name.startswith("Eyes"):
            group, budget = "eyes", 550
        elif source.name.startswith("Linen"):
            group = "linen_belt" if "waistband" in source.name else "linen_wrap"
            budget = 500 if "panel" in source.name else 320 if group == "linen_belt" else 90
        else:
            group, budget = "hair", 650 if "scalp" in source.name else 90
        reduce_mesh(reduced, budget)
        if reference is not None:
            transfer_cloth(source, reference, reduced)
            bpy.data.meshes.remove(reference)
        groups[group].append(reduced)
        print("REDUCED", source.name, len(reduced.data.vertices), flush=True)
    game_meshes = {name: join_meshes(objects, name) for name, objects in groups.items()}
    for obj in game_meshes.values():
        bind(obj, rig)
    for source in sources:
        bpy.data.objects.remove(source, do_unlink=True)
    clips = create_clips(rig, walk_action, idle_matrices)
    scene.frame_start, scene.frame_end = 1, 49
    scene.frame_set(1)
    core = [game_meshes[name] for name in ("body", "hair", "eyes")]
    for obj in core:
        obj["lod"] = 0
    low_meshes = []
    for name, budget in (("body", 3200), ("hair", 1200), ("eyes", 220)):
        low = game_meshes[name].copy()
        low.data = game_meshes[name].data.copy()
        low.name = name + "_low"
        low.modifiers.clear()
        scene.collection.objects.link(low)
        reduce_mesh(low, budget)
        bind(low, rig)
        low["lod"] = 1
        low_meshes.append(low)
    core += low_meshes
    export(rig, core, "commoner.glb", True)
    for name in ("linen_wrap", "linen_belt"):
        export(rig, [game_meshes[name]], name + ".glb", False)
    report = {"source": str(SOURCE.relative_to(ROOT)), "bones": len(rig.data.bones), "skinning": "dual_quaternion", "clips": ["idle", "walk", "carry_idle", "carry_walk"], "meshes": {}}
    for name, obj in game_meshes.items():
        obj.data.calc_loop_triangles()
        report["meshes"][name] = {"vertices": len(obj.data.vertices), "triangles": len(obj.data.loop_triangles), "morphs": len(obj.data.shape_keys.key_blocks) - 1 if obj.data.shape_keys else 0}
    report["triangles"] = sum(entry["triangles"] for entry in report["meshes"].values())
    report["low_triangles"] = sum(len(obj.data.loop_triangles) for obj in low_meshes) + report["meshes"]["linen_wrap"]["triangles"] + report["meshes"]["linen_belt"]["triangles"]
    (OUT / "export-report.json").write_text(json.dumps(report, indent=2) + "\n")
    bpy.context.preferences.filepaths.save_version = 0
    rig.animation_data.action = next(action for action in clips if action.name == "idle")
    scene.frame_set(1)
    for low in low_meshes:
        low.hide_set(True)
    bpy.ops.wm.save_as_mainfile(filepath=str(OUT / "male_commoner_realtime.blend"), compress=True)
    print("EXPORT_COMPLETE", json.dumps(report), flush=True)


if __name__ == "__main__":
    main()
