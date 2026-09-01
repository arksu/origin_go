"""Build the first editable source character for the 2D bake pipeline.

Run from the repository root:

    /Applications/Blender.app/Contents/MacOS/Blender --background \
      --python tools/blender/generate_male_commoner_v1.py

The model intentionally uses rigid low-poly body parts parented to a compact
armature. This makes the source asset easy to inspect and replace later with
weighted clothing meshes without changing its animation/action contract.
"""

from __future__ import annotations

import json
import math
from pathlib import Path

import bpy
from mathutils import Vector


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
OUTPUT_DIRECTORY = REPOSITORY_ROOT / "art_source" / "characters" / "male_commoner_v1"
BLEND_PATH = OUTPUT_DIRECTORY / "male_commoner_v1.blend"
PREVIEW_DIRECTORY = OUTPUT_DIRECTORY / "previews"
FPS = 12


def clear_scene() -> None:
    bpy.ops.object.select_all(action="SELECT")
    bpy.ops.object.delete(use_global=False)

    scene_root = bpy.context.scene.collection
    for collection in list(scene_root.children):
        scene_root.children.unlink(collection)
        bpy.data.collections.remove(collection)


def new_collection(name: str) -> bpy.types.Collection:
    collection = bpy.data.collections.new(name)
    bpy.context.scene.collection.children.link(collection)
    return collection


def move_to_collection(obj: bpy.types.Object, collection: bpy.types.Collection) -> None:
    for previous_collection in list(obj.users_collection):
        previous_collection.objects.unlink(obj)
    collection.objects.link(obj)


def make_material(name: str, color: tuple[float, float, float, float]) -> bpy.types.Material:
    material = bpy.data.materials.new(name)
    material.diffuse_color = color
    material.use_nodes = True
    shader = material.node_tree.nodes.get("Principled BSDF")
    shader.inputs["Base Color"].default_value = color
    shader.inputs["Roughness"].default_value = 0.92
    shader.inputs["Specular IOR Level"].default_value = 0.15
    return material


def assign_material(obj: bpy.types.Object, material: bpy.types.Material) -> None:
    obj.data.materials.append(material)


def make_ico_sphere(
    name: str,
    location: tuple[float, float, float],
    scale: tuple[float, float, float],
    material: bpy.types.Material,
    collection: bpy.types.Collection,
) -> bpy.types.Object:
    bpy.ops.mesh.primitive_ico_sphere_add(subdivisions=2, radius=1, location=location)
    obj = bpy.context.object
    obj.name = name
    obj.scale = scale
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    assign_material(obj, material)
    move_to_collection(obj, collection)
    return obj


def make_cone_between(
    name: str,
    start: tuple[float, float, float],
    end: tuple[float, float, float],
    radius_start: float,
    radius_end: float,
    material: bpy.types.Material,
    collection: bpy.types.Collection,
) -> bpy.types.Object:
    start_vector = Vector(start)
    end_vector = Vector(end)
    direction = end_vector - start_vector
    bpy.ops.mesh.primitive_cone_add(
        vertices=8,
        radius1=radius_start,
        radius2=radius_end,
        depth=direction.length,
        location=(start_vector + end_vector) / 2,
    )
    obj = bpy.context.object
    obj.name = name
    obj.rotation_mode = "QUATERNION"
    obj.rotation_quaternion = direction.to_track_quat("Z", "Y")
    assign_material(obj, material)
    move_to_collection(obj, collection)
    return obj


def make_box(
    name: str,
    location: tuple[float, float, float],
    scale: tuple[float, float, float],
    material: bpy.types.Material,
    collection: bpy.types.Collection,
) -> bpy.types.Object:
    bpy.ops.mesh.primitive_cube_add(size=1, location=location)
    obj = bpy.context.object
    obj.name = name
    obj.scale = scale
    bpy.ops.object.transform_apply(location=False, rotation=False, scale=True)
    assign_material(obj, material)
    move_to_collection(obj, collection)
    return obj


