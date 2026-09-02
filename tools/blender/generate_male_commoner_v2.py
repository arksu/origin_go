"""Generate the detailed, original male commoner v2 Blender source asset.

Run from the repository root:

    /Applications/Blender.app/Contents/MacOS/Blender --background \
      --python tools/blender/generate_male_commoner_v2.py
"""

from __future__ import annotations

import json
import math
from pathlib import Path

import bpy
from mathutils import Vector


ROOT = Path(__file__).resolve().parents[2]
OUTPUT = ROOT / "art_source" / "characters" / "male_commoner_v2"
BLEND_PATH = OUTPUT / "male_commoner_v2.blend"
PREVIEWS = OUTPUT / "previews"
FPS = 12


def clear_scene() -> None:
    bpy.ops.object.select_all(action="SELECT")
    bpy.ops.object.delete(use_global=False)
    root_collection = bpy.context.scene.collection
    for collection in list(root_collection.children):
        root_collection.children.unlink(collection)
        bpy.data.collections.remove(collection)


def collection(name: str) -> bpy.types.Collection:
    result = bpy.data.collections.new(name)
    bpy.context.scene.collection.children.link(result)
    return result


def move_to_collection(obj: bpy.types.Object, target: bpy.types.Collection) -> None:
    for current in list(obj.users_collection):
        current.objects.unlink(obj)
    target.objects.link(obj)


def material(name: str, rgba: tuple[float, float, float, float], roughness: float = 0.78) -> bpy.types.Material:
    result = bpy.data.materials.new(name)
    result.use_nodes = True
    shader = result.node_tree.nodes.get("Principled BSDF")
    shader.inputs["Base Color"].default_value = rgba
    shader.inputs["Roughness"].default_value = roughness
    shader.inputs["Specular IOR Level"].default_value = 0.22
    return result


def shade_smooth(obj: bpy.types.Object) -> None:
    for polygon in obj.data.polygons:
        polygon.use_smooth = True


def assign_bone_weight(obj: bpy.types.Object, bone_name: str) -> None:
    group = obj.vertex_groups.new(name=bone_name)
    group.add(range(len(obj.data.vertices)), 1.0, "REPLACE")


def ellipsoid(
    name: str,
    location: tuple[float, float, float],
    scale: tuple[float, float, float],
    target_collection: bpy.types.Collection,
    surface: bpy.types.Material,
    bone_name: str,
) -> bpy.types.Object:
    bpy.ops.mesh.primitive_uv_sphere_add(segments=24, ring_count=16, location=location)
    obj = bpy.context.object
    obj.name = name
    obj.scale = scale
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    obj.data.materials.append(surface)
    shade_smooth(obj)
    assign_bone_weight(obj, bone_name)
    move_to_collection(obj, target_collection)
    return obj


def capsule(
    name: str,
    start: tuple[float, float, float],
    end: tuple[float, float, float],
    radius: float,
    target_collection: bpy.types.Collection,
    surface: bpy.types.Material,
    bone_name: str,
) -> bpy.types.Object:
    start_v = Vector(start)
    end_v = Vector(end)
    direction = end_v - start_v
    midpoint = (start_v + end_v) / 2
    obj = ellipsoid(name, tuple(midpoint), (radius, radius, direction.length / 2 + radius * 0.45), target_collection, surface, bone_name)
    obj.rotation_mode = "QUATERNION"
    obj.rotation_quaternion = direction.to_track_quat("Z", "Y")
    return obj


def box(
    name: str,
    location: tuple[float, float, float],
    scale: tuple[float, float, float],
    target_collection: bpy.types.Collection,
    surface: bpy.types.Material,
    bone_name: str,
    bevel: float = 0.08,
) -> bpy.types.Object:
    bpy.ops.mesh.primitive_cube_add(size=1, location=location)
    obj = bpy.context.object
    obj.name = name
    obj.scale = scale
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    bevel_modifier = obj.modifiers.new("soft_edges", "BEVEL")
    bevel_modifier.width = bevel
    bevel_modifier.segments = 3
    bpy.context.view_layer.objects.active = obj
    bpy.ops.object.modifier_apply(modifier=bevel_modifier.name)
    obj.data.materials.append(surface)
    shade_smooth(obj)
    assign_bone_weight(obj, bone_name)
    move_to_collection(obj, target_collection)
    return obj


def join_meshes(objects: list[bpy.types.Object], name: str) -> bpy.types.Object:
    bpy.ops.object.select_all(action="DESELECT")
    for obj in objects:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = objects[0]
    bpy.ops.object.join()
    joined = bpy.context.object
    joined.name = name
    return joined


