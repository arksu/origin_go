"""Derive an editable cartoon source and tiny-frame proofs from the saved v3.

Run with Blender --background --python tools/blender/adapt_male_commoner_v3_pixel.py.
The original v3 and its UVs remain available; this writes only pixel_style/.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import math
import sys
from pathlib import Path

import bpy
import bmesh
import numpy as np
from mathutils import Vector
from mathutils.bvhtree import BVHTree

sys.path.insert(0, str(Path(__file__).resolve().parent))
from commoner_concept_refinement import refine_source

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / "art_source/characters/male_commoner_v3"
OUT = SOURCE / "pixel_style"
CONFIG = json.loads((OUT / "style_config.json").read_text())
JOINTS = {name: np.array(value) for name, value in json.loads((SOURCE / "landmarks.json").read_text()).items()}
DIRECTIONS = {"s": 0, "se": 45, "e": 90, "ne": 135, "n": 180, "nw": 225, "w": 270, "sw": 315}


def smoothstep(start, end, values):
    amount = np.clip((values - start) / (end - start), 0, 1)
    return amount * amount * (3 - 2 * amount)


def stylize(coordinates):
    """One continuous deformation keeps skin, eyelids, hair and clothing fitted."""
    original = coordinates.copy()
    result = coordinates.copy()
    horizontal, depth, height = original.T
    for side, sign in (("l", 1), ("r", -1)):
        shoulder = JOINTS[f"{side}-shoulder"]
        elbow = JOINTS[f"{side}-elbow"]
        wrist = JOINTS[f"{side}-hand"]
        shoulder_relative = original - shoulder
        deltoid = np.exp(-np.sum((shoulder_relative / [.125, .15, .13]) ** 2, axis=1))
        deltoid *= smoothstep(.14, .25, horizontal * sign)
        result += shoulder_relative * (CONFIG["shoulder_scale"] - 1) * deltoid[:, None]
        for start, end, scale, radius in ((shoulder, elbow, CONFIG["upper_arm_scale"], .18), (elbow, wrist, CONFIG["forearm_scale"], .135)):
            axis = end - start
            relative = original - start
            along = relative @ axis / np.dot(axis, axis)
            radial = relative - along[:, None] * axis
            envelope = smoothstep(-.18, .18, along) * (1 - smoothstep(.76, 1.12, along))
            envelope *= np.exp(-(np.linalg.norm(radial, axis=1) / radius) ** 4)
            envelope *= smoothstep(.19, .29, horizontal * sign)
            result += radial * (scale - 1) * envelope[:, None]
        eye_center = JOINTS[f"{side}-eye"]
        relative = original - eye_center
        eye_weight = np.exp(-((relative[:, 0] / .038) ** 4 + (relative[:, 2] / .045) ** 4))
        eye_weight *= 1 - smoothstep(-.10, -.045, depth)
        result += relative * (np.array(CONFIG["eye_scale"]) - 1) * eye_weight[:, None]

        hand_axis = JOINTS[f"{side}-finger-3-1"] - wrist
        hand_axis /= np.linalg.norm(hand_axis)
        distance_from_wrist = (original - wrist) @ hand_axis
        hand_weight = smoothstep(-.07, .025, distance_from_wrist)
        hand_weight *= smoothstep(.44, .52, horizontal * sign)
        result += (original - wrist) * (CONFIG["hand_scale"] - 1) * hand_weight[:, None]

        ankle = JOINTS[f"{side}-ankle"].copy()
        ankle[2] = .025
        foot_weight = (1 - smoothstep(.10, .24, height)) * smoothstep(.05, .12, horizontal * sign)
        result += (original - ankle) * (np.array(CONFIG["foot_scale"]) - 1) * foot_weight[:, None]

    # Broader lower legs and less pinched wrists preserve the extremities at 48 px.
    lower_leg = (1 - smoothstep(.85, 1.04, height)) * smoothstep(.10, .28, height)
    # The same deformation also fits clothing: a hard sign at the centreline folds
    # the cloth across itself, so the left/right leg influence must blend continuously.
    leg_center = np.tanh(horizontal / .07) * np.interp(height, [.1, .55, 1.03], [.212, .160, .118])
    result[:, 0] += (horizontal - leg_center) * .14 * lower_leg
    result[:, 1] += depth * .07 * lower_leg
    torso = smoothstep(.99, 1.26, height) * (1 - smoothstep(1.53, 1.64, height))
    result[:, 0] *= 1 + .045 * torso

    # A shorter, broader jaw and a small nose avoid a realistic adult face on a big head.
    front_face = 1 - smoothstep(-.13, -.07, depth)
    lower_face = np.exp(-((height - 1.668) / .052) ** 2) * np.exp(-(horizontal / .12) ** 2)
    result[:, 0] *= 1 + .04 * lower_face
    nose = np.exp(-(horizontal / .027) ** 2 - ((height - 1.744) / .039) ** 2) * front_face
    result[:, 1] += .004 * nose

    head_weight = smoothstep(1.565, 1.666, height)
    head_pivot = np.array([0, -.045, 1.632])
    result += (result - head_pivot) * (np.array(CONFIG["head_scale"]) - 1) * head_weight[:, None]
    result[:, 2] = np.interp(result[:, 2], CONFIG["height_source"], CONFIG["height_stylized"])
    return result


def relaxed_pose(coordinates):
    """Static presentation pose only; retains a separate editable rest source."""
    result = coordinates.copy()
    for side, sign in (("l", 1), ("r", -1)):
        shoulder = stylize(JOINTS[f"{side}-shoulder"][None, :])[0]
        local = coordinates - shoulder
        blend = smoothstep(.015, .14, local[:, 0] * sign)
        blend *= smoothstep(.70, .85, coordinates[:, 2])
        angle = math.radians(CONFIG["arm_relax_degrees"]) * sign * blend
        result[:, 0] += (np.cos(angle) - 1) * local[:, 0] + np.sin(angle) * local[:, 2]
        result[:, 2] += -np.sin(angle) * local[:, 0] + (np.cos(angle) - 1) * local[:, 2]
    return result


def apply_transform(obj, transform):
    inverse = obj.matrix_world.inverted()
    matrix = np.array(obj.matrix_world)
    inverse_matrix = np.array(inverse)
    def update(points):
        coordinates = np.empty(len(points) * 3, dtype=np.float64)
        points.foreach_get("co", coordinates)
        coordinates = coordinates.reshape((-1, 3)) @ matrix[:3, :3].T + matrix[:3, 3]
        coordinates = transform(coordinates)
        coordinates = (coordinates - matrix[:3, 3]) @ inverse_matrix[:3, :3].T
        points.foreach_set("co", coordinates.ravel())
    if obj.data.shape_keys:
        for block in obj.data.shape_keys.key_blocks:
            update(block.data)
    else:
        update(obj.data.vertices)
    obj.data.update()


def matte(name, color, toon_strength=.45):
    material = bpy.data.materials.new(name)
    material.diffuse_color = (*color, 1)
    material.use_nodes = True
    nodes, links = material.node_tree.nodes, material.node_tree.links
    nodes.clear()
    diffuse = nodes.new("ShaderNodeBsdfDiffuse")
    diffuse.inputs["Color"].default_value = (*color, 1)
    diffuse.inputs["Roughness"].default_value = .8
    toon = nodes.new("ShaderNodeBsdfToon")
    toon.inputs["Color"].default_value = (*color, 1)
    toon.inputs["Size"].default_value = .62
    toon.inputs["Smooth"].default_value = .17
    mix = nodes.new("ShaderNodeMixShader")
    mix.inputs[0].default_value = toon_strength
    links.new(diffuse.outputs[0], mix.inputs[1])
    links.new(toon.outputs[0], mix.inputs[2])
    output = nodes.new("ShaderNodeOutputMaterial")
    links.new(mix.outputs[0], output.inputs["Surface"])
    return material


def character_objects():
    return [obj for obj in bpy.data.objects if any(coll.name.startswith(("01", "02", "03", "04", "05", "80")) for coll in obj.users_collection)]


def adapt_character():
    body = next(obj for obj in character_objects() if obj.name.startswith("Body"))
    body.data.shape_keys.key_blocks["Concept • muscular surface sculpt"].value = CONFIG["body_sculpt_strength"]
    refine_source(body, CONFIG["concept_refinement"])
    materials = {
        "skin": matte("Pixel • warm skin / large shapes", (.62, .30, .12), .25),
        "hair": matte("Pixel • chestnut / broad locks", (.15, .060, .024), .35),
        "hair_dark": matte("Pixel • chestnut shadow locks", (.115, .042, .015), .35),
        "hair_light": matte("Pixel • chestnut light locks", (.19, .082, .030), .35),
        "brow": matte("Pixel • bold brows", (.044, .020, .018), .15),
        "linen": matte("Pixel • cream cloth / no weave", (.72, .60, .38), .60),
        "edge": matte("Pixel • cloth edge", (.63, .48, .27), .55),
    }
    for obj in character_objects():
        if obj.name.startswith(("Nail", "Brow fibre")) or "hem stitch" in obj.name:
            bpy.data.objects.remove(obj, do_unlink=True)
            continue
        if obj.type == "CURVE":
            bpy.ops.object.select_all(action="DESELECT")
            obj.hide_set(False)
            obj.select_set(True)
            bpy.context.view_layer.objects.active = obj
            bpy.ops.object.convert(target="MESH")
            obj = bpy.context.view_layer.objects.active
        if obj.type != "MESH":
            continue
        if obj.name.startswith("Hair"):
            smoothing = obj.modifiers.new("Broad lock surfaces", "SMOOTH")
            smoothing.factor = .65
            smoothing.iterations = 5
        apply_transform(obj, stylize)
        if obj.name.startswith("Eyes"):
            continue
        material = materials["skin"]
        if obj.name.startswith("Hair"):
            tone = int(obj.name[-2:]) % 5 if obj.name[-2:].isdigit() else 0
            material = materials["hair_light" if tone == 2 else "hair_dark" if tone == 4 else "hair"]
        elif obj.name.startswith("Brow"):
            material = materials["brow"]
        elif any(coll.name.startswith("04") for coll in obj.users_collection):
            material = materials["linen"]
            if "hem" in obj.name:
                material = materials["edge"]
        obj.data.materials.clear()
        obj.data.materials.append(material)
    bpy.context.view_layer.update()
    smoothing = body.modifiers.new("Soft cartoon anatomy", "SMOOTH")
    smoothing.factor = .65
    smoothing.iterations = 12
    face = body.vertex_groups.new(name="Face • reduce micro-creases")
    for vertex in body.data.vertices:
        if vertex.co.z > 1.41:
            face.add([vertex.index], float(smoothstep(1.41, 1.46, vertex.co.z)), "REPLACE")
    smoothing = body.modifiers.new("Clean facial planes", "SMOOTH")
    smoothing.vertex_group = face.name
    smoothing.factor = .8
    smoothing.iterations = 32
    return body


def configure_studio():
    scene = bpy.context.scene
    scene.render.engine = "CYCLES"
    scene.cycles.samples = 32
    scene.cycles.use_denoising = True
    scene.cycles.adaptive_threshold = .04
    scene.cycles.adaptive_min_samples = 12
    scene.cycles.max_bounces = 5
    scene.view_settings.view_transform = "Standard"
    scene.view_settings.look = "None"
    scene.view_settings.exposure = 0
    scene.view_settings.gamma = 1
    scene.render.image_settings.file_format = "PNG"
    scene.render.image_settings.color_mode = "RGBA"
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = False
    scene.render.filter_size = 1.0
    for obj in bpy.data.objects:
        if obj.type == "LIGHT":
            obj.hide_render = True
    light = bpy.data.lights.new("Pixel • upper left key", "AREA")
    light.energy = 650
    light.shape = "DISK"
    light.size = 1.7
    lamp = bpy.data.objects.new(light.name, light)
    scene.collection.objects.link(lamp)
    lamp.location = (-3, -4, 6)
    lamp.rotation_euler = (Vector((0, 0, .9)) - lamp.location).to_track_quat("-Z", "Y").to_euler()
    background = scene.world.node_tree.nodes.get("Background")
    background.inputs["Color"].default_value = (.80, .88, 1, 1)
    background.inputs["Strength"].default_value = .40


def place_camera(name, azimuth, elevation, target, scale):
    existing = bpy.data.objects.get(name)
    if existing:
        camera = existing
    else:
        camera = bpy.data.objects.new(name, bpy.data.cameras.new(name))
        bpy.context.scene.collection.objects.link(camera)
    azimuth, elevation = math.radians(azimuth), math.radians(elevation)
    camera.location = Vector(target) + Vector((math.sin(azimuth) * math.cos(elevation), -math.cos(azimuth) * math.cos(elevation), math.sin(elevation))) * 6
    camera.rotation_euler = (Vector(target) - camera.location).to_track_quat("-Z", "Y").to_euler()
    camera.data.type = "ORTHO"
    camera.data.ortho_scale = scale
    bpy.context.scene.camera = camera
    key = bpy.data.objects.get("Pixel • upper left key")
    if key:
        # Large model-review views use a matching studio key. The sprite bake
        # calls this once, then rotates only the actor beneath the fixed setup.
        key.location = (-3 * math.cos(azimuth) + 4 * math.sin(azimuth), -3 * math.sin(azimuth) - 4 * math.cos(azimuth), 6)
        key.rotation_euler = (Vector((0, 0, .9)) - key.location).to_track_quat("-Z", "Y").to_euler()
    return camera


def render(path, size, samples):
    scene = bpy.context.scene
    scene.render.resolution_x, scene.render.resolution_y = size
    scene.cycles.samples = samples
    path.parent.mkdir(parents=True, exist_ok=True)
    scene.render.filepath = str(path)
    bpy.ops.render.render(write_still=True)


def validate(body):
    report = {"source_sha256": hashlib.sha256((SOURCE / "male_commoner_v3.blend").read_bytes()).hexdigest(), "config_sha256": hashlib.sha256((OUT / "style_config.json").read_bytes()).hexdigest(), "blender": bpy.app.version_string, "rigged": False, "objects": {}}
    for obj in character_objects():
        if obj.type != "MESH":
            continue
        coordinates = np.empty(len(obj.data.vertices) * 3)
        obj.data.vertices.foreach_get("co", coordinates)
        if not np.isfinite(coordinates).all():
            raise ValueError(f"Nonfinite geometry: {obj.name}")
        if "broad concept panel" in obj.name:
            rows = coordinates.reshape((obj["grid_rows"], obj["grid_columns"], 3))
            if np.min(np.diff(rows[:, :, 0], axis=1)) <= 0:
                raise ValueError(f"Cloth crosses itself at its centreline: {obj.name}")
        report["objects"][obj.name] = {"vertices": len(obj.data.vertices), "faces": len(obj.data.polygons)}
    editable = bmesh.new()
    editable.from_mesh(body.data)
    if any(len(edge.link_faces) != 2 for edge in editable.edges):
        raise ValueError("Stylized body has an open or nonmanifold edge")
    if any(face.calc_area() < 1e-14 for face in editable.faces):
        raise ValueError("Stylized body has a degenerate face")
    editable.free()
    report["checks"] = {"finite_geometry": True, "closed_body": True, "no_degenerate_body_faces": True, "editable_shape_keys": len(body.data.shape_keys.key_blocks), "body_uv_layers": len(body.data.uv_layers)}
    if any("side tie" in obj.name for obj in character_objects()):
        raise ValueError("Obsolete projecting cloth ties remain in the character")
    depsgraph = bpy.context.evaluated_depsgraph_get()
    surface = BVHTree.FromObject(body, depsgraph)
    band = bpy.data.objects["Linen • fitted folded waistband"].evaluated_get(depsgraph)
    mesh = band.to_mesh()
    clearances = []
    for vertex in mesh.vertices:
        nearest, normal, _, distance = surface.find_nearest(vertex.co)
        if nearest is None:
            raise ValueError("Cannot verify waistband clearance")
        clearances.append((vertex.co - nearest).dot(normal))
    band.to_mesh_clear()
    if min(clearances) < -.001 or max(clearances) > .030:
        raise ValueError(f"Waistband does not fit the torso: {min(clearances):.5f}..{max(clearances):.5f} m")
    report["waistband_clearance_m"] = {"min": min(clearances), "max": max(clearances)}
    report["checks"]["waistband_fits_torso"] = True
    report["checks"]["no_projecting_ties"] = True
    report["checks"]["cloth_rows_do_not_fold_across_centreline"] = True
    (OUT / "validation.json").write_text(json.dumps(report, indent=2) + "\n")


def bake_raw():
    scene = bpy.context.scene
    scene.render.film_transparent = True
    floor = bpy.data.objects.get("Studio ground")
    floor.hide_render = True
    place_camera("Pixel • fixed bake camera", 0, CONFIG["camera_elevation_degrees"], (0, 0, CONFIG["camera_target_height"]), CONFIG["camera_scale"])
    turntable = bpy.data.objects.new("Pixel • MODEL ROTATION ONLY", None)
    scene.collection.objects.link(turntable)
    turntable.empty_display_type = "CIRCLE"
    turntable.empty_display_size = .6
    for obj in character_objects():
        world_matrix = obj.matrix_world.copy()
        obj.parent = turntable
        obj.matrix_world = world_matrix
    scene.frame_start, scene.frame_end = 1, len(DIRECTIONS)
    scene.render.fps = 8
    bpy.context.view_layer.update()
    initial_camera = np.array(scene.camera.matrix_world)
    key = bpy.data.objects["Pixel • upper left key"]
    initial_key = np.array(key.matrix_world)
    for name in DIRECTIONS:
        frame = list(DIRECTIONS).index(name) + 1
        turntable.rotation_euler.z = -math.radians(DIRECTIONS[name])
        turntable.keyframe_insert(data_path="rotation_euler", frame=frame)
        scene.frame_set(frame)
        if not np.allclose(initial_camera, np.array(scene.camera.matrix_world)) or not np.allclose(initial_key, np.array(key.matrix_world)):
            raise ValueError("Camera or light moved during the model turntable bake")
        for width, height in CONFIG["frame_sizes"]:
            factor = CONFIG["supersampling"]
            render(OUT / "bake/raw" / f"{name}_{width}x{height}.png", (width * factor, height * factor), 32)
    scene.frame_set(1)
    scene.render.resolution_x, scene.render.resolution_y = (48 * CONFIG["supersampling"], 64 * CONFIG["supersampling"])
    scene["asset_stage"] = "Static bake pose; only MODEL ROTATION ONLY turns on frames 1–8; camera and light fixed"
    scene["bake_rotation_subject"] = "model"
    scene["bake_frame_directions"] = json.dumps(list(DIRECTIONS))
    bpy.ops.wm.save_as_mainfile(filepath=str(OUT / "bake/male_commoner_v3_pixel_bake.blend"), compress=True)
    validate_bake_scene()
    floor.hide_render = False
    scene.render.film_transparent = False


def validate_bake_scene():
    scene = bpy.context.scene
    camera = scene.camera
    light = bpy.data.objects["Pixel • upper left key"]
    turntable = bpy.data.objects["Pixel • MODEL ROTATION ONLY"]
    scene.frame_set(1)
    camera_matrix = np.array(camera.matrix_world)
    light_matrix = np.array(light.matrix_world)
    if camera.parent or light.parent or camera.animation_data or light.animation_data:
        raise ValueError("Bake camera and light must be independent and unanimated")
    frames = {}
    for frame, (direction, angle) in enumerate(DIRECTIONS.items(), start=1):
        scene.frame_set(frame)
        if not np.allclose(camera_matrix, np.array(camera.matrix_world)):
            raise ValueError(f"Bake camera moved at frame {frame}")
        if not np.allclose(light_matrix, np.array(light.matrix_world)):
            raise ValueError(f"Bake light moved at frame {frame}")
        actual = math.degrees(turntable.rotation_euler.z)
        if not math.isclose(actual, -angle, abs_tol=.0001):
            raise ValueError(f"Incorrect model rotation at {direction}: {actual}")
        frames[str(frame)] = {"direction": direction, "model_rotation_z_degrees": -angle}
    scene.frame_set(1)
    contract = {"rotation_subject": "model", "rotation_origin": [0, 0, 0], "camera_fixed": True, "key_light_fixed": True, "camera_world_matrix": camera_matrix.tolist(), "key_light_world_matrix": light_matrix.tolist(), "frames": frames}
    (OUT / "bake/scene_contract.json").write_text(json.dumps(contract, indent=2) + "\n")
    print("Verified: model turns through 8 directions; camera and light stay fixed", flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--preview", action="store_true")
    parser.add_argument("--bake-only", action="store_true")
    parser.add_argument("--verify-saved", action="store_true")
    parser.add_argument("--verify-bake", action="store_true")
    args = parser.parse_args(sys.argv[sys.argv.index("--") + 1:] if "--" in sys.argv else [])
    bpy.context.preferences.filepaths.save_version = 0
    destination = OUT / "male_commoner_v3_pixel.blend"
    if args.verify_bake:
        bpy.ops.wm.open_mainfile(filepath=str(OUT / "bake/male_commoner_v3_pixel_bake.blend"))
        validate_bake_scene()
        return
    bpy.ops.wm.open_mainfile(filepath=str(destination if args.bake_only or args.verify_saved else SOURCE / "male_commoner_v3.blend"))
    if args.verify_saved:
        body = next(obj for obj in character_objects() if obj.name.startswith("Body"))
        validate(body)
        print("Saved cartoon source verified", flush=True)
        return
    if not args.bake_only:
        body = adapt_character()
        configure_studio()
        scene = bpy.context.scene
        scene["asset_name"] = "Male commoner v3 • pixel cartoon source"
        scene["asset_stage"] = "Static cartoon source; rest mesh and UVs retained; no animation rig"
        scene["primary_frame_size"] = "48x64"
        for name, path in (("ABOUT THIS CHARACTER", OUT / "README.md"), ("PIXEL STYLE CONFIG", OUT / "style_config.json")):
            block = bpy.data.texts.get(name) or bpy.data.texts.new(name)
            block.clear()
            block.write(path.read_text())
        (OUT / "landmarks.json").write_text(json.dumps({name: stylize(point[None, :])[0].tolist() for name, point in JOINTS.items()}, indent=2) + "\n")
        validate(body)
        place_camera("Pixel • model review", 30, 12, (0, -.02, .92), 2.05)
        scene.render.resolution_x, scene.render.resolution_y = (900, 1100)
        bpy.ops.object.select_all(action="DESELECT")
        body.select_set(True)
        bpy.context.view_layer.objects.active = body
        bpy.ops.wm.save_as_mainfile(filepath=str(destination), compress=True)
        for name, azimuth, elevation, target, scale in [
            ("hero", 30, 12, (0, -.02, .92), 2.05),
            ("front", 0, 0, (0, 0, .93), 2.05),
            ("back", 180, 0, (0, 0, .93), 2.05),
            ("portrait", 18, 5, (0, -.09, 1.62), .58),
            ("linen_side", 90, 0, (0, 0, .82), .52),
            ("linen_front", 0, 0, (0, -.02, .81), .52),
        ]:
            place_camera("Pixel • model review", azimuth, elevation, target, scale)
            render(OUT / "previews" / f"model_{name}.png", (720, 900) if args.preview else (1200, 1500), 24 if args.preview else 64)
        place_camera("Pixel • model review", 30, 12, (0, -.02, .92), 2.05)
        scene.render.resolution_x, scene.render.resolution_y = (1200, 1500)
        bpy.ops.wm.save_as_mainfile(filepath=str(destination), compress=True)
    # The bake pose is derived in memory; the saved source stays in its rest pose.
    for obj in character_objects():
        if obj.type == "MESH" and not obj.hide_render:
            apply_transform(obj, relaxed_pose)
    place_camera("Pixel • posed review", 35, CONFIG["camera_elevation_degrees"], (0, 0, .87), 2.08)
    render(OUT / "previews/model_bake_pose.png", (900, 1200), 32)
    bake_raw()
    print("Cartoon source and small-frame render proofs complete", flush=True)


if __name__ == "__main__":
    main()