def parent_to_bone(obj: bpy.types.Object, rig: bpy.types.Object, bone_name: str) -> None:
    """Keep the object in its authored world pose, then bind it to a bone."""
    world_matrix = obj.matrix_world.copy()
    bone_world_matrix = rig.matrix_world @ rig.pose.bones[bone_name].matrix
    obj.parent = rig
    obj.parent_type = "BONE"
    obj.parent_bone = bone_name
    obj.matrix_parent_inverse = bone_world_matrix.inverted()
    obj.matrix_world = world_matrix


def create_rig(collection: bpy.types.Collection) -> bpy.types.Object:
    armature_data = bpy.data.armatures.new("male_commoner_v1_rig")
    rig = bpy.data.objects.new("male_commoner_v1_rig", armature_data)
    collection.objects.link(rig)
    bpy.context.view_layer.objects.active = rig
    rig.select_set(True)
    bpy.ops.object.mode_set(mode="EDIT")

    bones: dict[str, tuple[tuple[float, float, float], tuple[float, float, float], str | None]] = {
        "root": ((0, 0, -0.28), (0, 0, 0.20), None),
        "pelvis": ((0, 0, 0.20), (0, 0, 0.95), "root"),
        "spine": ((0, 0, 0.95), (0, 0, 1.95), "pelvis"),
        "neck": ((0, 0, 1.95), (0, 0, 2.28), "spine"),
        "head": ((0, 0, 2.28), (0, 0, 2.86), "neck"),
        "upper_arm.L": ((-0.52, 0, 1.83), (-1.12, 0, 1.42), "spine"),
        "forearm.L": ((-1.12, 0, 1.42), (-1.24, -0.02, 0.76), "upper_arm.L"),
        "hand.L": ((-1.24, -0.02, 0.76), (-1.24, -0.10, 0.48), "forearm.L"),
        "upper_arm.R": ((0.52, 0, 1.83), (1.12, 0, 1.42), "spine"),
        "forearm.R": ((1.12, 0, 1.42), (1.24, -0.02, 0.76), "upper_arm.R"),
        "hand.R": ((1.24, -0.02, 0.76), (1.24, -0.10, 0.48), "forearm.R"),
        "thigh.L": ((-0.30, 0, 0.82), (-0.34, 0.02, 0.05), "pelvis"),
        "shin.L": ((-0.34, 0.02, 0.05), (-0.34, -0.03, -0.40), "thigh.L"),
        "foot.L": ((-0.34, -0.03, -0.40), (-0.34, -0.48, -0.40), "shin.L"),
        "thigh.R": ((0.30, 0, 0.82), (0.34, 0.02, 0.05), "pelvis"),
        "shin.R": ((0.34, 0.02, 0.05), (0.34, -0.03, -0.40), "thigh.R"),
        "foot.R": ((0.34, -0.03, -0.40), (0.34, -0.48, -0.40), "shin.R"),
    }
    created: dict[str, bpy.types.EditBone] = {}
    for name, (head, tail, parent_name) in bones.items():
        bone = armature_data.edit_bones.new(name)
        bone.head = head
        bone.tail = tail
        bone.use_deform = name != "root"
        if parent_name:
            bone.parent = created[parent_name]
        created[name] = bone

    bpy.ops.object.mode_set(mode="POSE")
    for pose_bone in rig.pose.bones:
        pose_bone.rotation_mode = "XYZ"
    bpy.ops.object.mode_set(mode="OBJECT")
    rig.show_in_front = True
    return rig