def bind_existing_weights(mesh: bpy.types.Object, rig: bpy.types.Object) -> None:
    modifier = mesh.modifiers.new("character_armature", "ARMATURE")
    modifier.object = rig
    mesh.parent = rig


def merge_and_auto_weight_body(mesh: bpy.types.Object, rig: bpy.types.Object) -> None:
    """Turn intersecting sculpt forms into one smooth body before skinning."""
    remesh = mesh.modifiers.new("organic_surface", "REMESH")
    remesh.mode = "VOXEL"
    remesh.voxel_size = 0.065
    remesh.use_smooth_shade = True
    bpy.context.view_layer.objects.active = mesh
    bpy.ops.object.modifier_apply(modifier=remesh.name)
    shade_smooth(mesh)
    mesh.vertex_groups.clear()
    bpy.ops.object.select_all(action="DESELECT")
    mesh.select_set(True)
    rig.select_set(True)
    bpy.context.view_layer.objects.active = rig
    bpy.ops.object.parent_set(type="ARMATURE_AUTO")


def create_rig(target_collection: bpy.types.Collection) -> bpy.types.Object:
    armature = bpy.data.armatures.new("male_commoner_v2_rig")
    rig = bpy.data.objects.new("male_commoner_v2_rig", armature)
    target_collection.objects.link(rig)
    bpy.context.view_layer.objects.active = rig
    rig.select_set(True)
    bpy.ops.object.mode_set(mode="EDIT")
    definitions = {
        "root": ((0, 0, -0.48), (0, 0, 0.15), None),
        "pelvis": ((0, 0, 0.15), (0, 0, 0.98), "root"),
        "spine": ((0, 0, 0.98), (0, 0, 1.88), "pelvis"),
        "chest": ((0, 0, 1.88), (0, 0, 2.28), "spine"),
        "neck": ((0, 0, 2.28), (0, 0, 2.55), "chest"),
        "head": ((0, 0, 2.55), (0, 0, 3.20), "neck"),
        "clavicle.L": ((-0.05, 0, 2.16), (-0.62, 0, 2.16), "chest"),
        "upper_arm.L": ((-0.62, 0, 2.16), (-1.34, 0, 2.16), "clavicle.L"),
        "forearm.L": ((-1.34, 0, 2.16), (-2.00, -0.03, 2.16), "upper_arm.L"),
        "hand.L": ((-2.00, -0.03, 2.16), (-2.28, -0.03, 2.16), "forearm.L"),
        "clavicle.R": ((0.05, 0, 2.16), (0.62, 0, 2.16), "chest"),
        "upper_arm.R": ((0.62, 0, 2.16), (1.34, 0, 2.16), "clavicle.R"),
        "forearm.R": ((1.34, 0, 2.16), (2.00, -0.03, 2.16), "upper_arm.R"),
        "hand.R": ((2.00, -0.03, 2.16), (2.28, -0.03, 2.16), "forearm.R"),
        "thigh.L": ((-0.31, 0, 0.86), (-0.36, 0.01, 0.02), "pelvis"),
        "shin.L": ((-0.36, 0.01, 0.02), (-0.36, -0.02, -0.48), "thigh.L"),
        "foot.L": ((-0.36, -0.02, -0.48), (-0.36, -0.56, -0.48), "shin.L"),
        "thigh.R": ((0.31, 0, 0.86), (0.36, 0.01, 0.02), "pelvis"),
        "shin.R": ((0.36, 0.01, 0.02), (0.36, -0.02, -0.48), "thigh.R"),
        "foot.R": ((0.36, -0.02, -0.48), (0.36, -0.56, -0.48), "shin.R"),
    }
    bones: dict[str, bpy.types.EditBone] = {}
    for name, (head, tail, parent_name) in definitions.items():
        bone = armature.edit_bones.new(name)
        bone.head = head
        bone.tail = tail
        bone.use_deform = name != "root"
        if parent_name:
            bone.parent = bones[parent_name]
        bones[name] = bone
    bpy.ops.object.mode_set(mode="POSE")
    for pose_bone in rig.pose.bones:
        pose_bone.rotation_mode = "XYZ"
    bpy.ops.object.mode_set(mode="OBJECT")
    rig.show_in_front = True
    return rig