def create_character(
    rig: bpy.types.Object,
    body: bpy.types.Collection,
    hair: bpy.types.Collection,
    loincloth: bpy.types.Collection,
) -> None:
    skin = make_material("skin_warm_olive", (0.46, 0.22, 0.11, 1))
    skin_shadow = make_material("skin_shadow", (0.25, 0.09, 0.04, 1))
    hair_material = make_material("hair_chestnut", (0.12, 0.035, 0.012, 1))
    cloth = make_material("loincloth_ochre", (0.22, 0.075, 0.020, 1))
    belt = make_material("belt_leather", (0.10, 0.025, 0.008, 1))

    torso = make_cone_between("body_torso", (0, 0, 0.88), (0, 0, 2.08), 0.62, 0.78, skin, body)
    parent_to_bone(torso, rig, "spine")
    pelvis = make_ico_sphere("body_pelvis", (0, 0, 0.88), (0.61, 0.45, 0.38), skin, body)
    parent_to_bone(pelvis, rig, "pelvis")
    neck = make_cone_between("body_neck", (0, 0, 2.02), (0, 0, 2.35), 0.19, 0.22, skin, body)
    parent_to_bone(neck, rig, "neck")
    head = make_ico_sphere("body_head", (0, -0.02, 2.68), (0.50, 0.43, 0.58), skin, body)
    parent_to_bone(head, rig, "head")
    nose = make_ico_sphere("body_nose", (0, -0.44, 2.70), (0.09, 0.11, 0.12), skin_shadow, body)
    parent_to_bone(nose, rig, "head")

    for side, x in (("L", -1), ("R", 1)):
        upper_arm = make_cone_between(
            f"body_upper_arm.{side}", (0.58 * x, 0, 1.82), (1.14 * x, 0, 1.41), 0.20, 0.16, skin, body
        )
        parent_to_bone(upper_arm, rig, f"upper_arm.{side}")
        forearm = make_cone_between(
            f"body_forearm.{side}", (1.14 * x, 0, 1.41), (1.25 * x, -0.02, 0.77), 0.15, 0.11, skin, body
        )
        parent_to_bone(forearm, rig, f"forearm.{side}")
        hand = make_ico_sphere(f"body_hand.{side}", (1.25 * x, -0.05, 0.60), (0.15, 0.13, 0.22), skin, body)
        parent_to_bone(hand, rig, f"hand.{side}")

        thigh = make_cone_between(
            f"body_thigh.{side}", (0.30 * x, 0, 0.86), (0.34 * x, 0.02, 0.03), 0.28, 0.19, skin, body
        )
        parent_to_bone(thigh, rig, f"thigh.{side}")
        shin = make_cone_between(
            f"body_shin.{side}", (0.34 * x, 0.02, 0.03), (0.34 * x, -0.03, -0.39), 0.18, 0.12, skin, body
        )
        parent_to_bone(shin, rig, f"shin.{side}")
        foot = make_box(f"body_foot.{side}", (0.34 * x, -0.22, -0.40), (0.21, 0.39, 0.12), skin, body)
        parent_to_bone(foot, rig, f"foot.{side}")

    hair_cap = make_ico_sphere("hair_cap", (0, 0.03, 3.10), (0.54, 0.47, 0.31), hair_material, hair)
    parent_to_bone(hair_cap, rig, "head")
    fringe = make_box("hair_fringe", (0, -0.41, 3.00), (0.37, 0.08, 0.10), hair_material, hair)
    parent_to_bone(fringe, rig, "head")

    skirt = make_cone_between("loincloth_skirt", (0, 0, 1.03), (0, 0, 0.46), 0.58, 0.43, cloth, loincloth)
    parent_to_bone(skirt, rig, "pelvis")
    belt_mesh = make_cone_between("loincloth_belt", (0, 0, 1.10), (0, 0, 1.22), 0.62, 0.62, belt, loincloth)
    parent_to_bone(belt_mesh, rig, "pelvis")


def reset_pose(rig: bpy.types.Object) -> None:
    for pose_bone in rig.pose.bones:
        pose_bone.location = (0, 0, 0)
        pose_bone.rotation_euler = (0, 0, 0)
        pose_bone.scale = (1, 1, 1)


def insert_pose_keyframes(rig: bpy.types.Object, frame: int, rotations: dict[str, float], root_height: float) -> None:
    bpy.context.scene.frame_set(frame)
    reset_pose(rig)
    rig.pose.bones["root"].location.z = root_height
    rig.pose.bones["root"].keyframe_insert(data_path="location", frame=frame)
    for bone_name, rotation in rotations.items():
        bone = rig.pose.bones[bone_name]
        bone.rotation_euler.x = rotation
        bone.keyframe_insert(data_path="rotation_euler", frame=frame)


def set_action_interpolation(action: bpy.types.Action) -> None:
    # Blender 5 stores F-curves in layered action channel bags rather than on
    # Action directly. Traversing the bags also works when an action later has
    # several slots.
    for layer in action.layers:
        for strip in layer.strips:
            for channel_bag in strip.channelbags:
                for curve in channel_bag.fcurves:
                    for keyframe in curve.keyframe_points:
                        keyframe.interpolation = "BEZIER"
                    curve.modifiers.new(type="CYCLES")