def create_body(rig: bpy.types.Object, target_collection: bpy.types.Collection, skin: bpy.types.Material) -> bpy.types.Object:
    parts: list[bpy.types.Object] = []
    add = lambda *args: parts.append(ellipsoid(*args))
    add_capsule = lambda *args: parts.append(capsule(*args))

    # Core volumes and deliberate muscle landmarks are separate smooth forms.
    add("torso_core", (0, 0.02, 1.55), (0.68, 0.40, 0.94), target_collection, skin, "spine")
    add("pelvis_core", (0, 0.02, 0.84), (0.56, 0.37, 0.42), target_collection, skin, "pelvis")
    add("chest.L", (-0.31, -0.25, 1.92), (0.43, 0.22, 0.31), target_collection, skin, "chest")
    add("chest.R", (0.31, -0.25, 1.92), (0.43, 0.22, 0.31), target_collection, skin, "chest")
    add("deltoid.L", (-0.73, -0.01, 2.12), (0.33, 0.30, 0.31), target_collection, skin, "upper_arm.L")
    add("deltoid.R", (0.73, -0.01, 2.12), (0.33, 0.30, 0.31), target_collection, skin, "upper_arm.R")
    for side, sign in (("L", -1), ("R", 1)):
        for row, z in enumerate((1.55, 1.28, 1.03)):
            add(f"ab_{row}.{side}", (0.19 * sign, -0.36, z), (0.19, 0.13, 0.17), target_collection, skin, "spine")
        add(f"oblique.{side}", (0.47 * sign, -0.19, 1.28), (0.19, 0.16, 0.38), target_collection, skin, "spine")
        add(f"lat.{side}", (0.53 * sign, 0.10, 1.62), (0.25, 0.20, 0.49), target_collection, skin, "chest")

    add_capsule("neck", (0, 0, 2.17), (0, 0, 2.58), 0.20, target_collection, skin, "neck")
    add("head", (0, -0.03, 2.91), (0.48, 0.43, 0.57), target_collection, skin, "head")
    add("jaw", (0, -0.20, 2.67), (0.36, 0.29, 0.23), target_collection, skin, "head")
    add("nose", (0, -0.47, 2.95), (0.08, 0.10, 0.12), target_collection, skin, "head")
    add("ear.L", (-0.47, -0.01, 2.92), (0.08, 0.05, 0.13), target_collection, skin, "head")
    add("ear.R", (0.47, -0.01, 2.92), (0.08, 0.05, 0.13), target_collection, skin, "head")

    for side, sign in (("L", -1), ("R", 1)):
        add_capsule(f"upper_arm.{side}", (0.83 * sign, 0, 2.12), (1.35 * sign, 0, 2.12), 0.20, target_collection, skin, f"upper_arm.{side}")
        add(f"bicep.{side}", (1.14 * sign, -0.08, 2.17), (0.31, 0.22, 0.25), target_collection, skin, f"upper_arm.{side}")
        add_capsule(f"forearm.{side}", (1.40 * sign, 0, 2.12), (1.99 * sign, -0.02, 2.12), 0.17, target_collection, skin, f"forearm.{side}")
        add(f"forearm_bulk.{side}", (1.68 * sign, -0.10, 2.12), (0.30, 0.20, 0.22), target_collection, skin, f"forearm.{side}")
        add(f"hand.{side}", (2.11 * sign, -0.03, 2.12), (0.25, 0.16, 0.20), target_collection, skin, f"hand.{side}")

        add_capsule(f"thigh.{side}", (0.29 * sign, 0, 0.81), (0.35 * sign, 0.02, 0.04), 0.26, target_collection, skin, f"thigh.{side}")
        add(f"quad.{side}", (0.33 * sign, -0.17, 0.51), (0.29, 0.18, 0.43), target_collection, skin, f"thigh.{side}")
        add_capsule(f"shin.{side}", (0.36 * sign, 0.01, 0.00), (0.36 * sign, -0.03, -0.45), 0.16, target_collection, skin, f"shin.{side}")
        add(f"calf.{side}", (0.36 * sign, 0.12, -0.12), (0.22, 0.17, 0.31), target_collection, skin, f"shin.{side}")
        add(f"foot.{side}", (0.36 * sign, -0.25, -0.48), (0.23, 0.40, 0.13), target_collection, skin, f"foot.{side}")
    body = join_meshes(parts, "male_commoner_v2_body")
    merge_and_auto_weight_body(body, rig)
    return body


def create_hair(rig: bpy.types.Object, target_collection: bpy.types.Collection, hair_material: bpy.types.Material) -> bpy.types.Object:
    parts: list[bpy.types.Object] = [
        ellipsoid("hair_cap", (0, 0.04, 3.26), (0.52, 0.46, 0.28), target_collection, hair_material, "head"),
        box("hair_fringe", (0, -0.40, 3.18), (0.34, 0.08, 0.10), target_collection, hair_material, "head", 0.06),
    ]
    for index, x in enumerate((-0.34, -0.17, 0.0, 0.17, 0.34)):
        strand = ellipsoid(f"hair_strand_{index}", (x, -0.25, 3.23 + 0.08 * math.cos(index)), (0.12, 0.12, 0.27), target_collection, hair_material, "head")
        strand.rotation_euler.y = -0.35 * x
        parts.append(strand)
    for index, x in enumerate((-0.31, -0.10, 0.11, 0.31)):
        parts.append(ellipsoid(f"hair_back_{index}", (x, 0.34, 3.09), (0.15, 0.12, 0.31), target_collection, hair_material, "head"))
    hair = join_meshes(parts, "male_commoner_v2_hair")
    bind_existing_weights(hair, rig)
    return hair


def cloth_panel(name: str, y: float, z_top: float, z_bottom: float, target_collection: bpy.types.Collection, surface: bpy.types.Material) -> bpy.types.Object:
    vertices = [(-0.48, y, z_top), (0.48, y, z_top), (0.36, y - 0.04, z_bottom), (0, y - 0.08, z_bottom - 0.08), (-0.36, y - 0.04, z_bottom)]
    mesh = bpy.data.meshes.new(name)
    mesh.from_pydata(vertices, [], [(0, 1, 2, 3, 4)])
    obj = bpy.data.objects.new(name, mesh)
    target_collection.objects.link(obj)
    obj.data.materials.append(surface)
    solidify = obj.modifiers.new("cloth_thickness", "SOLIDIFY")
    solidify.thickness = 0.035
    bevel_modifier = obj.modifiers.new("soft_cloth_edges", "BEVEL")
    bevel_modifier.width = 0.025
    bevel_modifier.segments = 2
    assign_bone_weight(obj, "pelvis")
    return obj


def create_loincloth(rig: bpy.types.Object, target_collection: bpy.types.Collection, linen: bpy.types.Material, leather: bpy.types.Material) -> bpy.types.Object:
    front = cloth_panel("loincloth_front", -0.39, 1.03, 0.38, target_collection, linen)
    back = cloth_panel("loincloth_back", 0.39, 1.03, 0.42, target_collection, linen)
    bpy.ops.mesh.primitive_torus_add(major_radius=0.56, minor_radius=0.06, major_segments=28, minor_segments=8, location=(0, 0, 1.03))
    belt = bpy.context.object
    belt.name = "loincloth_belt"
    belt.scale.y = 0.72
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    belt.data.materials.append(leather)
    shade_smooth(belt)
    assign_bone_weight(belt, "pelvis")
    move_to_collection(belt, target_collection)
    loincloth = join_meshes([front, back, belt], "male_commoner_v2_loincloth")
    bind_existing_weights(loincloth, rig)
    return loincloth


def create_face_details(rig: bpy.types.Object, target_collection: bpy.types.Collection, white: bpy.types.Material, iris: bpy.types.Material) -> bpy.types.Object:
    parts: list[bpy.types.Object] = []
    for side, sign in (("L", -1), ("R", 1)):
        parts.append(ellipsoid(f"eye_white.{side}", (0.18 * sign, -0.415, 3.00), (0.065, 0.026, 0.046), target_collection, white, "head"))
        parts.append(ellipsoid(f"iris.{side}", (0.18 * sign, -0.446, 3.00), (0.027, 0.012, 0.030), target_collection, iris, "head"))
    face_details = join_meshes(parts, "male_commoner_v2_face_details")
    bind_existing_weights(face_details, rig)
    return face_details


def reset_pose(rig: bpy.types.Object) -> None:
    for bone in rig.pose.bones:
        bone.location = (0, 0, 0)
        bone.rotation_euler = (0, 0, 0)


def insert_pose(rig: bpy.types.Object, frame: int, rotations: dict[str, tuple[int, float]], root_height: float) -> None:
    bpy.context.scene.frame_set(frame)
    reset_pose(rig)
    rig.pose.bones["root"].location.z = root_height
    rig.pose.bones["root"].keyframe_insert(data_path="location", frame=frame)
    for bone_name, (axis, angle) in rotations.items():
        pose_bone = rig.pose.bones[bone_name]
        pose_bone.rotation_euler[axis] = angle
        pose_bone.keyframe_insert(data_path="rotation_euler", frame=frame)