def create_actions(rig: bpy.types.Object) -> None:
    rig.animation_data_create()

    idle = bpy.data.actions.new("idle")
    rig.animation_data.action = idle
    for frame, height, spine_sway, head_sway in ((1, 0.00, -0.025, 0.018), (13, 0.035, 0.025, -0.018), (25, 0.00, -0.025, 0.018)):
        insert_pose_keyframes(rig, frame, {"spine": spine_sway, "head": head_sway}, height)
    set_action_interpolation(idle)
    idle["frame_count"] = 24
    idle["loop"] = True

    walk = bpy.data.actions.new("walk")
    rig.animation_data.action = walk
    phases = (0.0, 0.72, 1.0, 0.72, 0.0, -0.72, -1.0, -0.72, 0.0)
    for frame, phase in enumerate(phases, start=1):
        leg = math.radians(29) * phase
        arm = math.radians(23) * phase
        knee = math.radians(17) * max(0.0, -phase)
        insert_pose_keyframes(
            rig,
            frame,
            {
                "thigh.L": leg,
                "thigh.R": -leg,
                "shin.L": knee,
                "shin.R": math.radians(17) * max(0.0, phase),
                "upper_arm.L": -arm,
                "upper_arm.R": arm,
                "forearm.L": math.radians(8) * max(0.0, phase),
                "forearm.R": math.radians(8) * max(0.0, -phase),
                "spine": math.radians(3) * phase,
            },
            0.035 * (1 - abs(phase)),
        )
    set_action_interpolation(walk)
    walk["frame_count"] = 8
    walk["loop"] = True

    rig.animation_data.action = idle


def look_at(obj: bpy.types.Object, target: tuple[float, float, float]) -> None:
    obj.rotation_euler = (Vector(target) - obj.location).to_track_quat("-Z", "Y").to_euler()


def create_cameras(collection: bpy.types.Collection) -> dict[str, bpy.types.Object]:
    views = {
        "ne": (6.6, -6.6, 5.6),
        "e": (8.2, 0.0, 5.6),
        "se": (6.6, 6.6, 5.6),
        "s": (0.0, 8.2, 5.6),
        "sw": (-6.6, 6.6, 5.6),
    }
    cameras: dict[str, bpy.types.Object] = {}
    for view_name, location in views.items():
        camera_data = bpy.data.cameras.new(f"camera_{view_name}")
        camera_data.type = "ORTHO"
        camera_data.ortho_scale = 4.8
        camera = bpy.data.objects.new(f"camera_{view_name}", camera_data)
        collection.objects.link(camera)
        camera.location = location
        look_at(camera, (0, 0, 1.30))
        cameras[view_name] = camera
    return cameras


def create_lighting(collection: bpy.types.Collection) -> None:
    def add_area(name: str, location: tuple[float, float, float], energy: float, size: float, color: tuple[float, float, float]) -> None:
        light_data = bpy.data.lights.new(name, type="AREA")
        light_data.energy = energy
        light_data.shape = "DISK"
        light_data.size = size
        light_data.color = color
        light = bpy.data.objects.new(name, light_data)
        collection.objects.link(light)
        light.location = location
        look_at(light, (0, 0, 1.2))

    add_area("key_light", (3.5, -4.5, 6.0), 850, 4.0, (1.0, 0.63, 0.42))
    add_area("fill_light", (-4.0, -1.0, 3.5), 350, 5.0, (0.35, 0.48, 0.72))
    add_area("rim_light", (0.0, 5.0, 4.5), 500, 3.0, (1.0, 0.52, 0.26))

    floor_material = make_material("preview_ground", (0.06, 0.045, 0.035, 1))
    bpy.ops.mesh.primitive_plane_add(size=200, location=(0, 0, -0.52))
    floor = bpy.context.object
    floor.name = "preview_ground"
    assign_material(floor, floor_material)
    move_to_collection(floor, collection)