def finish_action(action: bpy.types.Action, frame_count: int) -> None:
    for layer in action.layers:
        for strip in layer.strips:
            for channel_bag in strip.channelbags:
                for curve in channel_bag.fcurves:
                    for key in curve.keyframe_points:
                        key.interpolation = "BEZIER"
                    curve.modifiers.new(type="CYCLES")
    action["frame_count"] = frame_count
    action["loop"] = True


def create_actions(rig: bpy.types.Object) -> None:
    rig.animation_data_create()
    idle = bpy.data.actions.new("idle")
    rig.animation_data.action = idle
    for frame, height, chest_angle in ((1, 0.00, math.radians(-1.2)), (13, 0.028, math.radians(1.2)), (25, 0.00, math.radians(-1.2))):
        insert_pose(
            rig,
            frame,
            {
                "spine": (0, chest_angle),
                "chest": (0, -chest_angle * 0.5),
                "head": (0, chest_angle * 0.35),
                "upper_arm.L": (0, math.radians(-72)),
                "upper_arm.R": (0, math.radians(-72)),
            },
            height,
        )
    finish_action(idle, 24)
    idle.use_fake_user = True

    walk = bpy.data.actions.new("walk")
    rig.animation_data.action = walk
    phases = (0.0, 0.72, 1.0, 0.72, 0.0, -0.72, -1.0, -0.72, 0.0)
    for frame, phase in enumerate(phases, start=1):
        leg = math.radians(27) * phase
        arm = math.radians(22) * phase
        insert_pose(
            rig,
            frame,
            {
                "thigh.L": (0, leg),
                "thigh.R": (0, -leg),
                "shin.L": (0, math.radians(17) * max(0, -phase)),
                "shin.R": (0, math.radians(17) * max(0, phase)),
                "upper_arm.L": (0, math.radians(-72) + arm),
                "upper_arm.R": (0, math.radians(-72) - arm),
                "forearm.L": (0, math.radians(8) * max(0, phase)),
                "forearm.R": (0, -math.radians(8) * max(0, -phase)),
                "spine": (0, math.radians(2.4) * phase),
            },
            0.028 * (1 - abs(phase)),
        )
    finish_action(walk, 8)
    walk.use_fake_user = True
    rig.animation_data.action = idle


def look_at(obj: bpy.types.Object, target: tuple[float, float, float]) -> None:
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def create_cameras(target_collection: bpy.types.Collection) -> dict[str, bpy.types.Object]:
    positions = {"ne": (7.4, -7.4, 5.5), "e": (9.0, 0, 5.5), "se": (7.4, 7.4, 5.5), "s": (0, 9.0, 5.5), "sw": (-7.4, 7.4, 5.5)}
    result: dict[str, bpy.types.Object] = {}
    for name, position in positions.items():
        data = bpy.data.cameras.new(f"camera_{name}")
        data.type = "ORTHO"
        data.ortho_scale = 5.25
        camera = bpy.data.objects.new(f"camera_{name}", data)
        target_collection.objects.link(camera)
        camera.location = position
        look_at(camera, (0, 0, 1.35))
        result[name] = camera
    return result


def create_lighting(target_collection: bpy.types.Collection) -> None:
    def area(name: str, location: tuple[float, float, float], color: tuple[float, float, float], energy: float, size: float) -> None:
        data = bpy.data.lights.new(name, "AREA")
        data.energy = energy
        data.color = color
        data.shape = "DISK"
        data.size = size
        obj = bpy.data.objects.new(name, data)
        target_collection.objects.link(obj)
        obj.location = location
        look_at(obj, (0, 0, 1.4))

    area("key", (4, -5, 6), (1.0, 0.62, 0.40), 1050, 4.0)
    area("fill", (-4, -2, 4), (0.45, 0.58, 0.90), 420, 5.0)
    area("rim", (0, 5, 5), (1.0, 0.46, 0.20), 620, 3.0)
    floor = material("review_floor", (0.045, 0.032, 0.028, 1))
    bpy.ops.mesh.primitive_plane_add(size=200, location=(0, 0, -0.62))
    plane = bpy.context.object
    plane.name = "review_floor"
    plane.data.materials.append(floor)
    move_to_collection(plane, target_collection)


def configure_render() -> None:
    scene = bpy.context.scene
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 512
    scene.render.resolution_y = 512
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.fps = FPS
    scene.world.use_nodes = True
    background = scene.world.node_tree.nodes.get("Background")
    background.inputs["Color"].default_value = (0.013, 0.010, 0.013, 1)
    background.inputs["Strength"].default_value = 0.30


def validate(rig: bpy.types.Object, cameras: dict[str, bpy.types.Object], body: bpy.types.Object) -> None:
    for collection_name in ("BODY", "HAIR", "BASE_LOINCLOTH", "FACE_DETAILS", "RIG", "CAMERAS", "LIGHTING"):
        if bpy.data.collections.get(collection_name) is None:
            raise RuntimeError(f"Missing collection {collection_name}")
    for action_name, frames in (("idle", 24), ("walk", 8)):
        action = bpy.data.actions.get(action_name)
        if action is None or action.get("frame_count") != frames:
            raise RuntimeError(f"Invalid action {action_name}")
    if not any(modifier.type == "ARMATURE" and modifier.object == rig for modifier in body.modifiers):
        raise RuntimeError("Body is not rigged")
    required_groups = {"spine", "head", "upper_arm.L", "upper_arm.R", "thigh.L", "thigh.R"}
    if not required_groups.issubset({group.name for group in body.vertex_groups}):
        raise RuntimeError("Body has incomplete skinning groups")
    if set(cameras) != {"ne", "e", "se", "s", "sw"}:
        raise RuntimeError("Bake cameras are incomplete")
    if len(rig.pose.bones) < 20:
        raise RuntimeError("Rig is incomplete")


def render_previews(rig: bpy.types.Object, cameras: dict[str, bpy.types.Object]) -> None:
    PREVIEWS.mkdir(parents=True, exist_ok=True)
    scene = bpy.context.scene
    rig.animation_data.action = bpy.data.actions["idle"]
    scene.frame_set(1)
    for view_name, camera in cameras.items():
        scene.camera = camera
        scene.render.filepath = str(PREVIEWS / f"male_commoner_v2_idle_{view_name}.png")
        bpy.ops.render.render(write_still=True)
    scene.camera = cameras["ne"]
    rig.animation_data.action = bpy.data.actions["walk"]
    scene.frame_set(3)
    scene.render.filepath = str(PREVIEWS / "male_commoner_v2_walk_ne_frame_03.png")
    bpy.ops.render.render(write_still=True)
    rig.animation_data.action = bpy.data.actions["idle"]
    scene.frame_set(1)


def main() -> None:
    OUTPUT.mkdir(parents=True, exist_ok=True)
    bpy.context.preferences.filepaths.save_version = 0
    clear_scene()
    body_collection = collection("BODY")
    hair_collection = collection("HAIR")
    cloth_collection = collection("BASE_LOINCLOTH")
    face_collection = collection("FACE_DETAILS")
    rig_collection = collection("RIG")
    camera_collection = collection("CAMERAS")
    lighting_collection = collection("LIGHTING")
    skin = material("skin_warm", (0.50, 0.20, 0.075, 1))
    hair = material("hair_chestnut", (0.095, 0.018, 0.005, 1))
    linen = material("linen_unbleached", (0.66, 0.47, 0.24, 1))
    leather = material("belt_leather", (0.095, 0.019, 0.006, 1))
    eye_white = material("eye_white", (0.85, 0.74, 0.57, 1), 0.55)
    iris = material("eye_amber", (0.12, 0.034, 0.008, 1), 0.48)
    rig = create_rig(rig_collection)
    body = create_body(rig, body_collection, skin)
    create_hair(rig, hair_collection, hair)
    create_loincloth(rig, cloth_collection, linen, leather)
    create_face_details(rig, face_collection, eye_white, iris)
    create_actions(rig)
    cameras = create_cameras(camera_collection)
    create_lighting(lighting_collection)
    configure_render()
    validate(rig, cameras, body)
    bpy.context.scene["origin_bake_contract"] = json.dumps({"character": "male_commoner_v2", "fps": FPS, "directions": list(cameras), "layers": ["BODY", "HAIR", "BASE_LOINCLOTH", "FACE_DETAILS"], "actions": {"idle": 24, "walk": 8}})
    bpy.ops.wm.save_as_mainfile(filepath=str(BLEND_PATH))
    render_previews(rig, cameras)
    bpy.ops.wm.save_as_mainfile(filepath=str(BLEND_PATH))
    print(f"Created {BLEND_PATH}")


if __name__ == "__main__":
    main()