def configure_render() -> None:
    scene = bpy.context.scene
    scene.render.engine = "BLENDER_EEVEE"
    scene.render.resolution_x = 256
    scene.render.resolution_y = 256
    scene.render.resolution_percentage = 100
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.film_transparent = False
    scene.render.fps = FPS
    scene.render.resolution_percentage = 100
    scene.world.use_nodes = True
    background = scene.world.node_tree.nodes.get("Background")
    background.inputs["Color"].default_value = (0.018, 0.014, 0.018, 1)
    background.inputs["Strength"].default_value = 0.25


def render_previews(rig: bpy.types.Object, cameras: dict[str, bpy.types.Object]) -> None:
    PREVIEW_DIRECTORY.mkdir(parents=True, exist_ok=True)
    scene = bpy.context.scene
    # `ne` looks toward the authored face (-Y) and is the useful review view.
    scene.camera = cameras["ne"]

    rig.animation_data.action = bpy.data.actions["idle"]
    scene.frame_set(1)
    scene.render.filepath = str(PREVIEW_DIRECTORY / "male_commoner_v1_idle_ne.png")
    bpy.ops.render.render(write_still=True)

    rig.animation_data.action = bpy.data.actions["walk"]
    scene.frame_set(3)
    scene.render.filepath = str(PREVIEW_DIRECTORY / "male_commoner_v1_walk_ne_frame_03.png")
    bpy.ops.render.render(write_still=True)

    rig.animation_data.action = bpy.data.actions["idle"]
    scene.frame_set(1)


def validate_scene(rig: bpy.types.Object, cameras: dict[str, bpy.types.Object]) -> None:
    required_collections = {"BODY", "HAIR", "BASE_LOINCLOTH", "RIG", "CAMERAS", "LIGHTING"}
    missing_collections = sorted(name for name in required_collections if bpy.data.collections.get(name) is None)
    if missing_collections:
        raise RuntimeError(f"Missing required collections: {', '.join(missing_collections)}")

    required_bones = {"root", "pelvis", "spine", "head", "upper_arm.L", "upper_arm.R", "thigh.L", "thigh.R"}
    missing_bones = sorted(name for name in required_bones if rig.pose.bones.get(name) is None)
    if missing_bones:
        raise RuntimeError(f"Missing required bones: {', '.join(missing_bones)}")

    for action_name, expected_frames in (("idle", 24), ("walk", 8)):
        action = bpy.data.actions.get(action_name)
        if action is None:
            raise RuntimeError(f"Missing required action: {action_name}")
        if action.get("frame_count") != expected_frames:
            raise RuntimeError(f"Action {action_name} has an invalid frame count")

    missing_cameras = sorted(name for name in ("ne", "e", "se", "s", "sw") if name not in cameras)
    if missing_cameras:
        raise RuntimeError(f"Missing bake cameras: {', '.join(missing_cameras)}")


def main() -> None:
    OUTPUT_DIRECTORY.mkdir(parents=True, exist_ok=True)
    bpy.context.preferences.filepaths.save_version = 0
    clear_scene()
    body = new_collection("BODY")
    hair = new_collection("HAIR")
    loincloth = new_collection("BASE_LOINCLOTH")
    rig_collection = new_collection("RIG")
    cameras_collection = new_collection("CAMERAS")
    lighting_collection = new_collection("LIGHTING")

    rig = create_rig(rig_collection)
    create_character(rig, body, hair, loincloth)
    create_actions(rig)
    cameras = create_cameras(cameras_collection)
    create_lighting(lighting_collection)
    configure_render()
    validate_scene(rig, cameras)

    bpy.context.scene["origin_bake_contract"] = json.dumps(
        {
            "character": "male_commoner_v1",
            "fps": FPS,
            "directions": ["ne", "e", "se", "s", "sw"],
            "actions": {"idle": {"frames": 24, "loop": True}, "walk": {"frames": 8, "loop": True}},
            "layers": ["BODY", "HAIR", "BASE_LOINCLOTH"],
            "ground_anchor_z": -0.52,
        }
    )
    bpy.ops.wm.save_as_mainfile(filepath=str(BLEND_PATH))
    render_previews(rig, cameras)
    bpy.ops.wm.save_as_mainfile(filepath=str(BLEND_PATH))
    print(f"Created {BLEND_PATH}")


if __name__ == "__main__":
    main()
